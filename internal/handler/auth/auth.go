package authrest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
)

type SignUpRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type SignInRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SignInResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type RefreshTokensRequest struct {
	RefreshToken string `json:"refresh_token"`
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
	handlers.BaseHandler
	uc Auth
}

func Register(r *chi.Mux, log *slog.Logger, uc Auth) {
	handler := &handler{handlers.BaseHandler{Log: log}, uc}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/signup", handler.SignUp())
		r.Post("/signin", handler.SignIn())
		r.Post("/tokens/refresh", handler.RefreshTokens())
	})
}

func (h *handler) SignUp() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var signUpRequest SignUpRequest
		if err := json.NewDecoder(r.Body).Decode(&signUpRequest); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		_, err := h.uc.SignUp(context.Background(), signUpRequest.Name, signUpRequest.Username, signUpRequest.Password)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed sign up", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededResponseOK)
	}
}

func (h *handler) SignIn() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var signInRequest SignInRequest
		if err := json.NewDecoder(r.Body).Decode(&signInRequest); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		accessToken, refreshToken, err := h.uc.SignIn(context.Background(), signInRequest.Username, signInRequest.Password)
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
		var refreshTokenRequest RefreshTokensRequest
		if err := json.NewDecoder(r.Body).Decode(&refreshTokenRequest); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		accessToken, refreshToken, err := h.uc.RefreshTokens(context.Background(), refreshTokenRequest.RefreshToken)
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
