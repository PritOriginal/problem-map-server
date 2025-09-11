package redis

import (
	"context"
	"fmt"

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
