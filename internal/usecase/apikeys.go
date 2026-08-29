package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/guregu/null/v6"
)

type APIKeysRepository interface {
	AddAPIKey(ctx context.Context, k models.APIKey) (int64, error)
	GetAPIKeyById(ctx context.Context, id int) (models.APIKey, error)
	GetAPIKeyByHash(ctx context.Context, hash string) (models.APIKey, error)
	GetAPIKeysByOwner(ctx context.Context, ownerUserID int) ([]models.APIKey, error)
	GetAllAPIKeys(ctx context.Context) ([]models.APIKey, error)
	RevokeAPIKey(ctx context.Context, id int) error
	TouchAPIKey(ctx context.Context, id int, at time.Time) error
}

// APIKeyCache memoises verified keys by hash (Redis); every method may fail,
// the usecase then falls back to the repository.
type APIKeyCache interface {
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Del(ctx context.Context, key string) error
}

// APIKeyThrottle is the fixed-window counter used to update last_used_at at
// most once per minute (the same Redis Incr the rate limiter uses).
type APIKeyThrottle interface {
	Incr(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error)
}

type APIKeysRepositories struct {
	APIKeys APIKeysRepository
	// Cache and Throttle are optional: without them every request hits the
	// database and last_used_at is written on every request.
	Cache    APIKeyCache
	Throttle APIKeyThrottle
}

const (
	// APIKeyCacheTTL bounds how long a revoked key found in the cache of
	// another instance keeps working; the revoking instance drops the entry.
	APIKeyCacheTTL = time.Minute
	// APIKeyTouchWindow is how often last_used_at is written per key.
	APIKeyTouchWindow = time.Minute
)

// APIKeys issues, lists and revokes API keys and verifies them for the
// apikey middleware.
type APIKeys struct {
	log   *slog.Logger
	repos APIKeysRepositories
	now   func() time.Time
}

func NewAPIKeys(log *slog.Logger, repos APIKeysRepositories) *APIKeys {
	return &APIKeys{log: log, repos: repos, now: time.Now}
}

// Create issues a key for the actor and returns the stored record together
// with the raw key, which is shown to the client this one time. expiresAt,
// when set, must be in the future.
func (uc *APIKeys) Create(ctx context.Context, actor models.Actor, name string, expiresAt null.Time) (models.APIKey, string, error) {
	const op = "usecase.APIKeys.Create"

	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > models.MaxAPIKeyNameLen {
		return models.APIKey{}, "", fmt.Errorf("%s: %w: name must be 1..%d characters", op, ErrInvalidArgument, models.MaxAPIKeyNameLen)
	}
	if expiresAt.Valid && !expiresAt.Time.After(uc.now()) {
		return models.APIKey{}, "", fmt.Errorf("%s: %w: expires_at must be in the future", op, ErrInvalidArgument)
	}

	raw, err := generateAPIKey()
	if err != nil {
		return models.APIKey{}, "", fmt.Errorf("%s: %w", op, err)
	}

	id, err := uc.repos.APIKeys.AddAPIKey(ctx, models.APIKey{
		OwnerUserID:     actor.UserID,
		Name:            name,
		KeyHash:         models.HashAPIKey(raw),
		Prefix:          models.APIKeyDisplayPrefix(raw),
		Scopes:          []string{string(models.ScopeRead)},
		RateLimitPerMin: models.DefaultAPIKeyRateLimitPerMin,
		Active:          true,
		ExpiresAt:       expiresAt,
	})
	if err != nil {
		return models.APIKey{}, "", mapRepoErr(op, err)
	}

	stored, err := uc.repos.APIKeys.GetAPIKeyById(ctx, int(id))
	if err != nil {
		return models.APIKey{}, "", mapRepoErr(op, err)
	}

	return stored, raw, nil
}

