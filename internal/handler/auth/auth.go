package authrest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Auth interface {
	SignUp(ctx context.Context, username, login, password string) (int64, error)
	SignIn(ctx context.Context, login, password string) (string, string, error)
	RefreshTokens(ctx context.Context, refreshToken string) (string, string, error)
}

type handler struct {
	*handlers.BaseHandler
	uc Auth
}

func Register(r *chi.Mux, uc Auth, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/signup", handler.SignUp())
		r.Post("/signin", handler.SignIn())
		r.Post("/tokens/refresh", handler.RefreshTokens())
	})
}

// SignUp sign up a new user
//
//	@Summary		Sign Up
//	@Description	sign up a new user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		authrest.SignUpRequest	true	"query params"
//	@Success		201		{object}	responses.SucceededResponse[any]
//	@Failure		400		{object}	responses.ErrorResponse
//	@Failure		409		{object}	responses.ErrorResponse
//	@Failure		500		{object}	responses.ErrorResponse
//	@Router			/auth/signup [post]
func (h *handler) SignUp() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SignUpRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
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

		_, err := h.uc.SignUp(context.Background(), req.Username, req.Username, req.Password)
		if err != nil {
			switch err {
			case usecase.ErrConflict:
				h.RenderError(w, r,
					handlers.HandlerError{Msg: "user already exists", Err: err},
					responses.ErrConflict,
				)
			default:
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed sign up", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}

// SignIn sign up a new user
//
//	@Summary		Sign In
//	@Description	sign in user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		authrest.SignInRequest	true	"query params"
//	@Success		200		{object}	responses.SucceededResponse[authrest.SignInResponse]
//	@Failure		400		{object}	responses.ErrorResponse
//	@Failure		401		{object}	responses.ErrorResponse
//	@Failure		500		{object}	responses.ErrorResponse
//	@Router			/auth/signin [post]
func (h *handler) SignIn() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SignInRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
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

		accessToken, refreshToken, err := h.uc.SignIn(context.Background(), req.Login, req.Password)
		if err != nil {
			switch err {
			case storage.ErrNotFound:
				h.RenderError(w, r,
					handlers.HandlerError{Msg: "failed sign in", Err: err},
					responses.ErrUnauthorized,
				)
			default:
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed sign in", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(SignInResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		}))
	}
}

// RefreshTokens Refresh access and refresh tokens
//
//	@Summary		Refresh tokens
//	@Description	refresh access and refresh tokens
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		authrest.RefreshTokensRequest	true	"query params"
//	@Success		200		{object}	responses.SucceededResponse[authrest.RefreshTokensResponse]
//	@Failure		400		{object}	responses.ErrorResponse
//	@Failure		401		{object}	responses.ErrorResponse
//	@Failure		500		{object}	responses.ErrorResponse
//	@Router			/auth/tokens/refresh [post]
func (h *handler) RefreshTokens() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RefreshTokensRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
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

		accessToken, refreshToken, err := h.uc.RefreshTokens(context.Background(), req.RefreshToken)
		if err != nil {
			switch err {
			case storage.ErrNotFound:
				h.RenderError(w, r,
					handlers.HandlerError{Msg: "failed refresh tokens", Err: err},
					responses.ErrUnauthorized,
				)
			default:
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed login", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(RefreshTokensResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		}))
	}
}
