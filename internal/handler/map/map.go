package maprest

import (
	"context"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
)

type GetRegionsResponse struct {
	Regions []models.Region `json:"regions"`
}

type GetCitiesResponse struct {
	Cities []models.City `json:"cities"`
}

type GetDistrictsResponse struct {
	Districts []models.District `json:"districts"`
}

type Map interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type handler struct {
	*handlers.BaseHandler
	uc Map
}

func Register(r *chi.Mux, uc Map, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/map", func(r chi.Router) {
		r.Get("/regions", handler.GetRegions())
		r.Get("/cities", handler.GetCities())
		r.Get("/districts", handler.GetDistricts())
	})
}

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
