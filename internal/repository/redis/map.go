package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type MapRepository struct {
	Redis *Redis
}

func (r *MapRepository) SetRegions(ctx context.Context, regions []models.Region) error {
	const op = "storage.redis.SetRegions"

	if err := r.Redis.Client.Set(ctx, "regions", regions, 86400*time.Second).Err(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (r *MapRepository) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "storage.redis.GetRegions"

	regions := []models.Region{}

	data, err := r.Redis.Client.Get(ctx, "regions").Bytes()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := json.Unmarshal(data, &regions); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return regions, nil
}

func (r *MapRepository) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "storage.redis.GetCities"

	cities := []models.City{}

	data, err := r.Redis.Client.Get(ctx, "cities").Bytes()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := json.Unmarshal(data, &cities); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return cities, nil
}

func (r *MapRepository) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "storage.redis.GetDistricts"

	districts := []models.District{}

	data, err := r.Redis.Client.Get(ctx, "districts").Bytes()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := json.Unmarshal(data, &districts); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return districts, nil
}

// func (r *MapRepository) Test() {
// 	r.Redis.Client.
// }

// func (r *MapRepository) Get[T any](a T) {

// }
