package usersrest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
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

func Register(r *gin.Engine, log *slog.Logger, uc Users) {
	handler := &handler{log: log, uc: uc}

	users := r.Group("/users")
	{
		users.GET("", handler.GetUsers())
		users.GET(":id", handler.GetUserById())
	}
}

// GetUserById lists all existing users
//
//	@Summary		Get user by id
//	@Description	get user by id
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
			if errors.Is(err, storage.ErrNotFound) {
				h.log.Debug("user not found", slog.Int("id", id))
				responses.NotFound(c, "user not found")
			} else {
				h.log.Error("failed get user by id", slog.Int("id", id), logger.Err(err))
				responses.Internal(c, "failed get user by id")
			}
			return
		}

		responses.OK(c, GetUserByIdResponse{
			User: user,
		})
	}
}

// GetUsers lists all existing users
//
//	@Summary		List users
//	@Description	get users
//	@Tags			users
//	@Produce		json
//	@Success		200	{object}	responses.Response[usersrest.GetUsersResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/users [get]
func (h *handler) GetUsers() gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := h.uc.GetUsers(c.Request.Context())
		if err != nil {
			h.log.Error("error get users", logger.Err(err))
			responses.Internal(c, "error get users")
			return
		}

		responses.OK(c, GetUsersResponse{
			Users: users,
		})
	}
}
