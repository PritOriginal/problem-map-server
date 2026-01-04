package marksrest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/storage/redis"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
)

type GetMarkByIdResponse struct {
	Mark models.Mark `json:"mark"`
}

type GetMarksResponse struct {
	Marks []models.Mark `json:"marks"`
}

type GetMarkTypesResponse struct {
	MarkTypes []models.MarkType `json:"mark_types"`
}

type GetMarkStatusesResponse struct {
	MarkStatuses []models.MarkStatus `json:"mark_statuses"`
}

type Marks interface {
	GetMarks(ctx context.Context) ([]models.Mark, error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark, photos [][]byte) (int64, error)
	GetMarkTypes(ctx context.Context) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error)
}

type handler struct {
	*handlers.BaseHandler
	uc Marks
}

func Register(r *chi.Mux, auth *jwtauth.JWTAuth, uc Marks, redis *redis.Redis, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/marks", func(r chi.Router) {
		r.Get("/", handler.GetMarks())
		r.Get("/{id}", handler.GetMarkById())
		r.Get("/user/{userId}", handler.GetMarksByUserId())
		r.Group(func(r chi.Router) {
			r.Use(jwtauth.Verifier(auth))
			r.Use(jwtauth.Authenticator(auth))
			r.Post("/", handler.AddMark())
		})
		r.Group(func(r chi.Router) {
			r.Use(mwcache.New(redis, 24*time.Hour))
			r.Get("/types", handler.GetMarkTypes())
			r.Get("/statuses", handler.GetMarkStatuses())
		})
	})
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

func (h *handler) GetMarkById() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		mark, err := h.uc.GetMarkById(context.Background(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.Render(w, r, responses.ErrNotFound)
			} else {
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get mark by id", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetMarkByIdResponse{
			Mark: mark,
		}))
	}
}

func (h *handler) GetMarksByUserId() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userId, err := strconv.Atoi(chi.URLParam(r, "userId"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		marks, err := h.uc.GetMarksByUserId(context.Background(), userId)
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

		_, err = h.uc.AddMark(context.Background(), newMark, photos)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error add mark", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}

func (h *handler) GetMarkTypes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		types, err := h.uc.GetMarkTypes(context.Background())

		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get mark types", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetMarkTypesResponse{
			MarkTypes: types,
		}))
	}
}

func (h *handler) GetMarkStatuses() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses, err := h.uc.GetMarkStatuses(context.Background())

		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get mark statuses", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetMarkStatusesResponse{
			MarkStatuses: statuses,
		}))
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
