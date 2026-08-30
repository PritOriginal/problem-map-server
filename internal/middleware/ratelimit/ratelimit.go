// Package ratelimit implements a fixed-window rate limiter backed by Redis:
// per client IP (New) or per an arbitrary key such as an API key (Allow).
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

// Counter increments the counter stored under key and returns the new value
// together with the time left until the key expires. When the key is created
// its TTL is set to window.
type Counter interface {
	Incr(ctx context.Context, key string, window time.Duration) (count int64, ttl time.Duration, err error)
}

// Config describes the limiter: Requests per Window per client.
type Config struct {
	Requests int
	Window   time.Duration
	// Headers adds X-RateLimit-Limit / -Remaining / -Reset to every
	// response that went through the limiter.
	Headers bool
}

// New returns a middleware limiting requests per client IP.
// When the counter backend is unavailable the middleware fails open
// and lets the request through.
func New(log *slog.Logger, counter Counter, cfg Config) gin.HandlerFunc {
	if cfg.Requests <= 0 || cfg.Window <= 0 || counter == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), c.ClientIP())
		if !Allow(c, log, counter, key, cfg) {
			return
		}

		c.Next()
	}
}

// Allow counts the request under key and reports whether it may continue.
// Over the limit it answers 429 with Retry-After and aborts the context;
// the caller then just returns. A disabled config or an unavailable
// backend lets the request through (fail open).
func Allow(c *gin.Context, log *slog.Logger, counter Counter, key string, cfg Config) bool {
	if cfg.Requests <= 0 || cfg.Window <= 0 || counter == nil {
		return true
	}

	count, ttl, err := counter.Incr(c.Request.Context(), key, cfg.Window)
	if err != nil {
		log.Warn("rate limiter unavailable, failing open", logger.Err(err))
		return true
	}

	retryAfter := retryAfterSeconds(ttl, cfg.Window)
	if cfg.Headers {
		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.Requests))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(max(int64(cfg.Requests)-count, 0), 10))
		c.Header("X-RateLimit-Reset", strconv.Itoa(retryAfter))
	}

	if count > int64(cfg.Requests) {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		responses.Fail(c, http.StatusTooManyRequests, "too many requests")
		c.Abort()
		return false
	}

	return true
}

// retryAfterSeconds converts the remaining window TTL into a Retry-After
// value: rounded up to whole seconds, at least 1, and never more than the
// full window (a backend reporting no TTL falls back to the window).
func retryAfterSeconds(ttl, window time.Duration) int {
	if ttl <= 0 || ttl > window {
		ttl = window
	}
	return int(math.Ceil(ttl.Seconds()))
}
