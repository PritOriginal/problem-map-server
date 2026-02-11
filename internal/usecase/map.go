package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type MapRepository interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type Map struct {
	log   *slog.Logger
	repos MapRepositories
}

type MapRepositories struct {
	Map MapRepository
}

func NewMap(log *slog.Logger, repos MapRepositories) *Map {
	return &Map{log, repos}
}

func (uc *Map) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "usecase.Map.GetRegions"

	regions, err := uc.repos.Map.GetRegions(ctx)
	if err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}
	return regions, nil
}

func (uc *Map) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "usecase.Map.GetCities"

	cities, err := uc.repos.Map.GetCities(ctx)
	if err != nil {
		return cities, fmt.Errorf("%s: %w", op, err)
	}
	return cities, nil
}

func (uc *Map) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "usecase.Map.GetDistricts"

	districts, err := uc.repos.Map.GetDistricts(ctx)
	if err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}
	return districts, nil
}
