package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// ErrUnavailable is returned by every method of a nil *Redis (or one without
// a client), so that callers built without Redis fail open instead of
// panicking.
var ErrUnavailable = errors.New("redis: client unavailable")

type Redis struct {
	Client *redis.Client
}

// New creates the client and pings the server. When the ping fails the
// client is still returned together with the error: go-redis reconnects
// lazily, so the caller may keep the client and let the cache, limiter and
// token stores fail open until Redis is back.
func New(cfg config.RedisConfig) (*Redis, error) {
	const op = "storage.redis.New"

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       0, // use default DB
	})
	r := &Redis{Client: client}

	if err := client.Ping(context.Background()).Err(); err != nil {
		return r, fmt.Errorf("%s: %w", op, err)
	}

	return r, nil
}

// available reports whether the receiver can issue commands.
func (r *Redis) available() bool {
	return r != nil && r.Client != nil
}

// Ping verifies the Redis connection is alive.
func (r *Redis) Ping(ctx context.Context) error {
	if !r.available() {
		return ErrUnavailable
	}
	return r.Client.Ping(ctx).Err()
}

// Close closes the Redis client.
func (r *Redis) Close() error {
	if !r.available() {
		return nil
	}
	return r.Client.Close()
}

func (r *Redis) Exists(ctx context.Context, key string) bool {
	if !r.available() {
		return false
	}
	return r.Client.Exists(ctx, key).Val() > 0
}

func (r *Redis) Get(ctx context.Context, key string, v any) error {
	data, err := r.GetBytes(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

// GetBytes returns the raw value of the key; a missing key is reported as
// repository.ErrNotFound so callers can tell a miss from a backend failure.
func (r *Redis) GetBytes(ctx context.Context, key string) ([]byte, error) {
	if !r.available() {
		return nil, ErrUnavailable
	}
	data, err := r.Client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, repository.ErrNotFound
	}
	return data, err
}

func (r *Redis) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	if !r.available() {
		return ErrUnavailable
	}
	if err := r.Client.Set(ctx, key, value, expiration).Err(); err != nil {
		return err
	}
	return nil
}

// DeleteByPrefix removes every key starting with prefix (SCAN + DEL in
// batches, so it is safe on a live instance). It is used to invalidate the
// HTTP cache of a dictionary when the dictionary changes.
func (r *Redis) DeleteByPrefix(ctx context.Context, prefix string) error {
	if !r.available() {
		return ErrUnavailable
	}

	pattern := globEscape(prefix) + "*"
	iter := r.Client.Scan(ctx, 0, pattern, 100).Iterator()
	batch := make([]string, 0, 100)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := r.Client.Del(ctx, batch...).Err()
		batch = batch[:0]
		return err
	}
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())
		if len(batch) == cap(batch) {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return flush()
}

// globEscape escapes the glob metacharacters of SCAN MATCH so that a prefix
// is matched literally.
func globEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// SetNX stores the value only when the key does not exist yet and reports
// whether it was stored (SET NX PX). It is the lock primitive of the
// idempotency middleware.
func (r *Redis) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	if !r.available() {
		return false, ErrUnavailable
	}
	return r.Client.SetNX(ctx, key, value, expiration).Result()
}

// Del removes the key; a missing key is not an error.
func (r *Redis) Del(ctx context.Context, key string) error {
	if !r.available() {
		return ErrUnavailable
	}
	return r.Client.Del(ctx, key).Err()
}

// incrScript atomically increments key, sets its TTL when the key has just
// been created (or has somehow lost its TTL) and returns {count, ttl_ms}.
// It only uses INCR/PTTL/PEXPIRE, so it works on Redis < 7.0 too (unlike
// EXPIRE ... NX), and a failure cannot leave a counter without expiry.
var incrScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
local ttl = redis.call("PTTL", KEYS[1])
if count == 1 or ttl < 0 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
	ttl = tonumber(ARGV[1])
