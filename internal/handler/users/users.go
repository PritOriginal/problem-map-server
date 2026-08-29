package usersrest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Users interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	ListUsers(ctx context.Context, p models.Pagination) (models.Page[models.User], error)
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
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		user, err := h.uc.GetUserById(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				h.log.Debug("user not found", slog.Int("id", id))
				responses.NotFound(c, "user not found")
			} else {
				h.log.Error("failed get user by id", slog.Int("id", id), logger.Err(err))
				responses.Internal(c, "failed get user by id")
			}
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
		id, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		user, err := h.uc.GetUserById(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				h.log.Debug("user not found", slog.Int("id", id))
				responses.NotFound(c, "user not found")
			} else {
				h.log.Error("failed get current user", slog.Int("id", id), logger.Err(err))
				responses.Internal(c, "failed get current user")
			}
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
		var query listquery.Pagination
		if err := c.ShouldBindQuery(&query); err != nil {
			h.log.Debug("failed parse query params", logger.Err(err))
			responses.BadRequest(c, "invalid query params")
			return
		}
		p := query.Model()

		page, err := h.uc.ListUsers(c.Request.Context(), p)
		if err != nil {
			if errors.Is(err, usecase.ErrInvalidArgument) {
				h.log.Debug("invalid pagination", logger.Err(err))
				responses.BadRequest(c, "invalid query params")
				return
			}
			h.log.Error("error get users", logger.Err(err))
			responses.Internal(c, "error get users")
			return
		}

		responses.OKList(c, GetUsersResponse{
			Users: NewPublicUsers(page.Items),
		}, listquery.Meta(p, page.Total))
	}
}
