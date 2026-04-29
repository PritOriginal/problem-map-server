package marksrest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"time"

	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/twpayne/go-geom"
)

type Marks interface {
	GetMarks(ctx context.Context, filters models.GetMarksFilters) ([]models.Mark, error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark, photos []io.Reader) (int64, error)
	GetMarkTypes(ctx context.Context) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error)
	GetMarkStatusHistoryByMarkId(ctx context.Context, markId int, withChecks bool) ([]models.MarkStatusHistoryItem, error)
}

type StatusUpdater interface {
	Confirm(ctx context.Context, markId int) (models.MarkStatusType, error)
	Reject(ctx context.Context, markId int) (models.MarkStatusType, error)
}

type handler struct {
	log           *slog.Logger
	uc            Marks
	statusUpdater StatusUpdater
}

type Params struct {
	AuthMiddleware *jwt.GinJWTMiddleware
	Cacher         mwcache.Cacher
	Usecase        Marks
	StatusUpdater  StatusUpdater
}

func Register(r *gin.Engine, log *slog.Logger, params Params) {
	handler := &handler{
		log:           log,
		uc:            params.Usecase,
		statusUpdater: params.StatusUpdater,
	}

	marks := r.Group("/marks")
	{
		marks.GET("", handler.GetMarks())
		id := marks.Group(":id")
		{
			id.GET("", handler.GetMarkById())
			id.GET("status-history", handler.GetMarkStatusHistoryByMarkId())
		}
		marks.GET("user/:userId", handler.GetMarksByUserId())
		auth := marks.Group("", authMiddleware.MiddlewareFunc())
		{
			auth.POST("", handler.AddMark())
		}
		cache := marks.Group("")
		cache.Use(mwcache.New(cacher, 24*time.Hour))
		{
			cache.GET("/types", handler.GetMarkTypes())
			cache.GET("/statuses", handler.GetMarkStatuses())
		}
	}
}

// GetMarks lists all existing markers
//
//	@Summary		List markers
//	@Description	get markers
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Param			mark_type_ids	query		[]number	false	"filter by mark types"
//	@Param			mark_status_ids	query		[]number	false	"filter by mark statuses"
//	@Success		200				{object}	responses.Response[marksrest.GetMarksResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/marks [get]
func (h *handler) GetMarks() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		markTypeIdsStr := ctx.Query("mark_type_ids")
		markTypeIds, err := handlers.ParseIntArray(markTypeIdsStr)
		if err != nil {
			h.log.Debug("failed parse mark type ids", logger.Err(err))
			responses.BadRequest(ctx, "failed parse mark type ids")
			return
		}
		markStatusIdsStr := ctx.Query("mark_status_ids")
		markStatusIds, err := handlers.ParseIntArray(markStatusIdsStr)
		if err != nil {
			h.log.Debug("failed parse mark status ids", logger.Err(err))
			responses.BadRequest(ctx, "failed parse mark status ids")
			return
		}

		marks, err := h.uc.GetMarks(ctx.Request.Context(), models.GetMarksFilters{
			MarkTypeIds:   markTypeIds,
			MarkStatusIds: markStatusIds,
		})
		if err != nil {
			h.log.Error("error get marks", logger.Err(err))
			responses.Internal(ctx, "error get marks")
			return
		}

		responses.OK(ctx, GetMarksResponse{
			Marks: marks,
		})
	}
}

// GetMarkById get mark by id
//
//	@Summary		Get mark by id
//	@Description	get mark by id
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.Response[marksrest.GetMarkByIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/{id} [get]
func (h *handler) GetMarkById() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		id, err := strconv.Atoi(ctx.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(ctx, "failed parse id")
			return
		}

		mark, err := h.uc.GetMarkById(ctx.Request.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.log.Debug("mark not found", slog.Int("id", id))
				responses.NotFound(ctx, "mark not found")
			} else {
				h.log.Error("error get mark by id", slog.Int("id", id), logger.Err(err))
				responses.Internal(ctx, "error get mark by id")
			}
			return
		}

		responses.OK(ctx, GetMarkByIdResponse{
			Mark: mark,
		})
	}
}

