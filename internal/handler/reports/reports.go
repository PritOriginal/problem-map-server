// Package reportsrest serves user reports (POST /reports) and the
// moderation queue (/moderation/...).
package reportsrest

import (
	"context"
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

type Reports interface {
	Create(ctx context.Context, report models.Report) (models.Report, error)
	ListQueue(ctx context.Context, filters models.GetReportsFilters) (models.Page[models.ReportWithTarget], error)
	ListMine(ctx context.Context, reporterId int, p models.Pagination) (models.Page[models.Report], error)
	Resolve(ctx context.Context, actor models.Actor, id int, status models.ReportStatus) (models.Report, error)
}

type handler struct {
	log *slog.Logger
	uc  Reports
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Reports) {
	handler := &handler{log: log, uc: uc}

	reports := r.Group("/reports", authMiddleware.MiddlewareFunc())
	{
		reports.POST("", handler.CreateReport())
	}

	moderation := r.Group("/moderation", authMiddleware.MiddlewareFunc())
	{
		moderation.GET("reports/mine", handler.GetMyReports())

		moderators := moderation.Group("", middleware.RequireRole(models.RoleModerator, models.RoleAdmin))
		{
			moderators.GET("queue", handler.GetQueue())
			moderators.PATCH("reports/:id", handler.ResolveReport())
		}
	}
}

// actorFromClaims builds the acting user from the validated JWT; it writes a
// 401 and returns false when the token carries no usable subject.
func (h *handler) actorFromClaims(c *gin.Context) (models.Actor, bool) {
	userId, err := middleware.UserIDFromClaims(c)
	if err != nil {
		h.log.Debug("invalid token", logger.Err(err))
		responses.Unauthorized(c, "invalid token")
		return models.Actor{}, false
	}
	return models.Actor{UserID: userId, Role: middleware.RoleFromClaims(c)}, true
}

// CreateReport files a report on a mark, a check or a comment
//
//	@Summary		Report content
//	@Description	file a complaint about a mark, a check or a comment. A mark or a check must exist (404) and must not be the reporter's own (403); a comment is accepted by id only. One report per target per user (409 on repeat); at most `reports.max-per-day` reports per user in 24 hours (429). When the open reports on a mark reach `reports.hide-threshold` the mark is hidden automatically
//	@Tags			moderation
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		reportsrest.CreateReportRequest	true	"report"
//	@Success		201		{object}	responses.Response[reportsrest.CreateReportResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		429		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/reports [post]
func (h *handler) CreateReport() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "reportsrest.CreateReport"

		var req CreateReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		report, err := h.uc.Create(c.Request.Context(), req.Model(actor.UserID))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("report created",
			slog.Int("report_id", report.ID),
			slog.Int("user_id", actor.UserID),
			slog.String("target_type", string(report.TargetType)),
			slog.Int("target_id", report.TargetID),
		)
		responses.Created(c, CreateReportResponse{Report: report})
	}
}

// GetQueue lists reports for moderators
//
//	@Summary		Moderation queue
//	@Description	page of reports (open by default, oldest first) with their targets: for a mark report `target.mark` is the short form of the mark (including hidden ones); pagination info is in the top-level `meta` field
//	@Tags			moderation
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status		query		string	false	"report status"		Enums(open, resolved, dismissed)	default(open)
//	@Param			target_type	query		string	false	"target type"		Enums(mark, check, comment)
//	@Param			limit		query		int		false	"page size, 1..500"	default(100)
//	@Param			offset		query		int		false	"page offset"		default(0)
//	@Success		200			{object}	responses.Response[reportsrest.GetQueueResponse]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		401			{object}	responses.Response[any]
//	@Failure		403			{object}	responses.Response[any]
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/moderation/queue [get]
func (h *handler) GetQueue() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "reportsrest.GetQueue"

		var req GetQueueRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters := req.Filters()

		page, err := h.uc.ListQueue(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetQueueResponse{Reports: page.Items}, filters.Pagination, page.Total)
	}
}

// GetMyReports lists the current user's reports
//
//	@Summary		My reports
//	@Description	page of the reports filed by the current user, oldest first; pagination info is in the top-level `meta` field
//	@Tags			moderation
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[reportsrest.GetMyReportsResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/moderation/reports/mine [get]
func (h *handler) GetMyReports() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "reportsrest.GetMyReports"

		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		page, err := h.uc.ListMine(c.Request.Context(), actor.UserID, p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetMyReportsResponse{Reports: page.Items}, p, page.Total)
	}
}

// ResolveReport decides an open report
//
//	@Summary		Decide report
//	@Description	set the final status of an open report: `resolved` (the complaint was justified) or `dismissed`. A report that is already decided is 409
//	@Tags			moderation
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int									true	"report id"
//	@Param			request	body		reportsrest.ResolveReportRequest	true	"decision"
//	@Success		200		{object}	responses.Response[reportsrest.ResolveReportResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/moderation/reports/{id} [patch]
func (h *handler) ResolveReport() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "reportsrest.ResolveReport"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req ResolveReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		report, err := h.uc.Resolve(c.Request.Context(), actor, id, models.ReportStatus(req.Status))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, ResolveReportResponse{Report: report})
	}
}