func generateAPIKey() (string, error) {
	buf := make([]byte, models.APIKeySecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return models.APIKeyPrefix + hex.EncodeToString(buf), nil
}

// List returns the actor's keys; with all set an admin gets every key
// (anyone else is refused).
func (uc *APIKeys) List(ctx context.Context, actor models.Actor, all bool) ([]models.APIKey, error) {
	const op = "usecase.APIKeys.List"

	var (
		keys []models.APIKey
		err  error
	)
	if all {
		if actor.Role != models.RoleAdmin {
			return nil, fmt.Errorf("%s: %w", op, ErrForbidden)
		}
		keys, err = uc.repos.APIKeys.GetAllAPIKeys(ctx)
	} else {
		keys, err = uc.repos.APIKeys.GetAPIKeysByOwner(ctx, actor.UserID)
	}
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	return keys, nil
}

// Revoke deactivates the key: the owner or an admin. The cached entry is
// dropped so the key stops working at once on this instance.
func (uc *APIKeys) Revoke(ctx context.Context, actor models.Actor, id int) error {
	const op = "usecase.APIKeys.Revoke"

	k, err := uc.repos.APIKeys.GetAPIKeyById(ctx, id)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if k.OwnerUserID != actor.UserID && actor.Role != models.RoleAdmin {
		return fmt.Errorf("%s: %w", op, ErrForbidden)
	}
	if err := uc.repos.APIKeys.RevokeAPIKey(ctx, id); err != nil {
		return mapRepoErr(op, err)
	}
	uc.forget(ctx, k.KeyHash)

	return nil
}

// ErrAPIKeyRevoked and ErrAPIKeyExpired refine ErrUnauthorized for the
// middleware message.
var (
	ErrAPIKeyRevoked = fmt.Errorf("%w: api key revoked", ErrUnauthorized)
	ErrAPIKeyExpired = fmt.Errorf("%w: api key expired", ErrUnauthorized)
)

// Authenticate verifies a raw key: it must exist (by SHA-256), be active
// and not expired; ErrUnauthorized otherwise. Verified records are cached
// for APIKeyCacheTTL and last_used_at is updated at most once per
// APIKeyTouchWindow.
func (uc *APIKeys) Authenticate(ctx context.Context, raw string) (models.APIKey, error) {
	const op = "usecase.APIKeys.Authenticate"

	if !strings.HasPrefix(raw, models.APIKeyPrefix) {
		return models.APIKey{}, fmt.Errorf("%s: %w: invalid api key", op, ErrUnauthorized)
	}
	hash := models.HashAPIKey(raw)

	k, cached, err := uc.lookup(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return models.APIKey{}, fmt.Errorf("%s: %w: invalid api key", op, ErrUnauthorized)
		}
		return models.APIKey{}, err
	}
	if !k.Active {
		return models.APIKey{}, fmt.Errorf("%s: %w", op, ErrAPIKeyRevoked)
	}
	if k.Expired(uc.now()) {
		return models.APIKey{}, fmt.Errorf("%s: %w", op, ErrAPIKeyExpired)
	}

	if !cached {
		uc.remember(ctx, hash, k)
	}
	uc.touch(ctx, k.ID)

	return k, nil
}

func apiKeyCacheKey(hash string) string { return "apikey:hash:" + hash }

// lookup reads the key from the cache, then from the repository; cached
// reports where it came from.
func (uc *APIKeys) lookup(ctx context.Context, hash string) (models.APIKey, bool, error) {
	const op = "usecase.APIKeys.lookup"

	if uc.repos.Cache != nil {
		if data, err := uc.repos.Cache.GetBytes(ctx, apiKeyCacheKey(hash)); err == nil {
			var k models.APIKey
			if err := json.Unmarshal(data, &k); err == nil && k.ID > 0 {
				return k, true, nil
			}
		}
	}

	k, err := uc.repos.APIKeys.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return models.APIKey{}, false, mapRepoErr(op, err)
	}
	return k, false, nil
}

func (uc *APIKeys) remember(ctx context.Context, hash string, k models.APIKey) {
	if uc.repos.Cache == nil {
		return
	}
	data, err := json.Marshal(k)
	if err != nil {
		return
	}
	if err := uc.repos.Cache.Set(ctx, apiKeyCacheKey(hash), data, APIKeyCacheTTL); err != nil {
		uc.log.Warn("api key cache unavailable", slogger.Err(err))
	}
}

func (uc *APIKeys) forget(ctx context.Context, hash string) {
	if uc.repos.Cache == nil {
		return
	}
	if err := uc.repos.Cache.Del(ctx, apiKeyCacheKey(hash)); err != nil {
		uc.log.Warn("api key cache unavailable", slogger.Err(err))
	}
}

// touch writes last_used_at once per APIKeyTouchWindow; without a throttle
// (or when it is unavailable) the write happens on every request.
func (uc *APIKeys) touch(ctx context.Context, id int) {
	if uc.repos.Throttle != nil {
		count, _, err := uc.repos.Throttle.Incr(ctx, fmt.Sprintf("apikey:touch:%d", id), APIKeyTouchWindow)
		if err == nil && count > 1 {
			return
		}
	}
	if err := uc.repos.APIKeys.TouchAPIKey(ctx, id, uc.now()); err != nil {
		uc.log.Warn("failed to update api key last_used_at", slog.Int("api_key_id", id), slogger.Err(err))
	}
}
