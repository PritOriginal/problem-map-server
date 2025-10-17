package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/postgres"
)

type PhotosRepository interface {
	AddPhotos(photos [][]byte) error
	GetPhotos() error
}

type Map struct {
	log        *slog.Logger
	mapRepo    postgres.MapRepository
	photosRepo PhotosRepository
}

func NewMap(log *slog.Logger, mapRepo postgres.MapRepository, photosRepo PhotosRepository) *Map {
	return &Map{log, mapRepo, photosRepo}
}

func (uc *Map) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "usecase.Map.GetRegions"

	regions, err := uc.mapRepo.GetRegions(ctx)
	if err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}
	return regions, nil
}

func (uc *Map) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "usecase.Map.GetCities"

	cities, err := uc.mapRepo.GetCities(ctx)
	if err != nil {
		return cities, fmt.Errorf("%s: %w", op, err)
	}
	return cities, nil
}

func (uc *Map) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "usecase.Map.GetDistricts"

	districts, err := uc.mapRepo.GetDistricts(ctx)
	if err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}
	return districts, nil
}

func (uc *Map) GetMarks(ctx context.Context) ([]models.Mark, error) {
	const op = "usecase.Map.GetMarks"

	marks, err := uc.mapRepo.GetMarks(ctx)
	if err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}
	return marks, nil
}

func (uc *Map) AddMark(ctx context.Context, mark models.Mark) error {
	const op = "usecase.Map.AddMark"

	if err := uc.mapRepo.AddMark(ctx, mark); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (uc *Map) AddPhotos(photos [][]byte) error {
	const op = "usecase.Map.AddPhotos"

	if err := uc.photosRepo.AddPhotos(photos); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (uc *Map) GetPhotos() error {

	return nil
}
