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
		mapRoute.Use(mwcache.New(cacher, 24*time.Hour))
		mapRoute.GET("regions", handler.GetRegions())
		mapRoute.GET("cities", handler.GetCities())
		mapRoute.GET("districts", handler.GetDistricts())
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
