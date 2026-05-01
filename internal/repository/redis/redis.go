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

func (r *Redis) Stop() error {
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
