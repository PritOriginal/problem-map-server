package maprest

import (
	"context"
	"log/slog"
	"time"

	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

type Map interface {
	GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error)
	GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error)
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type handler struct {
	log *slog.Logger
	uc  Map
}

func Register(r *gin.Engine, log *slog.Logger, uc Map, cacher mwcache.Cacher) {
	handler := &handler{log: log, uc: uc}

	mapRoute := r.Group("/map")
	{
		mapRoute.GET("admin-boundaries/marks/count", handler.GetAdminBoundariesMarksCount())
		cache := mapRoute.Group("")
		{
			cache.Use(mwcache.New(cacher, 24*time.Hour))
			cache.GET("admin-boundaries", handler.GetAdminBoundaries())
			cache.GET("regions", handler.GetRegions())
			cache.GET("cities", handler.GetCities())
			cache.GET("districts", handler.GetDistricts())
		}
	}
}

// GetAdminBoundaries lists all existing administrative boundaries
//
//	@Summary		List administrative boundaries
//	@Description	admin boundaries
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Param			admin_levels	query		[]number	false	"filter by admin level"
//	@Success		200				{object}	responses.Response[maprest.GetAdminBoundariesResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries [get]
func (h *handler) GetAdminBoundaries() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetAdminBoundaries"

		adminLevels, err := handlers.QueryIntArray(c, "admin_levels")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		boundaries, err := h.uc.GetAdminBoundaries(c.Request.Context(), models.GetAdminBoundaryFilters{
			AdminLevels: adminLevels,
		})
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetAdminBoundariesResponse{
			AdminBoundaries: boundaries,
		})
	}
}

// GetAdminBoundariesMarksCount display the count of markers of all administrative boundaries
//
//	@Summary		The count of markers of all administrative boundaries
//	@Description	the count of markers of all administrative boundaries
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Param			admin_levels	query		[]number	false	"filter by admin level"
//	@Param			mark_type_ids	query		[]number	false	"filter by mark type"
//	@Success		200				{object}	responses.Response[maprest.GetAdminBoundariesMarksCountResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries/marks/count [get]
func (h *handler) GetAdminBoundariesMarksCount() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetAdminBoundariesMarksCount"

		adminLevels, err := handlers.QueryIntArray(c, "admin_levels")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		markTypeIds, err := handlers.QueryIntArray(c, "mark_type_ids")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		boundariesCount, err := h.uc.GetAdminBoundariesMarksCount(c.Request.Context(), models.GetAdminBoundaryMarksCountFilters{
			AdminLevels: adminLevels,
			MarkTypeIds: markTypeIds,
		})
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetAdminBoundariesMarksCountResponse{
			AdminBoundaries: boundariesCount,
		})
	}
}

// GetCities lists all existing regions
//
//	@Summary		List regions
//	@Description	get regions
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[maprest.GetRegionsResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/map/regions [get]
func (h *handler) GetRegions() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetRegions"

		regions, err := h.uc.GetRegions(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetRegionsResponse{
			Regions: regions,
		})
	}
}

// GetCities lists all existing cities
//
//	@Summary		List cities
//	@Description	get cities
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[maprest.GetCitiesResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/map/cities [get]
func (h *handler) GetCities() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetCities"

		cities, err := h.uc.GetCities(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetCitiesResponse{
			Cities: cities,
		})
	}
}

// GetDistricts lists all existing districts
//
//	@Summary		List districts
//	@Description	get districts
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[maprest.GetDistrictsResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/map/districts [get]
func (h *handler) GetDistricts() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetDistricts"

		districts, err := h.uc.GetDistricts(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetDistrictsResponse{
			Districts: districts,
		})
	}
}
