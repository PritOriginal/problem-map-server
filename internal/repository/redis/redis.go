package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
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

func (r *Redis) GetBytes(ctx context.Context, key string) ([]byte, error) {
	if !r.available() {
		return nil, ErrUnavailable
	}
	return r.Client.Get(ctx, key).Bytes()
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

// Key layout of the auth data.
const (
	refreshKeyPrefix = "refresh:"
	authVersionKey   = "user:%d:auth_version"
)

func refreshKey(userID int, jti string) string {
	return refreshKeyPrefix + strconv.Itoa(userID) + ":" + jti
}

// SaveRefresh registers a refresh token id for the user; the key expires
// together with the token.
func (r *Redis) SaveRefresh(ctx context.Context, userID int, jti string, ttl time.Duration) error {
	if !r.available() {
		return ErrUnavailable
	}
	return r.Client.Set(ctx, refreshKey(userID, jti), 1, ttl).Err()
}

// DeleteRefresh removes a refresh token id and reports whether it existed.
// A single DEL makes the rotation one-time: two concurrent refreshes with
// the same token cannot both succeed.
func (r *Redis) DeleteRefresh(ctx context.Context, userID int, jti string) (bool, error) {
	if !r.available() {
		return false, ErrUnavailable
	}
	n, err := r.Client.Del(ctx, refreshKey(userID, jti)).Result()
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

	pattern := refreshKey(userID, "*")
	iter := r.Client.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return r.Client.Del(ctx, keys...).Err()
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