end
return {count, ttl}
`)

// Incr increments the integer stored under key and returns the new value
// together with the remaining time before the key expires. The expiration is
// set to window only when the key has just been created.
func (r *Redis) Incr(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error) {
	if !r.available() {
		return 0, 0, ErrUnavailable
	}
	res, err := incrScript.Run(ctx, r.Client, []string{key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return 0, 0, err
	}
	if len(res) != 2 {
		return 0, 0, fmt.Errorf("redis.Incr: unexpected script reply %v", res)
	}
	return res[0], time.Duration(res[1]) * time.Millisecond, nil
}

// Key layout of the auth data. Every active refresh token has its own key
// (so it expires with the token) and its id is also listed in a per-user
// set, so that "revoke everything" is a bounded lookup instead of a SCAN.
const (
	refreshKeyPrefix = "refresh:"
	authVersionKey   = "user:%d:auth_version"
)

// refreshSetKey is the set of active refresh token ids of the user.
func refreshSetKey(userID int) string {
	return refreshKeyPrefix + strconv.Itoa(userID)
}

func refreshKey(userID int, jti string) string {
	return refreshSetKey(userID) + ":" + jti
}

// saveRefreshScript registers the id: KEYS[1] is the token key, KEYS[2] the
// per-user set, ARGV[1] the id and ARGV[2] the TTL in milliseconds. The set
// lives as long as the most recently issued token, so it disappears once
// every token expired.
var saveRefreshScript = redis.NewScript(`
redis.call("SET", KEYS[1], 1, "PX", ARGV[2])
redis.call("SADD", KEYS[2], ARGV[1])
redis.call("PEXPIRE", KEYS[2], ARGV[2])
return 1
`)

// deleteRefreshScript consumes the id atomically: the token key is deleted
// and the id is dropped from the set in one step, and the reply says whether
// the key existed. Two concurrent refreshes with the same token therefore
// cannot both succeed.
var deleteRefreshScript = redis.NewScript(`
local n = redis.call("DEL", KEYS[1])
redis.call("SREM", KEYS[2], ARGV[1])
return n
`)

// deleteAllRefreshScript deletes every token key listed in the user's set
// (KEYS[1], the set key; ARGV[1], the token key prefix) and the set itself.
var deleteAllRefreshScript = redis.NewScript(`
local ids = redis.call("SMEMBERS", KEYS[1])
for _, id in ipairs(ids) do
	redis.call("DEL", ARGV[1] .. id)
end
redis.call("DEL", KEYS[1])
return #ids
`)

// SaveRefresh registers a refresh token id for the user; the entry expires
// together with the token.
func (r *Redis) SaveRefresh(ctx context.Context, userID int, jti string, ttl time.Duration) error {
	if !r.available() {
		return ErrUnavailable
	}
	return saveRefreshScript.Run(ctx, r.Client,
		[]string{refreshKey(userID, jti), refreshSetKey(userID)}, jti, ttl.Milliseconds()).Err()
}

// DeleteRefresh removes a refresh token id and reports whether it existed.
// The check and the removal are one atomic step (see deleteRefreshScript),
// which makes the rotation one-time.
func (r *Redis) DeleteRefresh(ctx context.Context, userID int, jti string) (bool, error) {
	if !r.available() {
		return false, ErrUnavailable
	}
	n, err := deleteRefreshScript.Run(ctx, r.Client,
		[]string{refreshKey(userID, jti), refreshSetKey(userID)}, jti).Int64()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DeleteAllRefresh removes every refresh token id of the user.
func (r *Redis) DeleteAllRefresh(ctx context.Context, userID int) error {
	if !r.available() {
		return ErrUnavailable
	}
	return deleteAllRefreshScript.Run(ctx, r.Client,
		[]string{refreshSetKey(userID)}, refreshSetKey(userID)+":").Err()
}

// AuthVersion returns the user's auth version; a user without one is at 0.
func (r *Redis) AuthVersion(ctx context.Context, userID int) (int64, error) {
	if !r.available() {
		return 0, ErrUnavailable
	}
	v, err := r.Client.Get(ctx, fmt.Sprintf(authVersionKey, userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return v, err
}

// IncrAuthVersion bumps the user's auth version and returns the new value.
// Every token issued with an older version becomes invalid.
func (r *Redis) IncrAuthVersion(ctx context.Context, userID int) (int64, error) {
	if !r.available() {
		return 0, ErrUnavailable
	}
	return r.Client.Incr(ctx, fmt.Sprintf(authVersionKey, userID)).Result()
}
