package usecase

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type MapRepository interface {
	GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error)
	GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error)
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

func (uc *Map) GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error) {
	const op = "usecase.Map.GetAdminBoundariesMarksCount"

	boundariesCount, err := uc.repos.Map.GetAdminBoundariesMarksCount(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return boundariesCount, nil
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

	// var districts []models.District
	// var err error

	// const key = "districts"
	// if uc.redis.Exists(ctx, key) {
	// 	var districtsList models.DistrictList
	// 	if err := uc.redis.Get(ctx, key, &districtsList); err != nil {
	// 		return districts, mapRepoErr(op, err)
	// 	}
	// 	districts = districtsList.List
	// } else {
	// 	districts, err = uc.repos.Map.GetDistricts(ctx)
	// 	if err != nil {
	// 		return districts, mapRepoErr(op, err)
	// 	}

	// 	if err := uc.redis.Set(ctx, key, models.DistrictList{List: districts}, 0*time.Second); err != nil {
	// 		return districts, mapRepoErr(op, err)
	// 	}

	// 	uc.log.Debug("cached districts", slog.Int("len", len(districts)))
	// }

	return districts, nil
}
