package marksrest

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/twpayne/go-geom"
)

type Marks interface {
	ListMarks(ctx context.Context, filters models.GetMarksFilters) (models.Page[models.Mark], error)
	GetMarksNearby(ctx context.Context, filters models.GetMarksNearbyFilters) (models.Page[models.MarkWithDistance], error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	ListMarksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error)
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
		marks.GET("nearby", handler.GetMarksNearby())
		id := marks.Group(":id")
		{
			id.GET("", handler.GetMarkById())
			id.GET("status-history", handler.GetMarkStatusHistoryByMarkId())
			moderation := id.Group("", params.AuthMiddleware.MiddlewareFunc(),
				middleware.RequireRole(models.RoleModerator, models.RoleAdmin))
			{
				moderation.POST("confirm", handler.Confirm())
				moderation.POST("reject", handler.Reject())
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

// GetMarks lists markers matching the filters, paginated
//
//	@Summary		List markers
//	@Description	get markers page; pagination info is returned in the top-level `meta` field ({limit, offset, total})
//	@Tags			marks
//	@Accept			json
//	@Produce		json
//	@Param			mark_type_ids	query		string	false	"filter by mark types, comma-separated ids"
//	@Param			mark_status_ids	query		string	false	"filter by mark statuses, comma-separated ids"
//	@Param			user_id			query		int		false	"filter by author"
//	@Param			bbox			query		string	false	"bounding box minLon,minLat,maxLon,maxLat (WGS84)"
//	@Param			created_from	query		string	false	"created_at >= (RFC3339)"
//	@Param			created_to		query		string	false	"created_at <= (RFC3339)"
//	@Param			sort			query		string	false	"sort column"		Enums(created_at, updated_at)	default(created_at)
//	@Param			order			query		string	false	"sort order"		Enums(asc, desc)				default(desc)
//	@Param			limit			query		int		false	"page size, 1..500"	default(100)
//	@Param			offset			query		int		false	"page offset"		default(0)
//	@Success		200				{object}	responses.Response[marksrest.GetMarksResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/marks [get]
func (h *handler) GetMarks() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.GetMarks"

		var req GetMarksRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		page, err := h.uc.ListMarks(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetMarksResponse{Marks: page.Items}, filters.Pagination, page.Total)
	}
}

// GetMarksNearby lists markers within a radius of a point, nearest first
//
//	@Summary		List nearby markers
//	@Description	get markers within `radius` meters of (lon, lat) ordered by distance; each item carries `distance_m`; pagination info is in the top-level `meta` field
//	@Tags			marks
//	@Produce		json
//	@Param			lon				query		number	true	"longitude"
//	@Param			lat				query		number	true	"latitude"
//	@Param			radius			query		number	true	"radius in meters, at most 50000"
//	@Param			mark_type_ids	query		string	false	"filter by mark types, comma-separated ids"
//	@Param			mark_status_ids	query		string	false	"filter by mark statuses, comma-separated ids"
//	@Param			limit			query		int		false	"page size, 1..500"	default(100)
//	@Param			offset			query		int		false	"page offset"		default(0)
//	@Success		200				{object}	responses.Response[marksrest.GetMarksNearbyResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/marks/nearby [get]
func (h *handler) GetMarksNearby() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.GetMarksNearby"

		var req GetMarksNearbyRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		page, err := h.uc.GetMarksNearby(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetMarksNearbyResponse{Marks: page.Items}, filters.Pagination, page.Total)
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
		const op = "marksrest.GetMarkById"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		mark, err := h.uc.GetMarkById(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
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
//	@Param			id		path		int	true	"user id"
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[marksrest.GetMarksByUserIdResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/marks/user/{id} [get]
func (h *handler) GetMarksByUserId() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.GetMarksByUserId"

		userId, err := handlers.ParamInt(c, "userId")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		page, err := h.uc.ListMarksByUserId(c.Request.Context(), userId, p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetMarksByUserIdResponse{Marks: page.Items}, p, page.Total)
	}
}

// AddMark add mark
//
//	@Summary		Add mark
//	@Description	add mark. Coordinates are WGS84: `longitude` is X, `latitude` is Y (GeoJSON/PostGIS order)
//	@Tags			marks
//	@Accept			mpfd
//	@Produce		json
//	@Security		BearerAuth
//	@Param			photos			formData	file	true	"Photos of the problem"
//	@Param			longitude		formData	number	true	"Longitude in degrees (X), WGS84"	example(41.44)
//	@Param			latitude		formData	number	true	"Latitude in degrees (Y), WGS84"	example(52.72)
//	@Param			mark_type_id	formData	int		true	"Mark type id"
//	@Param			description		formData	string	false	"Description (max 256 chars)"
//	@Success		201				{object}	responses.Response[marksrest.AddMarkResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		401				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/marks [post]
func (h *handler) AddMark() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.AddMark"

		var req AddMarkRequest
		if err := c.ShouldBind(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		photos, err := handlers.ParsePhotos(req.Photos)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		userId, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		newMark := models.Mark{
			Geom:        models.NewPoint(geom.Coord{req.Longitude, req.Latitude}),
			MarkTypeID:  req.MarkTypeID,
			UserID:      userId,
			Description: req.Description,
		}
		markId, err := h.uc.AddMark(c.Request.Context(), newMark, photos)
		if err != nil {
			responses.FromError(c, h.log, op, err)
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
		const op = "marksrest.GetMarkTypes"

		types, err := h.uc.GetMarkTypes(c.Request.Context())

		if err != nil {
			responses.FromError(c, h.log, op, err)
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
		const op = "marksrest.GetMarkStatuses"

		statuses, err := h.uc.GetMarkStatuses(c.Request.Context())

		if err != nil {
			responses.FromError(c, h.log, op, err)
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
		const op = "marksrest.GetMarkStatusHistoryByMarkId"

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
			responses.FromError(c, h.log, op, err)
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
//	@Security		BearerAuth
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.Response[marksrest.ConfirmResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		409	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/{id}/confirm [post]
func (h *handler) Confirm() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.Confirm"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		newStatusId, err := h.statusUpdater.Confirm(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
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
//	@Security		BearerAuth
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.Response[marksrest.RejectResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		409	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/{id}/reject [post]
func (h *handler) Reject() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.Reject"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		newStatus, err := h.statusUpdater.Reject(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("mark status has been updated", slog.Int("mark_id", id), slog.Int("new_mark_status_id", int(newStatus)))
		responses.OK(c, RejectResponse{
			NewMarkStausId: newStatus,
		})
	}
}
