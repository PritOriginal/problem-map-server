package usecase

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/db"
)

type Map interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
	GetMarks(ctx context.Context) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark) error
}

type MapUseCase struct {
	mapRepo db.MapRepository
}

func NewMap(repo db.MapRepository) *MapUseCase {
	return &MapUseCase{mapRepo: repo}
}

func (uc *MapUseCase) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "usecase.Map.GetRegions"

	regions, err := uc.mapRepo.GetRegions(ctx)
	if err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}
	return regions, nil
}

func (uc *MapUseCase) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "usecase.Map.GetDistricts"

	districts, err := uc.mapRepo.GetDistricts(ctx)
	if err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}
	return districts, nil
}

func (uc *MapUseCase) GetMarks(ctx context.Context) ([]models.Mark, error) {
	const op = "usecase.Map.GetMarks"

	marks, err := uc.mapRepo.GetMarks(ctx)
	if err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}
	return marks, nil
}

func (uc *MapUseCase) AddMark(ctx context.Context, mark models.Mark) error {
	const op = "usecase.Map.AddMark"

	if err := uc.mapRepo.AddMark(ctx, mark); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
