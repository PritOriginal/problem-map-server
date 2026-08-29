// Package syncrest serves GET /users/me/sync: the personal changes a
// client missed while offline, in one request.
package syncrest

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Sync interface {
	GetUserSync(ctx context.Context, userId int, filters models.UserSyncFilters) (models.UserSync, error)
}

type handler struct {
	log *slog.Logger
	uc  Sync
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Sync) {
	handler := &handler{log: log, uc: uc}

	me := r.Group("/users/me", authMiddleware.MiddlewareFunc())
	{
		me.GET("sync", handler.GetUserSync())
	}
}

// GetUserSync returns the personal changes since an instant
//
//	@Summary		Personal changes since an instant
//	@Description	one request for a client coming back online: the caller's tasks updated after `since`, unread notifications received after it and checks submitted after it. Each collection is paged independently by limit/offset; `totals` carry the full counts. Store `server_time` and pass it as the next `since`; with several server instances subtract a safety margin (`server_time - 1s`) to cover clock skew between them. `since` in the future is rejected with 400
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			since	query		string	true	"RFC3339 instant of the previous sync"
//	@Param			limit	query		int		false	"page size of each collection, 1..500"	default(100)
//	@Param			offset	query		int		false	"page offset of each collection"		default(0)
//	@Success		200		{object}	responses.Response[syncrest.GetUserSyncResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/users/me/sync [get]
func (h *handler) GetUserSync() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "syncrest.GetUserSync"

		userId, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		var req GetUserSyncRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		sync, err := h.uc.GetUserSync(c.Request.Context(), userId, filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, NewGetUserSyncResponse(sync))
	}
}
