package maprest

import (
	"context"
	"net/http"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/redis"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
)

type Map interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type handler struct {
	*handlers.BaseHandler
	uc Map
}

func Register(r *chi.Mux, uc Map, redis *redis.Redis, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/map", func(r chi.Router) {
		r.Use(cache.New(redis, 24*time.Hour))
		r.Get("/regions", handler.GetRegions())
		r.Get("/cities", handler.GetCities())
		r.Get("/districts", handler.GetDistricts())
	})
}

// GetCities lists all existing regions
//
//	@Summary		List regions
//	@Description	get regions
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[maprest.GetRegionsResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/map/regions [get]
func (h *handler) GetRegions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		regions, err := h.uc.GetRegions(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get regions", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(GetRegionsResponse{
			Regions: regions,
		}))
	}
}

// GetCities lists all existing cities
//
//	@Summary		List cities
//	@Description	get cities
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[maprest.GetCitiesResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/map/cities [get]
func (h *handler) GetCities() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cities, err := h.uc.GetCities(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get cities", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(GetCitiesResponse{
			Cities: cities,
		}))
	}
}

// GetDistricts lists all existing districts
//
//	@Summary		List districts
//	@Description	get districts
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[maprest.GetDistrictsResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/map/districts [get]
func (h *handler) GetDistricts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		districts, err := h.uc.GetDistricts(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get districts", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(GetDistrictsResponse{
			Districts: districts,
		}))
	}
}
