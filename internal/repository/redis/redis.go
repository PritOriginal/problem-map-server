package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/redis/go-redis/v9"
)

type Redis struct {
	Client *redis.Client
}

func New(cfg config.RedisConfig) (*Redis, error) {
	const op = "storage.redis.New"

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       0, // use default DB
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Redis{Client: client}, nil
}

// Ping verifies the Redis connection is alive.
func (r *Redis) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

// Close closes the Redis client.
func (r *Redis) Close() error {
	return r.Client.Close()
}

func (r *Redis) Exists(ctx context.Context, key string) bool {
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
	return r.Client.Get(ctx, key).Bytes()
}

func (r *Redis) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
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
	res, err := incrScript.Run(ctx, r.Client, []string{key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return 0, 0, err
	}
	if len(res) != 2 {
		return 0, 0, fmt.Errorf("redis.Incr: unexpected script reply %v", res)
	}
	return res[0], time.Duration(res[1]) * time.Millisecond, nil
}
