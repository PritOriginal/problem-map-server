package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type APIKeysSuite struct {
	suite.Suite
	uc       *usecase.APIKeys
	repo     *usecase.MockAPIKeysRepository
	cache    *usecase.MockAPIKeyCache
	throttle *usecase.MockAPIKeyThrottle
}

func (suite *APIKeysSuite) SetupTest() {
	suite.repo = usecase.NewMockAPIKeysRepository(suite.T())
	suite.cache = usecase.NewMockAPIKeyCache(suite.T())
	suite.throttle = usecase.NewMockAPIKeyThrottle(suite.T())
	suite.uc = usecase.NewAPIKeys(slogdiscard.NewDiscardLogger(), usecase.APIKeysRepositories{
		APIKeys:  suite.repo,
		Cache:    suite.cache,
		Throttle: suite.throttle,
	})
}

func TestAPIKeys(t *testing.T) {
	suite.Run(t, new(APIKeysSuite))
}

const (
	keyOwner = 7
	keyRaw   = "pm_live_0123456789abcdef0123456789abcdef"
)

func keyOwnerActor() models.Actor { return models.Actor{UserID: keyOwner, Role: models.RoleUser} }
func keyAdminActor() models.Actor { return models.Actor{UserID: 1, Role: models.RoleAdmin} }

func storedKey() models.APIKey {
	return models.APIKey{
		ID: 5, OwnerUserID: keyOwner, Name: "dashboard", KeyHash: models.HashAPIKey(keyRaw),
		Prefix: models.APIKeyDisplayPrefix(keyRaw), Scopes: []string{"read"}, RateLimitPerMin: 600, Active: true,
	}
}

func (suite *APIKeysSuite) TestCreate() {
	future := null.TimeFrom(time.Now().Add(time.Hour))
	past := null.TimeFrom(time.Now().Add(-time.Hour))

	tests := []struct {
		name      string
		keyName   string
		expiresAt null.Time
		errAdd    error
		errGet    error
		wantErr   error
	}{
		{name: "Ok", keyName: "dashboard"},
		{name: "OkExpires", keyName: " dashboard ", expiresAt: future},
		{name: "ErrEmptyName", keyName: "  ", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrLongName", keyName: strings.Repeat("x", models.MaxAPIKeyNameLen+1), wantErr: usecase.ErrInvalidArgument},
		{name: "ErrExpiresInPast", keyName: "dashboard", expiresAt: past, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrAdd", keyName: "dashboard", errAdd: errRepo, wantErr: errRepo},
		{name: "ErrGet", keyName: "dashboard", errGet: repository.ErrNotFound, wantErr: usecase.ErrNotFound},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			var added models.APIKey
			if tt.wantErr == nil || tt.errAdd != nil || tt.errGet != nil {
				suite.repo.On("AddAPIKey", mock.Anything, mock.MatchedBy(func(k models.APIKey) bool {
					added = k
					return true
				})).Once().Return(int64(5), tt.errAdd)
			}
			if tt.errAdd == nil && (tt.wantErr == nil || tt.errGet != nil) {
				suite.repo.On("GetAPIKeyById", mock.Anything, 5).Once().Return(storedKey(), tt.errGet)
			}

			got, raw, err := suite.uc.Create(context.Background(), keyOwnerActor(), tt.keyName, tt.expiresAt)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(storedKey(), got)
			suite.True(strings.HasPrefix(raw, models.APIKeyPrefix))
			suite.Len(raw, len(models.APIKeyPrefix)+2*models.APIKeySecretBytes)
			suite.Equal(models.HashAPIKey(raw), added.KeyHash)
			suite.Equal(raw[:models.APIKeyDisplayLen], added.Prefix)
			suite.Equal("dashboard", added.Name)
			suite.Equal(keyOwner, added.OwnerUserID)
			suite.Equal([]string{"read"}, added.Scopes)
			suite.Equal(models.DefaultAPIKeyRateLimitPerMin, added.RateLimitPerMin)
			suite.True(added.Active)
			suite.Equal(tt.expiresAt, added.ExpiresAt)
		})
	}
}

func (suite *APIKeysSuite) TestList() {
	tests := []struct {
		name    string
		actor   models.Actor
		all     bool
		wantErr error
	}{
		{name: "Own", actor: keyOwnerActor()},
		{name: "AdminAll", actor: keyAdminActor(), all: true},
		{name: "AdminOwn", actor: keyAdminActor()},
		{name: "ErrUserAll", actor: keyOwnerActor(), all: true, wantErr: usecase.ErrForbidden},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			switch {
			case tt.wantErr != nil:
			case tt.all:
				suite.repo.On("GetAllAPIKeys", mock.Anything).Once().Return([]models.APIKey{storedKey()}, nil)
			default:
				suite.repo.On("GetAPIKeysByOwner", mock.Anything, tt.actor.UserID).Once().Return([]models.APIKey{storedKey()}, nil)
			}

			got, err := suite.uc.List(context.Background(), tt.actor, tt.all)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal([]models.APIKey{storedKey()}, got)
		})
	}
}

