package checksrest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Checks interface {
	AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error)
	GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error)
}

type handler struct {
	log *slog.Logger
	uc  Checks
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Checks) {
	handler := &handler{log: log, uc: uc}

	checks := r.Group("/checks")
	{
		checks.GET(":id", handler.GetCheckById())
		checks.GET("mark/:markId", handler.GetChecksByMarkId())
		checks.GET("user/:userId", handler.GetChecksByUserId())
		auth := checks.Group("", authMiddleware.MiddlewareFunc())
		{
			auth.POST("", handler.AddCheck())
		}
	}
}

// GetCheckById get check by id
//
//	@Summary		Get check by id
//	@Description	get check by id
//	@Tags			checks
//	@Produce		json
//	@Param			id	path		int	true	"check id"
//	@Success		200	{object}	responses.Response[checksrest.GetCheckByIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/checks/{id} [get]
func (h *handler) GetCheckById() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		check, err := h.uc.GetCheckById(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.log.Debug("check not found", slog.Int("id", id))
				responses.NotFound(c, "check not found")
			} else {
				h.log.Error("error get check by id", logger.Err(err))
				responses.Internal(c, "error get check by id")
			}
			return
		}

		responses.OK(c, GetCheckByIdResponse{
			Check: check,
		})
	}
}

// GetChecksByMarkId get check by mark id
//
//	@Summary		Get check by mark id
//	@Description	get check by mark id
//	@Tags			checks
//	@Produce		json
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.Response[checksrest.GetChecksByMarkIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/checks/mark/{id} [get]
func (h *handler) GetChecksByMarkId() gin.HandlerFunc {
	return func(c *gin.Context) {
		markId, err := strconv.Atoi(c.Param("markId"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		checks, err := h.uc.GetChecksByMarkId(c.Request.Context(), markId)
		if err != nil {
			h.log.Error("error get checks by mark id", logger.Err(err))
			responses.Internal(c, "error get checks by mark id")
			return
		}

		responses.OK(c, GetChecksByMarkIdResponse{
			Checks: checks,
		})
	}
}

// GetChecksByUserId get checks by user id
//
//	@Summary		List checks by user id
//	@Description	get checks by user id
//	@Tags			checks
//	@Produce		json
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.Response[checksrest.GetChecksByUserIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/checks/user/{id} [get]
func (h *handler) GetChecksByUserId() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, err := strconv.Atoi(c.Param("userId"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		checks, err := h.uc.GetChecksByUserId(c.Request.Context(), userId)
		if err != nil {
			h.log.Error("error get checks by user id", logger.Err(err))
			responses.Internal(c, "error get checks by user id")
			return
		}

		responses.OK(c, GetChecksByUserIdResponse{
			Checks: checks,
		})
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
//	@Success		201				{object}	responses.Response[checksrest.AddCheckResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/checks [post]
func (h *handler) AddCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AddCheckRequest
		if err := c.ShouldBind(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		photos, err := handlers.ParsePhotos(req.Photos)
		if err != nil {
			h.log.Error("error parse photos", logger.Err(err))
			responses.Internal(c, "error parse photos")
			return
		}

		claims := jwt.ExtractClaims(c)

		userIdStr, err := claims.GetSubject()
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}
		userId, err := strconv.Atoi(userIdStr)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		check := models.Check{
			UserID:  userId,
			MarkID:  req.MarkID,
			Result:  req.Result,
			Comment: req.Comment,
		}
		checkId, err := h.uc.AddCheck(c.Request.Context(), check, photos)
		if err != nil {
			switch err {
			case usecase.ErrNotFound:
				h.log.Debug("mark not found", slog.Int("mark_id", req.MarkID))
				responses.BadRequest(c, "mark not found")
				return
			case usecase.ErrConflict:
				h.log.Debug("user has already completed the check", slog.Int("mark_id", req.MarkID), slog.Int("user_id", userId))
				responses.Conflict(c, "user has already completed the check")
			default:
				h.log.Error("error add check", logger.Err(err))
				responses.Internal(c, "error add check")
			}
			return
		}

		h.log.Debug("Add mark", slog.Int("userId", userId))
		h.log.Info("add new check",
			slog.Int64("check_id", checkId),
			slog.Int("user_id", userId),
			slog.Int("mark_id", req.MarkID),
			slog.Bool("result", req.Result),
			slog.Int("photos", len(photos)),
		)
		responses.Created(c, AddCheckResponse{
			CheckId: int(checkId),
		})
	}
}
