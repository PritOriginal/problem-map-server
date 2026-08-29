// Package webhooksrest manages outgoing webhooks: subscriptions of
// moderators/admins to domain events delivered over HTTPS.
package webhooksrest

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

type Webhooks interface {
	Create(ctx context.Context, actor models.Actor, w models.Webhook) (models.Webhook, error)
	List(ctx context.Context, actor models.Actor) ([]models.Webhook, error)
	Update(ctx context.Context, actor models.Actor, id int, upd models.WebhookUpdate) (models.Webhook, error)
	Delete(ctx context.Context, actor models.Actor, id int) error
	ListDeliveries(ctx context.Context, actor models.Actor, id int, p models.Pagination) (models.Page[models.WebhookDelivery], error)
	SendTest(ctx context.Context, actor models.Actor, id int) (models.WebhookDelivery, error)
}

type handler struct {
	log *slog.Logger
	uc  Webhooks
}

// Register mounts /webhooks for moderators and admins.
func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Webhooks) {
	handler := &handler{log: log, uc: uc}

	webhooks := r.Group("/webhooks", authMiddleware.MiddlewareFunc(),
		middleware.RequireRole(models.RoleModerator, models.RoleAdmin))
	{
		webhooks.POST("", handler.CreateWebhook())
		webhooks.GET("", handler.GetWebhooks())
		id := webhooks.Group(":id")
		{
			id.PATCH("", handler.UpdateWebhook())
			id.DELETE("", handler.DeleteWebhook())
			id.GET("deliveries", handler.GetDeliveries())
			id.POST("test", handler.TestWebhook())
		}
	}
}

// actor builds the acting user from the validated JWT; it writes a 401 and
// returns false when the token carries no usable subject.
func (h *handler) actor(c *gin.Context) (models.Actor, bool) {
	userId, err := middleware.UserIDFromClaims(c)
	if err != nil {
		h.log.Debug("invalid token", logger.Err(err))
		responses.Unauthorized(c, "invalid token")
		return models.Actor{}, false
	}
	return models.Actor{UserID: userId, Role: middleware.RoleFromClaims(c)}, true
}

// CreateWebhook registers a webhook
//
//	@Summary		Create webhook
//	@Description	subscribe an https endpoint to domain events. The response carries the signing `secret` once; deliveries are signed with `X-Signature: sha256=<hex HMAC-SHA256(secret, body)>`. Private/loopback targets are rejected
//	@Tags			webhooks
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		webhooksrest.CreateWebhookRequest	true	"webhook"
//	@Success		201		{object}	responses.Response[webhooksrest.CreateWebhookResponse]
//	@Failure		400		{object}	responses.Response[any]	"invalid url (not https / private host) or unknown event"
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/webhooks [post]
func (h *handler) CreateWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "webhooksrest.CreateWebhook"

		actor, ok := h.actor(c)
		if !ok {
			return
		}

		var req CreateWebhookRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		webhook, err := h.uc.Create(c.Request.Context(), actor, req.Model())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("webhook created", slog.Int("webhook_id", webhook.ID), slog.Int("user_id", actor.UserID))
		responses.Created(c, CreateWebhookResponse{Webhook: webhook, Secret: webhook.Secret})
	}
}

// GetWebhooks lists the caller's webhooks
//
//	@Summary		List webhooks
//	@Description	webhooks owned by the current user
//	@Tags			webhooks
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	responses.Response[webhooksrest.GetWebhooksResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/webhooks [get]
func (h *handler) GetWebhooks() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "webhooksrest.GetWebhooks"

		actor, ok := h.actor(c)
		if !ok {
			return
		}

		webhooks, err := h.uc.List(c.Request.Context(), actor)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetWebhooksResponse{Webhooks: webhooks})
	}
}

// UpdateWebhook changes active and/or events
//
//	@Summary		Update webhook
//	@Description	enable/disable the webhook or change its events; the owner or an admin
//	@Tags			webhooks
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int									true	"webhook id"
//	@Param			request	body		webhooksrest.UpdateWebhookRequest	true	"fields to change"
//	@Success		200		{object}	responses.Response[webhooksrest.WebhookResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/webhooks/{id} [patch]
func (h *handler) UpdateWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "webhooksrest.UpdateWebhook"

		actor, ok := h.actor(c)
		if !ok {
			return
		}
		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req UpdateWebhookRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		webhook, err := h.uc.Update(c.Request.Context(), actor, id, req.Model())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, WebhookResponse{Webhook: webhook})
	}
}

// DeleteWebhook removes a webhook with its delivery log
//
//	@Summary		Delete webhook
//	@Description	delete the webhook; the owner or an admin
//	@Tags			webhooks
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"webhook id"
//	@Success		200	{object}	responses.Response[webhooksrest.DeleteWebhookResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/webhooks/{id} [delete]
func (h *handler) DeleteWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "webhooksrest.DeleteWebhook"

		actor, ok := h.actor(c)
		if !ok {
			return
		}
		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		if err := h.uc.Delete(c.Request.Context(), actor, id); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("webhook deleted", slog.Int("webhook_id", id), slog.Int("user_id", actor.UserID))
		responses.OK(c, DeleteWebhookResponse{WebhookId: id})
	}
}

// GetDeliveries lists the delivery log of a webhook
//
//	@Summary		List webhook deliveries
//	@Description	deliveries of the webhook, newest first, with attempt count, status and next retry; pagination info is in the top-level `meta` field
//	@Tags			webhooks
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int	true	"webhook id"
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[webhooksrest.GetDeliveriesResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/webhooks/{id}/deliveries [get]
func (h *handler) GetDeliveries() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "webhooksrest.GetDeliveries"

		actor, ok := h.actor(c)
		if !ok {
			return
		}
		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}
		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		page, err := h.uc.ListDeliveries(c.Request.Context(), actor, id, p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetDeliveriesResponse{Deliveries: page.Items}, p, page.Total)
	}
}

// TestWebhook sends a test event
//
//	@Summary		Test webhook
//	@Description	deliver a synthetic `webhook.test` event once (no retries) and report the outcome; works for inactive webhooks too
//	@Tags			webhooks
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"webhook id"
//	@Success		200	{object}	responses.Response[webhooksrest.TestWebhookResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/webhooks/{id}/test [post]
func (h *handler) TestWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "webhooksrest.TestWebhook"

		actor, ok := h.actor(c)
		if !ok {
			return
		}
		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		delivery, err := h.uc.SendTest(c.Request.Context(), actor, id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, TestWebhookResponse{Delivery: delivery})
	}
}