func (suite *APIKeysSuite) TestRevoke() {
	tests := []struct {
		name      string
		actor     models.Actor
		errGet    error
		errRevoke error
		errDel    error
		wantErr   error
	}{
		{name: "Owner", actor: keyOwnerActor()},
		{name: "Admin", actor: keyAdminActor()},
		{name: "CacheDownStillRevoked", actor: keyOwnerActor(), errDel: errors.New("redis down")},
		{name: "ErrOtherUser", actor: models.Actor{UserID: 99, Role: models.RoleModerator}, wantErr: usecase.ErrForbidden},
		{name: "ErrNotFound", actor: keyOwnerActor(), errGet: repository.ErrNotFound, wantErr: usecase.ErrNotFound},
		{name: "ErrRevoke", actor: keyOwnerActor(), errRevoke: errRepo, wantErr: errRepo},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("GetAPIKeyById", mock.Anything, 5).Once().Return(storedKey(), tt.errGet)
			if tt.errGet == nil && !errors.Is(tt.wantErr, usecase.ErrForbidden) {
				suite.repo.On("RevokeAPIKey", mock.Anything, 5).Once().Return(tt.errRevoke)
			}
			if tt.errGet == nil && tt.errRevoke == nil && tt.wantErr == nil {
				suite.cache.On("Del", mock.Anything, "apikey:hash:"+storedKey().KeyHash).Once().Return(tt.errDel)
			}

			err := suite.uc.Revoke(context.Background(), tt.actor, 5)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *APIKeysSuite) TestAuthenticate() {
	cached, err := json.Marshal(storedKey())
	suite.Require().NoError(err)
	revoked := storedKey()
	revoked.Active = false
	expired := storedKey()
	expired.ExpiresAt = null.TimeFrom(time.Now().Add(-time.Minute))
	notYet := storedKey()
	notYet.ExpiresAt = null.TimeFrom(time.Now().Add(time.Hour))

	tests := []struct {
		name        string
		raw         string
		cacheHit    []byte
		repoKey     models.APIKey
		errRepo     error
		touchCount  int64
		errThrottle error
		wantTouch   bool
		wantErr     error
	}{
		{name: "CacheMissThenTouch", raw: keyRaw, repoKey: storedKey(), touchCount: 1, wantTouch: true},
		{name: "CacheHitNoTouch", raw: keyRaw, cacheHit: cached, touchCount: 2},
		{name: "NotExpiredYet", raw: keyRaw, repoKey: notYet, touchCount: 1, wantTouch: true},
		{name: "ThrottleDownTouches", raw: keyRaw, cacheHit: cached, errThrottle: errors.New("redis down"), wantTouch: true},
		{name: "ErrBadPrefix", raw: "sk_test_abc", wantErr: usecase.ErrUnauthorized},
		{name: "ErrUnknown", raw: keyRaw, errRepo: repository.ErrNotFound, wantErr: usecase.ErrUnauthorized},
		{name: "ErrRepo", raw: keyRaw, errRepo: errRepo, wantErr: errRepo},
		{name: "ErrRevoked", raw: keyRaw, repoKey: revoked, wantErr: usecase.ErrAPIKeyRevoked},
		{name: "ErrRevokedFromCache", raw: keyRaw, cacheHit: mustJSON(revoked), wantErr: usecase.ErrAPIKeyRevoked},
		{name: "ErrExpired", raw: keyRaw, repoKey: expired, wantErr: usecase.ErrAPIKeyExpired},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			cacheKey := "apikey:hash:" + models.HashAPIKey(tt.raw)
			if strings.HasPrefix(tt.raw, models.APIKeyPrefix) {
				if tt.cacheHit != nil {
					suite.cache.On("GetBytes", mock.Anything, cacheKey).Once().Return(tt.cacheHit, nil)
				} else {
					suite.cache.On("GetBytes", mock.Anything, cacheKey).Once().Return(nil, errors.New("redis: nil"))
					suite.repo.On("GetAPIKeyByHash", mock.Anything, models.HashAPIKey(tt.raw)).Once().Return(tt.repoKey, tt.errRepo)
				}
			}
			if tt.wantErr == nil {
				if tt.cacheHit == nil {
					suite.cache.On("Set", mock.Anything, cacheKey, mock.Anything, usecase.APIKeyCacheTTL).Once().Return(nil)
				}
				suite.throttle.On("Incr", mock.Anything, "apikey:touch:5", usecase.APIKeyTouchWindow).Once().
					Return(tt.touchCount, time.Minute, tt.errThrottle)
				if tt.wantTouch {
					suite.repo.On("TouchAPIKey", mock.Anything, 5, mock.Anything).Once().Return(nil)
				}
			}

			got, err := suite.uc.Authenticate(context.Background(), tt.raw)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				if errors.Is(tt.wantErr, usecase.ErrAPIKeyRevoked) || errors.Is(tt.wantErr, usecase.ErrAPIKeyExpired) {
					suite.ErrorIs(err, usecase.ErrUnauthorized)
				}
				return
			}
			suite.Require().NoError(err)
			suite.Equal(5, got.ID)
			suite.Equal(keyOwner, got.OwnerUserID)
		})
	}
}

// TestAuthenticateWithoutRedis covers the wiring without cache and throttle:
// every request hits the repository and touches the key.
func (suite *APIKeysSuite) TestAuthenticateWithoutRedis() {
	uc := usecase.NewAPIKeys(slogdiscard.NewDiscardLogger(), usecase.APIKeysRepositories{APIKeys: suite.repo})
	suite.repo.On("GetAPIKeyByHash", mock.Anything, models.HashAPIKey(keyRaw)).Once().Return(storedKey(), nil)
	suite.repo.On("TouchAPIKey", mock.Anything, 5, mock.Anything).Once().Return(errors.New("db hiccup"))

	got, err := uc.Authenticate(context.Background(), keyRaw)
	suite.Require().NoError(err)
	suite.Equal(5, got.ID)
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
