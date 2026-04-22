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
	GetGroupedChecksByMarkStatusHistoryId(ctx context.Context, markId int) ([]models.GroupedChecksByMarkStatusHistoryId, error)
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
	return func(ctx *gin.Context) {
		id, err := strconv.Atoi(ctx.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(ctx, "failed parse id")
			return
		}

		check, err := h.uc.GetCheckById(ctx.Request.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.log.Debug("check not found", slog.Int("id", id))
				responses.NotFound(ctx, "check not found")
			} else {
				h.log.Error("error get check by id", logger.Err(err))
				responses.Internal(ctx, "error get check by id")
			}
			return
		}

		responses.OK(ctx, GetCheckByIdResponse{
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
	return func(ctx *gin.Context) {
		markId, err := strconv.Atoi(ctx.Param("markId"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(ctx, "failed parse id")
			return
		}

		checks, err := h.uc.GetChecksByMarkId(ctx.Request.Context(), markId)
		if err != nil {
			h.log.Error("error get checks by mark id", logger.Err(err))
			responses.Internal(ctx, "error get checks by mark id")
			return
		}

		responses.OK(ctx, GetChecksByMarkIdResponse{
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
	return func(ctx *gin.Context) {
		userId, err := strconv.Atoi(ctx.Param("userId"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(ctx, "failed parse id")
			return
		}

		checks, err := h.uc.GetChecksByUserId(ctx.Request.Context(), userId)
		if err != nil {
			h.log.Error("error get checks by user id", logger.Err(err))
			responses.Internal(ctx, "error get checks by user id")
			return
		}

		responses.OK(ctx, GetChecksByUserIdResponse{
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
	return func(ctx *gin.Context) {
		var req AddCheckRequest
		if err := ctx.ShouldBind(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(ctx, "invalid request")
			return
		}

		photos, err := handlers.ParsePhotos(req.Photos)
		if err != nil {
			h.log.Error("error parse photos", logger.Err(err))
			responses.Internal(ctx, "error parse photos")
			return
		}

		claims := jwt.ExtractClaims(ctx)

		userIdStr, err := claims.GetSubject()
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(ctx, "invalid token")
			return
		}
		userId, err := strconv.Atoi(userIdStr)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(ctx, "invalid token")
			return
		}

		check := models.Check{
			UserID:  userId,
			MarkID:  req.MarkID,
			Result:  req.Result,
			Comment: req.Comment,
		}
		checkId, err := h.uc.AddCheck(ctx.Request.Context(), check, photos)
		if err != nil {
			switch err {
			case usecase.ErrNotFound:
				h.log.Debug("mark not found", slog.Int("mark_id", req.MarkID))
				responses.BadRequest(ctx, "mark not found")
				return
			case usecase.ErrConflict:
				h.log.Debug("user has already completed the check", slog.Int("mark_id", req.MarkID), slog.Int("user_id", userId))
				responses.Conflict(ctx, "user has already completed the check")
			default:
				h.log.Error("error add check", logger.Err(err))
				responses.Internal(ctx, "error add check")
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
		responses.Created(ctx, AddCheckResponse{
			CheckId: int(checkId),
		})
	}
}
