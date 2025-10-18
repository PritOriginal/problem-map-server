package authrest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type SignUpRequest struct {
	Name     string `json:"name" validate:"required"`
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=64"`
}

type SignInRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=64"`
}

type SignInResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type RefreshTokensRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type RefreshTokensResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Auth interface {
	SignUp(ctx context.Context, name, username, password string) (int64, error)
	SignIn(ctx context.Context, username, password string) (string, string, error)
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

		_, err := h.uc.SignUp(context.Background(), req.Name, req.Username, req.Password)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed sign up", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededResponseOK)
	}
}

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

		accessToken, refreshToken, err := h.uc.SignIn(context.Background(), req.Username, req.Password)
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
