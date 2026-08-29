// Package notificationsrest serves the current user's in-app notifications
// and push-device registration.
package notificationsrest

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

type Notifications interface {
	List(ctx context.Context, userId int, filters models.GetNotificationsFilters) (models.Page[models.Notification], error)
	UnreadCount(ctx context.Context, userId int) (int, error)
	MarkRead(ctx context.Context, userId int, id int) error
	MarkAllRead(ctx context.Context, userId int) (int64, error)
	RegisterDevice(ctx context.Context, device models.UserDevice) (models.UserDevice, error)
	DeleteDevice(ctx context.Context, userId int, token string) error
}

type handler struct {
	log *slog.Logger
	uc  Notifications
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Notifications) {
	handler := &handler{log: log, uc: uc}

	notifications := r.Group("/notifications", authMiddleware.MiddlewareFunc())
	{
		notifications.GET("", handler.GetNotifications())
		notifications.GET("unread-count", handler.GetUnreadCount())
		notifications.PATCH("read-all", handler.MarkAllRead())
		notifications.PATCH(":id/read", handler.MarkRead())
	}

	devices := r.Group("/users/me/devices", authMiddleware.MiddlewareFunc())
	{
		devices.POST("", handler.RegisterDevice())
		devices.DELETE(":token", handler.DeleteDevice())
	}
}

// userID extracts the authenticated user; on failure it writes a 401 and
// returns false.
func (h *handler) userID(c *gin.Context) (int, bool) {
	id, err := middleware.UserIDFromClaims(c)
	if err != nil {
		h.log.Debug("invalid token", logger.Err(err))
		responses.Unauthorized(c, "invalid token")
		return 0, false
	}
	return id, true
}

// GetNotifications lists the current user's notifications, paginated
//
//	@Summary		List my notifications
//	@Description	get notifications of the authenticated user, newest first; pagination info is returned in the top-level `meta` field ({limit, offset, total})
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			unread	query		bool	false	"only unread notifications"
//	@Param			limit	query		int		false	"page size, 1..500"	default(100)
//	@Param			offset	query		int		false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[notificationsrest.GetNotificationsResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/notifications [get]
func (h *handler) GetNotifications() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "notificationsrest.GetNotifications"

		userId, ok := h.userID(c)
		if !ok {
			return
		}

		var req GetNotificationsRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters := req.Filters()

		page, err := h.uc.List(c.Request.Context(), userId, filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetNotificationsResponse{Notifications: page.Items}, filters.Pagination, page.Total)
	}
}

// GetUnreadCount returns the number of unread notifications
//
//	@Summary		Count my unread notifications
//	@Description	get the number of unread notifications of the authenticated user
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	responses.Response[notificationsrest.UnreadCountResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/notifications/unread-count [get]
func (h *handler) GetUnreadCount() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "notificationsrest.GetUnreadCount"

		userId, ok := h.userID(c)
		if !ok {
			return
		}

		count, err := h.uc.UnreadCount(c.Request.Context(), userId)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, UnreadCountResponse{Count: count})
	}
}

// MarkRead marks one notification as read
//
//	@Summary		Mark notification as read
//	@Description	mark the notification of the authenticated user as read (idempotent)
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"notification id"
//	@Success		200	{object}	responses.Response[any]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/notifications/{id}/read [patch]
func (h *handler) MarkRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "notificationsrest.MarkRead"

		userId, ok := h.userID(c)
		if !ok {
			return
		}

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		if err := h.uc.MarkRead(c.Request.Context(), userId, id); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK[any](c, nil)
	}
}

// MarkAllRead marks every unread notification as read
//
//	@Summary		Mark all notifications as read
//	@Description	mark every unread notification of the authenticated user as read
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	responses.Response[notificationsrest.MarkAllReadResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/notifications/read-all [patch]
func (h *handler) MarkAllRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "notificationsrest.MarkAllRead"

		userId, ok := h.userID(c)
		if !ok {
			return
		}

		updated, err := h.uc.MarkAllRead(c.Request.Context(), userId)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, MarkAllReadResponse{Updated: updated})
	}
}

// RegisterDevice registers a push token of the current user
//
//	@Summary		Register push device
//	@Description	register (upsert) a push token of the authenticated user; a token already registered is moved to this user
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		notificationsrest.RegisterDeviceRequest	true	"device"
//	@Success		200		{object}	responses.Response[notificationsrest.RegisterDeviceResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/users/me/devices [post]
func (h *handler) RegisterDevice() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "notificationsrest.RegisterDevice"

		userId, ok := h.userID(c)
		if !ok {
			return
		}

		var req RegisterDeviceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		device, err := h.uc.RegisterDevice(c.Request.Context(), models.UserDevice{
			UserID:   userId,
			Platform: req.Platform,
			Token:    req.Token,
		})
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, RegisterDeviceResponse{Device: device})
	}
}

// DeleteDevice removes a push token of the current user
//
//	@Summary		Delete push device
//	@Description	remove the push token of the authenticated user
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			token	path		string	true	"push token"
//	@Success		200		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/users/me/devices/{token} [delete]
func (h *handler) DeleteDevice() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "notificationsrest.DeleteDevice"

		userId, ok := h.userID(c)
		if !ok {
			return
		}

		if err := h.uc.DeleteDevice(c.Request.Context(), userId, c.Param("token")); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK[any](c, nil)
	}
}
