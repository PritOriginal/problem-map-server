package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/google/uuid"
)

// ItemKey builds the store key of a per-item idempotency key. It is
// deliberately a different namespace from Key: the middleware stores whole
// HTTP responses, Keys stores the id of one created mark.
func ItemKey(userID int, key string) string {
	return fmt.Sprintf("idem:item:%d:%s", userID, key)
}

// itemRecord is what the store holds under a per-item key: first the
// reservation (MarkID == 0), then the id of the created mark.
type itemRecord struct {
	MarkID int64 `json:"mark_id,omitempty"`
}

// Keys is the per-item idempotency of the batch endpoints. The elements of
// a batch are not separate HTTP requests, so the Idempotency-Key middleware
// cannot deduplicate them; Keys gives every element its own key over the
// same Redis store and TTL.
//
// It lives next to the middleware, outside the usecase, for the same reason
// the middleware does: idempotency is a property of the transport (the
// client's retry), not of the domain.
//
// A nil *Keys is usable: every method is then a no-op reporting "no
// idempotency", so the batch works exactly as it does without Redis.
type Keys struct {
	log   *slog.Logger
	store Store
	ttl   time.Duration
}

// NewKeys returns the per-item idempotency over the same store and TTL as
// the middleware. A nil store disables it (fail open).
func NewKeys(log *slog.Logger, store Store, cfg Config) *Keys {
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	return &Keys{log: log, store: store, ttl: cfg.TTL}
}

// Reserve claims the key for the user before the item is applied.
//
//   - replayed is true when the key already carries a created mark: the item
//     must not be applied again and markID is the id stored for it.
//   - ok reports whether the key is usable at all. It is false for a
//     malformed key, for a disabled or unavailable store and for a key that
//     is currently reserved by another in-flight batch; the caller then
//     applies the item without idempotency (fail open), exactly as it would
//     without Redis.
func (k *Keys) Reserve(ctx context.Context, userID int, key string) (markID int64, replayed bool, ok bool) {
	if k == nil || k.store == nil || key == "" {
		return 0, false, false
	}
	if len(key) > MaxKeyLen || uuid.Validate(key) != nil {
		return 0, false, false
	}

	storeKey := ItemKey(userID, key)
	acquired, err := k.store.SetNX(ctx, storeKey, mustJSON(itemRecord{}), k.ttl)
	if err != nil {
		k.log.Warn("idempotency: item key reserve failed", logger.Err(err))
		return 0, false, false
	}
	if acquired {
		return 0, false, true
	}

	raw, err := k.store.GetBytes(ctx, storeKey)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			k.log.Warn("idempotency: item key read failed", logger.Err(err))
		}
		return 0, false, false
	}
	var rec itemRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		k.log.Warn("idempotency: item key decode failed", logger.Err(err))
		return 0, false, false
	}
	if rec.MarkID == 0 {
		// Reserved by a batch that is still running (or that died before
		// committing): apply the item rather than answer with a mark id
		// that does not exist yet.
		return 0, false, false
	}
	return rec.MarkID, true, true
}

// Commit stores the id created for the key, so that a repeat of the same
// item is replayed instead of creating a second mark.
func (k *Keys) Commit(ctx context.Context, userID int, key string, markID int64) {
	if k == nil || k.store == nil || key == "" || markID == 0 {
		return
	}
	if err := k.store.Set(ctx, ItemKey(userID, key), mustJSON(itemRecord{MarkID: markID}), k.ttl); err != nil {
		k.log.Warn("idempotency: item key commit failed", logger.Err(err))
	}
}

// Release drops the reservation of a key whose item was not created, so
// that the client may retry it with the same key.
func (k *Keys) Release(ctx context.Context, userID int, key string) {
	if k == nil || k.store == nil || key == "" {
		return
	}
	if err := k.store.Del(ctx, ItemKey(userID, key)); err != nil {
		k.log.Warn("idempotency: item key release failed", logger.Err(err))
	}
}
