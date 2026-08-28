// Package health exposes liveness and readiness endpoints.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/gin-gonic/gin"
)

// Pinger is implemented by infrastructure clients that can verify connectivity.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Dependencies lists the named dependencies checked by the readiness probe.
// A nil Pinger is reported as "ok" (dependency not configured).
type Dependencies map[string]Pinger

const (
	StatusOK    = "ok"
	StatusError = "error"

	readyTimeout = 3 * time.Second
)

type handler struct {
	log  *slog.Logger
	deps Dependencies
}

// Register mounts GET /healthz and GET /readyz on the router.
func Register(r gin.IRouter, log *slog.Logger, deps Dependencies) {
	h := &handler{log: log, deps: deps}

	r.GET("/healthz", h.Healthz())
	r.GET("/readyz", h.Readyz())
}

// Healthz is the liveness probe; it always returns 200.
func (h *handler) Healthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": StatusOK})
	}
}

// Readyz is the readiness probe; it pings every dependency and returns 503
// with a per-dependency report when at least one of them fails.
func (h *handler) Readyz() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyTimeout)
		defer cancel()

		report := make(map[string]string, len(h.deps))
		status := http.StatusOK

		for name, p := range h.deps {
			report[name] = StatusOK
			if p == nil {
				continue
			}
			if err := p.Ping(ctx); err != nil {
				h.log.Warn("readiness check failed", slog.String("dependency", name), slogger.Err(err))
				report[name] = StatusError
				status = http.StatusServiceUnavailable
			}
		}

		c.JSON(status, report)
	}
}
