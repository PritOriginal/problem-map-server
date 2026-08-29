package checksrest

import (
	"context"
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Checks interface {
	AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	ListChecksByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Check], error)
	ListChecksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Check], error)
}

type handler struct {
	log *slog.Logger
	uc  Checks
}

// Register mounts the routes. idempotency handles the Idempotency-Key
// header of POST /checks and may be nil.
func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Checks, idempotency gin.HandlerFunc) {
	handler := &handler{log: log, uc: uc}

	checks := r.Group("/checks")
	{
		checks.GET(":id", handler.GetCheckById())
		checks.GET("mark/:markId", handler.GetChecksByMarkId())
		checks.GET("user/:userId", handler.GetChecksByUserId())
		auth := checks.Group("", authMiddleware.MiddlewareFunc())
		{
			// The body limit must be in place before the idempotency
			// middleware reads the form to fingerprint it.
			create := auth.Group("", middleware.MaxBodySize(handlers.MaxUploadBodySize))
			if idempotency != nil {
				create.Use(idempotency)
			}
			create.POST("", handler.AddCheck())
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
		const op = "checksrest.GetCheckById"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		check, err := h.uc.GetCheckById(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
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
//	@Param			id		path		int	true	"mark id"
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[checksrest.GetChecksByMarkIdResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/checks/mark/{id} [get]
func (h *handler) GetChecksByMarkId() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "checksrest.GetChecksByMarkId"

		markId, err := handlers.ParamInt(c, "markId")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		page, err := h.uc.ListChecksByMarkId(c.Request.Context(), markId, p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetChecksByMarkIdResponse{Checks: page.Items}, p, page.Total)
	}
}

// GetChecksByUserId get checks by user id
//
//	@Summary		List checks by user id
//	@Description	get checks by user id
//	@Tags			checks
//	@Produce		json
//	@Param			id		path		int	true	"user id"
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[checksrest.GetChecksByUserIdResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/checks/user/{id} [get]
func (h *handler) GetChecksByUserId() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "checksrest.GetChecksByUserId"

		userId, err := handlers.ParamInt(c, "userId")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		page, err := h.uc.ListChecksByUserId(c.Request.Context(), userId, p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetChecksByUserIdResponse{Checks: page.Items}, p, page.Total)
	}
}

// AddCheck add check
//
//	@Summary		Add check
//	@Description	add check; the mark's author may not check their own mark (403), one check per voting stage (409), at most `rating.max-checks-per-day` checks per rolling 24 hours (429)
//	@Tags			checks
//	@Accept			mpfd
//	@Produce		json
//	@Security		BearerAuth
//	@Param			Idempotency-Key	header		string	false	"UUID chosen by the client; a repeat with the same key within 24h returns the stored response with `Idempotent-Replayed: true` (409 while the first request is in flight, 422 when reused with other form fields)"
//	@Success		201				{object}	responses.Response[checksrest.AddCheckResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		401				{object}	responses.Response[any]
//	@Failure		403				{object}	responses.Response[any]
//	@Failure		404				{object}	responses.Response[any]
//	@Failure		409				{object}	responses.Response[any]
//	@Failure		422				{object}	responses.Response[any]	"Idempotency-Key reused with a different payload"
//	@Failure		429				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/checks [post]
func (h *handler) AddCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "checksrest.AddCheck"

		var req AddCheckRequest
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

		check := models.Check{
			UserID:  userId,
			MarkID:  req.MarkID,
			Result:  req.Result,
			Comment: req.Comment,
		}
		checkId, err := h.uc.AddCheck(c.Request.Context(), check, photos)
		if err != nil {
			responses.FromError(c, h.log, op, err)
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
