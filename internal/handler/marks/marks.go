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
	"github.com/PritOriginal/problem-map-server/internal/usecase"
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
			auth := id.Group("", params.AuthMiddleware.MiddlewareFunc())
			{
				auth.POST("confirm", handler.Confirm())
				auth.POST("reject", handler.Reject())
			}
		}
		marks.GET("user/:userId", handler.GetMarksByUserId())
		auth := marks.Group("", params.AuthMiddleware.MiddlewareFunc())
		{
			auth.POST("", handler.AddMark())
		}
		cache := marks.Group("")
		cache.Use(mwcache.New(params.Cacher, 24*time.Hour))
		{
			cache.GET("types", handler.GetMarkTypes())
			cache.GET("statuses", handler.GetMarkStatuses())
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
	return func(c *gin.Context) {
		markTypeIdsStr := c.Query("mark_type_ids")
		markTypeIds, err := handlers.ParseIntArray(markTypeIdsStr)
		if err != nil {
			h.log.Debug("failed parse mark type ids", logger.Err(err))
			responses.BadRequest(c, "failed parse mark type ids")
			return
		}
		markStatusIdsStr := c.Query("mark_status_ids")
		markStatusIds, err := handlers.ParseIntArray(markStatusIdsStr)
		if err != nil {
			h.log.Debug("failed parse mark status ids", logger.Err(err))
			responses.BadRequest(c, "failed parse mark status ids")
			return
		}

		marks, err := h.uc.GetMarks(c.Request.Context(), models.GetMarksFilters{
			MarkTypeIds:   markTypeIds,
			MarkStatusIds: markStatusIds,
		})
		if err != nil {
			h.log.Error("error get marks", logger.Err(err))
			responses.Internal(c, "error get marks")
			return
		}

		responses.OK(c, GetMarksResponse{
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
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		mark, err := h.uc.GetMarkById(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.log.Debug("mark not found", slog.Int("id", id))
				responses.NotFound(c, "mark not found")
			} else {
				h.log.Error("error get mark by id", slog.Int("id", id), logger.Err(err))
				responses.Internal(c, "error get mark by id")
			}
			return
		}

		responses.OK(c, GetMarkByIdResponse{
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
	return func(c *gin.Context) {
		userId, err := strconv.Atoi(c.Param("userId"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		marks, err := h.uc.GetMarksByUserId(c.Request.Context(), userId)
		if err != nil {
			h.log.Error("error get marks by user id", slog.Int("user_id", userId), logger.Err(err))
			responses.Internal(c, "error get marks by user id")
			return
		}

		responses.OK(c, GetMarksByUserIdResponse{
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
	return func(c *gin.Context) {
		var req AddMarkRequest
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

		newMark := models.Mark{
			Geom:        models.NewPoint(geom.Coord{req.Latitude, req.Longitude}),
			MarkTypeID:  req.MarkTypeID,
			UserID:      userId,
			Description: req.Description,
		}
		markId, err := h.uc.AddMark(c.Request.Context(), newMark, photos)
		if err != nil {
			h.log.Error("error add mark", logger.Err(err))
			responses.Internal(c, "error add mark")
			return
		}

		h.log.Info("add new mark",
			slog.Int64("mark_id", markId),
			slog.Int("user_id", userId),
			slog.Float64("longitude", req.Longitude),
			slog.Float64("latitude", req.Latitude),
			slog.Int("photos", len(photos)),
		)
		responses.Created(c, AddMarkResponse{
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
	return func(c *gin.Context) {
		types, err := h.uc.GetMarkTypes(c.Request.Context())

		if err != nil {
			h.log.Error("error get mark types", logger.Err(err))
			responses.Internal(c, "error get mark types")
			return
		}

		responses.OK(c, GetMarkTypesResponse{
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
	return func(c *gin.Context) {
		statuses, err := h.uc.GetMarkStatuses(c.Request.Context())

		if err != nil {
			h.log.Error("error get mark statuses", logger.Err(err))
			responses.Internal(c, "error get mark statuses")
			return
		}

		responses.OK(c, GetMarkStatusesResponse{
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
	return func(c *gin.Context) {
		var req GetMarkStatusHistoryByMarkIdRequest
		if err := c.ShouldBindUri(&req); err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		if err := c.ShouldBindQuery(&req); err != nil {
			h.log.Debug("failed parse query params", logger.Err(err))
			responses.BadRequest(c, "failed parse query params")
			return
		}

		historyItems, err := h.uc.GetMarkStatusHistoryByMarkId(c.Request.Context(), req.MarkId, req.WithChecks)
		if err != nil {
			h.log.Error("error get mark status history", slog.Int("mark_id", req.MarkId), logger.Err(err))
			responses.Internal(c, "error get mark status history")
			return
		}

		responses.OK(c, GetMarkStatusHistoryByMarkIdResponse{
			HistoryItems: historyItems,
		})
	}
}

// Confirm сonfirm the mark and moves it to a new status
//
//	@Summary		Confirm the mark
//	@Description	сonfirm the mark and moves it to a new status
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[marksrest.ConfirmResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		409	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/{id}/confirm [post]
func (h *handler) Confirm() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		newStatusId, err := h.statusUpdater.Confirm(c.Request.Context(), id)
		if err != nil {
			switch err {
			case usecase.ErrConflict:
				h.log.Debug("unable to update the mark status", slog.Int("mark_id", id))
				responses.Conflict(c, "user already exists")
			default:
				h.log.Error("error confirm mark", slog.Int("mark_id", id), logger.Err(err))
				responses.Internal(c, "error confirm mark")
			}
			return
		}

		h.log.Info("mark status has been updated", slog.Int("mark_id", id), slog.Int("new_mark_status_id", int(newStatusId)))
		responses.OK(c, ConfirmResponse{
			NewMarkStausId: newStatusId,
		})
	}
}

// Reject reject the mark and moves it to a new status
//
//	@Summary		Reject the mark
//	@Description	reject the mark and moves it to a new status
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[marksrest.RejectResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		409	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/{id}/reject [post]
func (h *handler) Reject() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		newStatus, err := h.statusUpdater.Reject(c.Request.Context(), id)
		if err != nil {
			switch err {
			case usecase.ErrConflict:
				h.log.Debug("unable to update the mark status", slog.Int("mark_id", id))
				responses.Conflict(c, "user already exists")
			default:
				h.log.Error("error confirm mark", slog.Int("mark_id", id), logger.Err(err))
				responses.Internal(c, "error confirm mark")
			}
			return
		}

		h.log.Info("mark status has been updated", slog.Int("mark_id", id), slog.Int("new_mark_status_id", int(newStatus)))
		responses.OK(c, RejectResponse{
			NewMarkStausId: newStatus,
		})
	}
}
