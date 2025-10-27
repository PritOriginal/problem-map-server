package maprest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
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

type GetMarksResponse struct {
	Marks []models.Mark `json:"marks"`
}

type Map interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
	GetMarks(ctx context.Context) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark) (int64, error)
	PhotosRepository
}

type PhotosRepository interface {
	AddPhotos(photos [][]byte) error
	GetPhotos() error
}

type handler struct {
	*handlers.BaseHandler
	uc Map
}

func Register(r *chi.Mux, auth *jwtauth.JWTAuth, uc Map, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/map", func(r chi.Router) {
		r.Get("/regions", handler.GetRegions())
		r.Get("/cities", handler.GetCities())
		r.Get("/districts", handler.GetDistricts())
		r.Route("/marks", func(r chi.Router) {
			r.Get("/", handler.GetMarks())
			r.Group(func(r chi.Router) {
				r.Use(jwtauth.Verifier(auth))
				r.Use(jwtauth.Authenticator(auth))
				r.Post("/", handler.AddMark())
				r.Post("/photos", handler.AddPhotos())
			})
		})
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

func (h *handler) GetMarks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		marks, err := h.uc.GetMarks(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get marks", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(GetMarksResponse{
			Marks: marks,
		}))
	}
}

func (h *handler) AddMark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(32 << 10) // 32 MB
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error parse multipart form", Err: err})
			return
		}

		photos, err := ParsePhotos(w, r)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error parse photos", Err: err})
			return
		}
		var newMark models.Mark
		if err := json.Unmarshal([]byte(r.FormValue("data")), &newMark); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "error unmarshal data", Err: err},
				responses.ErrBadRequest,
			)
			return
		}
		newMark.Geom.Ewkb.SetSRID(4326)

		if _, err := h.uc.AddMark(context.Background(), newMark); err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error add mark", Err: err})
			return
		}
		if err := h.uc.AddPhotos(photos); err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error add photos", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}

func (h *handler) AddPhotos() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(32 << 10) // 32 MB
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error parse multipart form", Err: err})
			return
		}

		photos, err := ParsePhotos(w, r)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error parse photos", Err: err})
			return
		}

		if err := h.uc.AddPhotos(photos); err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error add photos", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}

func ParsePhotos(w http.ResponseWriter, r *http.Request) ([][]byte, error) {
	var photos [][]byte

	for _, fheaders := range r.MultipartForm.File {
		for _, header := range fheaders {
			file, err := header.Open()
			if err != nil {
				return photos, err
			}
			defer file.Close()

			buf := bytes.NewBuffer(nil)
			if _, err := io.Copy(buf, file); err != nil {
				return photos, err
			}
			photo := buf.Bytes()

			photos = append(photos, photo)
		}
	}
	return photos, nil
}
