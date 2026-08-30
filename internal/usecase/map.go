package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type MapRepository interface {
	GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error)
	GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error)
	GetAdminBoundaryById(ctx context.Context, id int) (models.AdminBoundary, error)
	GetHeatmap(ctx context.Context, filters models.HeatmapFilters) ([]models.HeatmapCell, error)
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

func (uc *Map) GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error) {
	const op = "usecase.Map.GetAdminBoundaries"

	boundaries, err := uc.repos.Map.GetAdminBoundaries(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return boundaries, nil
}

// GetAdminBoundaryById returns one boundary with its geometry.
func (uc *Map) GetAdminBoundaryById(ctx context.Context, id int) (models.AdminBoundary, error) {
	const op = "usecase.Map.GetAdminBoundaryById"

	boundary, err := uc.repos.Map.GetAdminBoundaryById(ctx, id)
	if err != nil {
		return models.AdminBoundary{}, mapRepoErr(op, err)
	}
	return boundary, nil
}

func (uc *Map) GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error) {
	const op = "usecase.Map.GetAdminBoundariesMarksCount"

	if err := filters.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	boundariesCount, err := uc.repos.Map.GetAdminBoundariesMarksCount(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return boundariesCount, nil
}

// GetHeatmap applies the default cell size and rejects grids that would
// exceed models.MaxHeatmapCells with ErrTooManyHeatmapCells.
func (uc *Map) GetHeatmap(ctx context.Context, filters models.HeatmapFilters) ([]models.HeatmapCell, error) {
	const op = "usecase.Map.GetHeatmap"

	if filters.CellM == 0 {
		filters.CellM = models.DefaultHeatmapCellM
	}
	if err := filters.Validate(); err != nil {
		if errors.Is(err, models.ErrTooManyHeatmapCells) {
			return nil, fmt.Errorf("%s: %w", op, ErrTooManyHeatmapCells)
		}
		return nil, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	cells, err := uc.repos.Map.GetHeatmap(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return cells, nil
}

func (uc *Map) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "usecase.Map.GetRegions"

	regions, err := uc.repos.Map.GetRegions(ctx)
	if err != nil {
		return regions, mapRepoErr(op, err)
	}
	return regions, nil
}

func (uc *Map) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "usecase.Map.GetCities"

	cities, err := uc.repos.Map.GetCities(ctx)
	if err != nil {
		return cities, mapRepoErr(op, err)
	}
	return cities, nil
}

func (uc *Map) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "usecase.Map.GetDistricts"

	districts, err := uc.repos.Map.GetDistricts(ctx)
	if err != nil {
		return districts, mapRepoErr(op, err)
	}

	return districts, nil
}
