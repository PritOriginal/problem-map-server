package usersrest

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Users interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUsers(ctx context.Context) ([]models.User, error)
}

type handler struct {
	log *slog.Logger
	uc  Users
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Users) {
	handler := &handler{log: log, uc: uc}

	users := r.Group("/users")
	{
		users.GET("", handler.GetUsers())
		auth := users.Group("", authMiddleware.MiddlewareFunc())
		{
			auth.GET("me", handler.GetMe())
		}
		users.GET(":id", handler.GetUserById())
	}
}

// GetUserById returns the public profile of a user
//
//	@Summary		Get user by id
//	@Description	get public user profile by id
//	@Tags			users
//	@Produce		json
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.Response[usersrest.GetUserByIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/users/{id} [get]
func (h *handler) GetUserById() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "usersrest.GetUserById"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		user, err := h.uc.GetUserById(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetUserByIdResponse{
			User: NewPublicUser(user),
		})
	}
}

// GetMe returns the full profile of the authenticated user
//
//	@Summary		Get current user
//	@Description	get full profile of the authenticated user
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	responses.Response[usersrest.GetMeResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/users/me [get]
func (h *handler) GetMe() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "usersrest.GetMe"

		id, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		user, err := h.uc.GetUserById(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetMeResponse{
			User: user,
		})
	}
}

// GetUsers lists all existing users
//
//	@Summary		List users
//	@Description	get public profiles of all users
//	@Tags			users
//	@Produce		json
//	@Success		200	{object}	responses.Response[usersrest.GetUsersResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/users [get]
func (h *handler) GetUsers() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "usersrest.GetUsers"

		users, err := h.uc.GetUsers(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetUsersResponse{
			Users: NewPublicUsers(users),
		})
	}
}
