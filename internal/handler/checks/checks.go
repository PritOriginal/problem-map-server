package checksrest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-playground/validator/v10"
)

type Checks interface {
	AddCheck(ctx context.Context, check models.Check, photos [][]byte) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error)
	GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error)
}

type handler struct {
	*handlers.BaseHandler
	uc Checks
}

func Register(r *chi.Mux, auth *jwtauth.JWTAuth, uc Checks, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/checks", func(r chi.Router) {
		r.Get("/{id}", handler.GetCheckById())
		r.Get("/mark/{markId}", handler.GetChecksByMarkId())
		r.Get("/user/{userId}", handler.GetChecksByUserId())
		r.Group(func(r chi.Router) {
			r.Use(jwtauth.Verifier(auth))
			r.Use(jwtauth.Authenticator(auth))
			r.Post("/", handler.AddCheck())
		})
	})
}

// GetCheckById get check by id
//
//	@Summary		Get check by id
//	@Description	get check by id
//	@Tags			checks
//	@Produce		json
//	@Param			id	path		int	true	"check id"
//	@Success		200	{object}	responses.SucceededResponse[checksrest.GetCheckByIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		404	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/checks/{id} [get]
func (h *handler) GetCheckById() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		check, err := h.uc.GetCheckById(context.Background(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.Render(w, r, responses.ErrNotFound)
			} else {
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get check by id", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetCheckByIdResponse{
			Check: check,
		}))
	}
}

// GetChecksByMarkId get check by mark id
//
//	@Summary		Get check by mark id
//	@Description	get check by mark id
//	@Tags			checks
//	@Produce		json
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.SucceededResponse[checksrest.GetChecksByMarkIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/checks/mark/{id} [get]
func (h *handler) GetChecksByMarkId() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		markId, err := strconv.Atoi(chi.URLParam(r, "markId"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		checks, err := h.uc.GetChecksByMarkId(context.Background(), markId)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get checks", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetChecksByMarkIdResponse{
			Checks: checks,
		}))
	}
}

// GetChecksByUserId get checks by user id
//
//	@Summary		List checks by user id
//	@Description	get checks by user id
//	@Tags			checks
//	@Produce		json
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.SucceededResponse[checksrest.GetChecksByUserIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/checks/user/{id} [get]
func (h *handler) GetChecksByUserId() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userId, err := strconv.Atoi(chi.URLParam(r, "userId"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		checks, err := h.uc.GetChecksByUserId(context.Background(), userId)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get checks", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetChecksByUserIdResponse{
			Checks: checks,
		}))
	}
}

// AddCheck add check
//
//	@Summary		Add check
//	@Description	add check
//	@Tags			checks
//	@Accept			mpfd
//	@Produce		json
//	@Param			Authorization	header		string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Success		201				{object}	responses.SucceededResponse[any]
//	@Failure		400				{object}	responses.ErrorResponse
//	@Failure		500				{object}	responses.ErrorResponse
//	@Router			/checks [post]
func (h *handler) AddCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(32 << 10) // 32 MB
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error parse multipart form", Err: err})
			return
		}

		photos, err := h.ParsePhotos(w, r)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error parse photos", Err: err})
			return
		}

		var req AddCheckRequest
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

		check := models.Check{
			UserID:  req.UserID,
			MarkID:  req.UserID,
			Result:  req.Result,
			Comment: req.Comment,
		}
		_, err = h.uc.AddCheck(context.Background(), check, photos)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error add check", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}