// GetMarkById List markers by user id
//
//	@Summary		List markers by user id
//	@Description	get markers by user id
//	@Tags			marks
//	@Produce		json
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.Response[marksrest.GetMarksByUserIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/user/{id} [get]
func (h *handler) GetMarksByUserId() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId, err := strconv.Atoi(ctx.Param("userId"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(ctx, "failed parse id")
			return
		}

		marks, err := h.uc.GetMarksByUserId(ctx.Request.Context(), userId)
		if err != nil {
			h.log.Error("error get marks by user id", slog.Int("user_id", userId), logger.Err(err))
			responses.Internal(ctx, "error get marks by user id")
			return
		}

		responses.OK(ctx, GetMarksByUserIdResponse{
			Marks: marks,
		})
	}
}

// AddMark add mark
//
//	@Summary		Add mark
//	@Description	add mark
//	@Tags			marks
//	@Accept			mpfd
//	@Produce		json
//	@Param			Authorization	header		string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Success		201				{object}	responses.Response[marksrest.AddMarkResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/marks [post]
func (h *handler) AddMark() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req AddMarkRequest
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

		newMark := models.Mark{
			Geom:        models.NewPoint(geom.Coord{req.Latitude, req.Longitude}),
			MarkTypeID:  req.MarkTypeID,
			UserID:      userId,
			Description: req.Description,
		}
		markId, err := h.uc.AddMark(ctx.Request.Context(), newMark, photos)
		if err != nil {
			h.log.Error("error add mark", logger.Err(err))
			responses.Internal(ctx, "error add mark")
			return
		}

		h.log.Info("add new mark",
			slog.Int64("mark_id", markId),
			slog.Int("user_id", userId),
			slog.Float64("longitude", req.Longitude),
			slog.Float64("latitude", req.Latitude),
			slog.Int("photos", len(photos)),
		)
		responses.Created(ctx, AddMarkResponse{
			MarkId: int(markId),
		})
	}
}

// GetMarkTypes lists all existing mark types
//
//	@Summary		List mark types
//	@Description	get mark types
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[marksrest.GetMarkTypesResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/types [get]
func (h *handler) GetMarkTypes() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		types, err := h.uc.GetMarkTypes(ctx.Request.Context())

		if err != nil {
			h.log.Error("error get mark types", logger.Err(err))
			responses.Internal(ctx, "error get mark types")
			return
		}

		responses.OK(ctx, GetMarkTypesResponse{
			MarkTypes: types,
		})
	}
}

// GetMarkStatuses lists all existing mark statuses
//
//	@Summary		List mark statuses
//	@Description	get mark statuses
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[marksrest.GetMarkStatusesResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/statuses [get]
func (h *handler) GetMarkStatuses() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		statuses, err := h.uc.GetMarkStatuses(ctx.Request.Context())

		if err != nil {
			h.log.Error("error get mark statuses", logger.Err(err))
			responses.Internal(ctx, "error get mark statuses")
			return
		}

		responses.OK(ctx, GetMarkStatusesResponse{
			MarkStatuses: statuses,
		})
	}
}

// GetMarkStatusHistoryByMarkId displays the entire list of status changes history
//
//	@Summary		List mark statuses
//	@Description	displays the entire list of status changes history for a specific marker by markId
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Param			id			path		int		true	"mark id"
//	@Param			withChecks	query		boolean	false	"with checks"
//	@Success		200			{object}	responses.Response[marksrest.GetMarkStatusHistoryByMarkIdResponse]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/marks/{id}/status-history [get]
func (h *handler) GetMarkStatusHistoryByMarkId() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req GetMarkStatusHistoryByMarkIdRequest
		if err := ctx.ShouldBindUri(&req); err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(ctx, "failed parse id")
			return
		}

		if err := ctx.ShouldBindQuery(&req); err != nil {
			h.log.Debug("failed parse query params", logger.Err(err))
			responses.BadRequest(ctx, "failed parse query params")
			return
		}

		historyItems, err := h.uc.GetMarkStatusHistoryByMarkId(ctx.Request.Context(), req.MarkId, req.WithChecks)
		if err != nil {
			h.log.Error("error get mark status history", slog.Int("mark_id", req.MarkId), logger.Err(err))
			responses.Internal(ctx, "error get mark status history")
			return
		}

		responses.OK(ctx, GetMarkStatusHistoryByMarkIdResponse{
			HistoryItems: historyItems,
		})
	}
}
