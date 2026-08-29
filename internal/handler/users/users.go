package usersrest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
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
	ListUsers(ctx context.Context, p models.Pagination) (models.Page[models.User], error)
	ChangePassword(ctx context.Context, id int, oldPassword, newPassword string) error
	SetRole(ctx context.Context, actorID, id int, role models.Role) error
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
			auth.POST("me/password", handler.ChangePassword())
			auth.PATCH(":id/role", middleware.RequireRole(models.RoleAdmin), handler.SetRole())
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

// GetUsers lists users, paginated
//
//	@Summary		List users
//	@Description	get public profiles of users; pagination info is returned in the top-level `meta` field ({limit, offset, total})
//	@Tags			users
//	@Produce		json
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[usersrest.GetUsersResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/users [get]
func (h *handler) GetUsers() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "usersrest.GetUsers"

		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		page, err := h.uc.ListUsers(c.Request.Context(), p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetUsersResponse{Users: NewPublicUsers(page.Items)}, p, page.Total)
	}
}

// ChangePassword changes the password of the authenticated user
//
//	@Summary		Change password
//	@Description	change the password of the authenticated user; every session of the user is revoked, sign in again afterwards
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body	usersrest.ChangePasswordRequest	true	"old and new password"
//	@Success		204
//	@Failure		400	{object}	responses.Response[any]	"invalid request or new password shorter than 8"
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]	"old password does not match"
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/users/me/password [post]
func (h *handler) ChangePassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "usersrest.ChangePassword"

		id, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		var req ChangePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		if err := h.uc.ChangePassword(c.Request.Context(), id, req.OldPassword, req.NewPassword); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		c.Status(http.StatusNoContent)
	}
}

// SetRole changes the role of a user (admin only)
//
//	@Summary		Set user role
//	@Description	change the role of a user; the user's sessions are revoked so the new role applies immediately. The last admin cannot give up the admin role
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path	int							true	"user id"
//	@Param			request	body	usersrest.SetRoleRequest	true	"new role"
//	@Success		204
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]	"not an admin, or the last admin demoting themselves"
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/users/{id}/role [patch]
func (h *handler) SetRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "usersrest.SetRole"

		actorID, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req SetRoleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		if err := h.uc.SetRole(c.Request.Context(), actorID, id, req.Role); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		c.Status(http.StatusNoContent)
	}
}
