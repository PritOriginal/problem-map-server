// Package openrest serves the open-data endpoints: public aggregates that
// need no account and are cached for a few minutes.
package openrest

import (
	"context"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

// StatsCacheTTL is how long GET /open/stats is served from cache.
const StatsCacheTTL = 5 * time.Minute

type Stats interface {
	GetOpenStats(ctx context.Context, boundaryID int) (models.OpenStats, error)
}

type handler struct {
	log *slog.Logger
	uc  Stats
}

// Register mounts /open; middlewares (e.g. the optional API key) run before
// the cache so that every request is rate limited.
func Register(r *gin.Engine, log *slog.Logger, uc Stats, cacher mwcache.Cacher, middlewares ...gin.HandlerFunc) {
	handler := &handler{log: log, uc: uc}

	open := r.Group("/open", middlewares...)
	open.Use(mwcache.New(cacher, StatsCacheTTL))
	{
		open.GET("stats", handler.GetStats())
	}
}

// GetStats summarises the marks for the public
//
//	@Summary		Open statistics
//	@Description	public summary of the marks, optionally inside an admin boundary: totals, per-status and per-type counts, marks closed during the last 30 days and the mean closing time in hours. Cached for 5 minutes
//	@Tags			open
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Param			boundary_id	query		int	false	"only marks inside this admin boundary"
//	@Success		200			{object}	responses.Response[models.OpenStats]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		401			{object}	responses.Response[any]	"invalid, revoked or expired API key"
//	@Failure		429			{object}	responses.Response[any]	"API key quota exhausted"
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/open/stats [get]
func (h *handler) GetStats() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "openrest.GetStats"

		var req GetStatsRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}

		stats, err := h.uc.GetOpenStats(c.Request.Context(), req.BoundaryID)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, stats)
	}
}
