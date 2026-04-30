package maprest

import (
	"context"
	"log/slog"
	"time"

	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
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
		adminLevelsStr := c.Query("admin_levels")
		adminLevels, err := handlers.ParseIntArray(adminLevelsStr)
		if err != nil {
			h.log.Debug("failed parse admin levels", logger.Err(err))
			responses.BadRequest(c, "failed parse admin levels")
			return
		}

		boundaries, err := h.uc.GetAdminBoundaries(c.Request.Context(), models.GetAdminBoundaryFilters{
			AdminLevels: adminLevels,
		})
		if err != nil {
			h.log.Error("error get admin boundaries", logger.Err(err))
			responses.Internal(c, "error get admin boundaries")
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
		adminLevelsStr := c.Query("admin_levels")
		adminLevels, err := handlers.ParseIntArray(adminLevelsStr)
		if err != nil {
			h.log.Debug("failed parse admin levels", logger.Err(err))
			responses.BadRequest(c, "failed parse admin levels")
			return
		}

		markTypeIdsStr := c.Query("mark_type_ids")
		markTypeIds, err := handlers.ParseIntArray(markTypeIdsStr)
		if err != nil {
			h.log.Debug("failed parse mark type ids", logger.Err(err))
			responses.BadRequest(c, "failed parse mark type ids")
			return
		}

		boundariesCount, err := h.uc.GetAdminBoundariesMarksCount(c.Request.Context(), models.GetAdminBoundaryMarksCountFilters{
			AdminLevels: adminLevels,
			MarkTypeIds: markTypeIds,
		})
		if err != nil {
			h.log.Error("error get admin boundaries markers count", logger.Err(err))
			responses.Internal(c, "error get admin boundaries markers count")
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
		regions, err := h.uc.GetRegions(c.Request.Context())
		if err != nil {
			h.log.Error("error get regions", logger.Err(err))
			responses.Internal(c, "error get regions")
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
		cities, err := h.uc.GetCities(c.Request.Context())
		if err != nil {
			h.log.Error("error get cities", logger.Err(err))
			responses.Internal(c, "error get cities")
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
		districts, err := h.uc.GetDistricts(c.Request.Context())
		if err != nil {
			h.log.Error("error get districts", logger.Err(err))
			responses.Internal(c, "error get districts")
			return
		}

		responses.OK(c, GetDistrictsResponse{
			Districts: districts,
		})
	}
}
