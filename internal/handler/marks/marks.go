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
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-playground/validator/v10"
	"github.com/twpayne/go-geom"
)

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

func Register(r *chi.Mux, auth *jwtauth.JWTAuth, uc Marks, cacher mwcache.Cacher, bh *handlers.BaseHandler) {
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
			r.Use(mwcache.New(cacher, 24*time.Hour))
			r.Get("/types", handler.GetMarkTypes())
			r.Get("/statuses", handler.GetMarkStatuses())
		})
	})
}

// GetMarks lists all existing markers
//
//	@Summary		List markers
//	@Description	get markers
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[marksrest.GetMarksResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/marks [get]
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

// GetMarkById get mark by id
//
//	@Summary		Get mark by id
//	@Description	get mark by id
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.SucceededResponse[marksrest.GetMarkByIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		404	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/marks/{id} [get]
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

// GetMarkById List markers by user id
//
//	@Summary		List markers by user id
//	@Description	get markers by user id
//	@Tags			marks
//	@Produce		json
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.SucceededResponse[marksrest.GetMarksByUserIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/marks/user/{id} [get]
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

		h.Render(w, r, responses.SucceededRenderer(GetMarksByUserIdResponse{
			Marks: marks,
		}))
	}
}

// AddMark add mark
//
//	@Summary		Add mark
//	@Description	add mark
//	@Tags			marks
//	@Accept			mpfd
//	@Produce		json
//	@Param			Authorization	header		string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Success		201				{object}	responses.SucceededResponse[any]
//	@Failure		400				{object}	responses.ErrorResponse
//	@Failure		500				{object}	responses.ErrorResponse
//	@Router			/marks [post]
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

		var req AddMarkRequest
		if err := json.Unmarshal([]byte(r.FormValue("data")), &req); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "error unmarshal data", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		if err := h.ValidateStruct(req); err != nil {
			validateErr := err.(validator.ValidationErrors)
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "invalid request", Err: validateErr},
				responses.ErrBadRequest,
			)
			return
		}

		_, claims, err := jwtauth.FromContext(r.Context())
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "invalid token", Err: err},
				responses.ErrUnauthorized,
			)
			return
		}

		userIdStr, ok := claims["sub"].(string)
		if !ok {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "invalid token", Err: err},
				responses.ErrUnauthorized,
			)
			return
		}
		userId, err := strconv.Atoi(userIdStr)
		if !ok {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "invalid token", Err: err},
				responses.ErrUnauthorized,
			)
			return
		}

		h.Log.Debug("Add mark", slog.Int("userId", userId))

		newMark := models.Mark{
			Geom:        models.NewPoint(geom.Coord{req.Point.Latitude, req.Point.Longitude}),
			MarkTypeID:  req.MarkTypeID,
			UserID:      userId,
			Description: req.Description,
		}
		_, err = h.uc.AddMark(context.Background(), newMark, photos)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error add mark", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}

// GetMarkTypes lists all existing mark types
//
//	@Summary		List mark types
//	@Description	get mark types
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[marksrest.GetMarkTypesResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/marks/types [get]
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

// GetMarkStatuses lists all existing mark statuses
//
//	@Summary		List mark statuses
//	@Description	get mark statuses
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[marksrest.GetMarkStatusesResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/marks/statuses [get]
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
