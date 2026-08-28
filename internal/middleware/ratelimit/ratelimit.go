// Package ratelimit implements a fixed-window per-IP rate limiter backed by Redis.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

// Counter increments the counter stored under key and returns the new value.
// When the key is created its TTL is set to window.
type Counter interface {
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
}

// Config describes the limiter: Requests per Window per client IP.
type Config struct {
	Requests int
	Window   time.Duration
}

// New returns a middleware limiting requests per client IP.
// When the counter backend is unavailable the middleware fails open
// and lets the request through.
func New(log *slog.Logger, counter Counter, cfg Config) gin.HandlerFunc {
	if cfg.Requests <= 0 || cfg.Window <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), c.ClientIP())

		count, err := counter.Incr(c.Request.Context(), key, cfg.Window)
		if err != nil {
			log.Warn("rate limiter unavailable, failing open", logger.Err(err))
			c.Next()
			return
		}

		if count > int64(cfg.Requests) {
			c.Header("Retry-After", strconv.Itoa(int(cfg.Window.Seconds())))
			responses.Fail(c, http.StatusTooManyRequests, "too many requests")
			c.Abort()
			return
		}

		c.Next()
	}
}
