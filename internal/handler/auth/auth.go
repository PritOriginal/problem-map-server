package authrest

import (
	"context"
	"errors"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

type Auth interface {
	SignUp(ctx context.Context, username, login, password string) (int64, error)
	SignIn(ctx context.Context, login, password string) (string, string, error)
	RefreshTokens(ctx context.Context, refreshToken string) (string, string, error)
}

type handler struct {
	log *slog.Logger
	uc  Auth
}

func Register(r *gin.Engine, log *slog.Logger, uc Auth) {
	handler := &handler{log: log, uc: uc}

	auth := r.Group("/auth")
	{
		auth.POST("signup", handler.SignUp())
		auth.POST("signin", handler.SignIn())
		auth.POST("tokens/refresh", handler.RefreshTokens())
	}
}

// SignUp sign up a new user
//
//	@Summary		Sign Up
//	@Description	sign up a new user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		authrest.SignUpRequest	true	"query params"
//	@Success		201		{object}	responses.Response[authrest.SignUpResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/auth/signup [post]
func (h *handler) SignUp() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SignUpRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		userId, err := h.uc.SignUp(c.Request.Context(), req.Username, req.Login, req.Password)
		if err != nil {
			switch err {
			case usecase.ErrConflict:
				h.log.Debug("user already exists", slog.String("login", req.Login))
				responses.Conflict(c, "user already exists")
			default:
				h.log.Error("failed sign up", logger.Err(err))
				responses.Internal(c, "failed sign up")
			}
			return
		}

		h.log.Info("new user has registered", slog.String("login", req.Login), slog.Int64("id", userId))
		responses.Created(c, SignUpResponse{
			UserId: int(userId),
		})
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
//	@Success		200		{object}	responses.Response[authrest.SignInResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/auth/signin [post]
func (h *handler) SignIn() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SignInRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		accessToken, refreshToken, err := h.uc.SignIn(c.Request.Context(), req.Login, req.Password)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.log.Debug("failed sign in")
				responses.Unauthorized(c, "failed sign in")
			} else {
				h.log.Error("failed sign in", logger.Err(err))
				responses.Internal(c, "failed sign in")
			}
			return
		}

		responses.OK(c, SignInResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		})
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
//	@Success		200		{object}	responses.Response[authrest.RefreshTokensResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/auth/tokens/refresh [post]
func (h *handler) RefreshTokens() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshTokensRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		accessToken, refreshToken, err := h.uc.RefreshTokens(c.Request.Context(), req.RefreshToken)
		if err != nil {
			if errors.Is(err, usecase.ErrUnauthorized) {
				h.log.Debug("failed refresh tokens", slog.String("refresh_token", req.RefreshToken))
				responses.Unauthorized(c, "failed refresh tokens")
			} else {
				h.log.Error("failed refresh tokens", logger.Err(err))
				responses.Internal(c, "failed refresh tokens")
			}
			return
		}

		responses.OK(c, RefreshTokensResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		})
	}
}
