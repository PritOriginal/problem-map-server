package maprest

import (
	"context"
	"log/slog"
	"time"

	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

type Map interface {
	GetAdminBoundaries(ctx context.Context, params models.GetAdminBoundaryParams) ([]models.AdminBoundary, error)
	GetAdminBoundariesMarksCount(ctx context.Context, params models.GetAdminBoundaryMarksCountParams) ([]models.AdminBoundaryMarksCount, error)
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
//	@Param			admin_levels	query		[]number	false	"filter by admin level"	collectionFormat(multi)
//	@Success		200				{object}	responses.Response[maprest.GetAdminBoundariesResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries [get]
func (h *handler) GetAdminBoundaries() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req GetAdminBoundariesRequest
		if err := ctx.ShouldBindQuery(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(ctx, "invalid request")
			return
		}

		boundaries, err := h.uc.GetAdminBoundaries(ctx.Request.Context(), models.GetAdminBoundaryParams{
			AdminLevels: req.AdminLevels,
		})
		if err != nil {
			h.log.Error("error get admin boundaries", logger.Err(err))
			responses.Internal(ctx, "error get admin boundaries")
			return
		}

		responses.OK(ctx, GetAdminBoundariesResponse{
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
//	@Param			admin_levels	query		[]number	false	"filter by admin level"	collectionFormat(multi)
//	@Success		200				{object}	responses.Response[maprest.GetAdminBoundariesMarksCountResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries/marks/count [get]
func (h *handler) GetAdminBoundariesMarksCount() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req GetAdminBoundariesMarksCountRequest
		if err := ctx.ShouldBindQuery(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(ctx, "invalid request")
			return
		}

		boundariesCount, err := h.uc.GetAdminBoundariesMarksCount(ctx.Request.Context(), models.GetAdminBoundaryMarksCountParams{
			AdminLevels: req.AdminLevels,
		})
		if err != nil {
			h.log.Error("error get admin boundaries markers count", logger.Err(err))
			responses.Internal(ctx, "error get admin boundaries markers count")
			return
		}

		responses.OK(ctx, GetAdminBoundariesMarksCountResponse{
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
	return func(ctx *gin.Context) {
		regions, err := h.uc.GetRegions(ctx.Request.Context())
		if err != nil {
			h.log.Error("error get regions", logger.Err(err))
			responses.Internal(ctx, "error get regions")
			return
		}

		responses.OK(ctx, GetRegionsResponse{
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
	return func(ctx *gin.Context) {
		cities, err := h.uc.GetCities(ctx.Request.Context())
		if err != nil {
			h.log.Error("error get cities", logger.Err(err))
			responses.Internal(ctx, "error get cities")
			return
		}

		responses.OK(ctx, GetCitiesResponse{
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
	return func(ctx *gin.Context) {
		districts, err := h.uc.GetDistricts(ctx.Request.Context())
		if err != nil {
			h.log.Error("error get districts", logger.Err(err))
			responses.Internal(ctx, "error get districts")
			return
		}

		responses.OK(ctx, GetDistrictsResponse{
			Districts: districts,
		})
	}
}
