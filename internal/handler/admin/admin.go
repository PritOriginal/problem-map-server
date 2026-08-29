// Package adminrest exposes the administration endpoints: the runtime
// settings (voting threshold, rating deltas, tasker limits) with their change
// history and the mark type dictionary. Every route requires the admin role.
package adminrest

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Settings interface {
	Load(ctx context.Context) (usecase.RuntimeSettings, error)
	Update(ctx context.Context, s usecase.RuntimeSettings, updatedBy int) (usecase.RuntimeSettings, error)
	History(ctx context.Context, limit int) ([]models.SettingChange, error)
}

type MarkTypes interface {
	List(ctx context.Context, lang models.Lang) ([]models.MarkType, error)
	Create(ctx context.Context, t models.MarkTypeCreate, lang models.Lang) (models.MarkType, error)
	Update(ctx context.Context, id int, upd models.MarkTypeUpdate, lang models.Lang) (models.MarkType, error)
}

type handler struct {
	log       *slog.Logger
	settings  Settings
	markTypes MarkTypes
}

type Params struct {
	AuthMiddleware *jwt.GinJWTMiddleware
	Settings       Settings
	MarkTypes      MarkTypes
}

func Register(r *gin.Engine, log *slog.Logger, params Params) {
	h := &handler{log: log, settings: params.Settings, markTypes: params.MarkTypes}

	admin := r.Group("/admin", params.AuthMiddleware.MiddlewareFunc(), middleware.RequireRole(models.RoleAdmin))
	{
		admin.GET("settings", h.GetSettings())
		admin.PUT("settings", h.UpdateSettings())
		admin.GET("settings/history", h.GetSettingsHistory())

		admin.GET("mark-types", h.GetMarkTypes())
		admin.POST("mark-types", h.CreateMarkType())
		admin.PATCH("mark-types/:id", h.UpdateMarkType())
	}
}

// GetSettings returns the runtime settings
//
//	@Summary		Get runtime settings
//	@Description	current values of the admin-editable settings (database, or the config defaults until the first PUT)
//	@Tags			admin
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	responses.Response[adminrest.SettingsResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/admin/settings [get]
func (h *handler) GetSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "adminrest.GetSettings"

		s, err := h.settings.Load(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, SettingsResponse{Settings: s})
	}
}

// UpdateSettings replaces the runtime settings
//
//	@Summary		Update runtime settings
//	@Description	stores the full settings document (every field is required) after range validation; the change is applied within 30 s on every instance
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		adminrest.UpdateSettingsRequest	true	"settings"
//	@Success		200		{object}	responses.Response[adminrest.SettingsResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/admin/settings [put]
func (h *handler) UpdateSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "adminrest.UpdateSettings"

		var req UpdateSettingsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		userId, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		s, err := h.settings.Update(c.Request.Context(), *req.Settings, userId)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, SettingsResponse{Settings: s})
	}
}

// GetSettingsHistory lists the latest changes of the runtime settings
//
//	@Summary		Settings change history
//	@Description	latest changes of the runtime settings, newest first
//	@Tags			admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int	false	"page size (1..100)"	default(20)
//	@Success		200		{object}	responses.Response[adminrest.SettingsHistoryResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/admin/settings/history [get]
func (h *handler) GetSettingsHistory() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "adminrest.GetSettingsHistory"

		var req HistoryRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}

		changes, err := h.settings.History(c.Request.Context(), req.Limit)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, SettingsHistoryResponse{Changes: changes})
	}
}

// GetMarkTypes lists every mark type
//
//	@Summary		List mark types (admin)
//	@Description	all mark types, inactive ones included, sorted by sort_order and name
//	@Tags			admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			Accept-Language	header		string	false	"ru (default) or en"
//	@Success		200				{object}	responses.Response[adminrest.MarkTypesResponse]
//	@Failure		401				{object}	responses.Response[any]
//	@Failure		403				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/admin/mark-types [get]
func (h *handler) GetMarkTypes() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "adminrest.GetMarkTypes"

		ctx := c.Request.Context()
		types, err := h.markTypes.List(ctx, models.LangFromContext(ctx))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, MarkTypesResponse{MarkTypes: types})
	}
}

// CreateMarkType adds a mark type
//
//	@Summary		Create mark type
//	@Description	adds a mark type with its Russian and optional English name; 409 when the code is taken
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		adminrest.CreateMarkTypeRequest	true	"mark type"
//	@Success		201		{object}	responses.Response[adminrest.MarkTypeResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/admin/mark-types [post]
func (h *handler) CreateMarkType() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "adminrest.CreateMarkType"

		var req CreateMarkTypeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		ctx := c.Request.Context()
		t, err := h.markTypes.Create(ctx, req.Model(), models.LangFromContext(ctx))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.Created(c, MarkTypeResponse{MarkType: t})
	}
}

// UpdateMarkType changes a mark type
//
//	@Summary		Update mark type
//	@Description	changes the given fields of a mark type (names, icon, color, SLA, active, sort_order); omitted fields stay unchanged
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int								true	"mark type id"
//	@Param			request	body		adminrest.UpdateMarkTypeRequest	true	"fields to change"
//	@Success		200		{object}	responses.Response[adminrest.MarkTypeResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/admin/mark-types/{id} [patch]
func (h *handler) UpdateMarkType() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "adminrest.UpdateMarkType"

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			responses.BadRequest(c, "invalid id")
			return
		}

		var req UpdateMarkTypeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		ctx := c.Request.Context()
		t, err := h.markTypes.Update(ctx, id, req.Model(), models.LangFromContext(ctx))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, MarkTypeResponse{MarkType: t})
	}
}
