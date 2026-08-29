// Package health exposes liveness and readiness endpoints.
package health

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

const (
	// PathLive is the liveness probe route.
	PathLive = "/healthz"
	// PathReady is the readiness probe route.
	PathReady = "/readyz"
)

// Checker reports the connectivity of infrastructure dependencies.
type Checker interface {
	Check(ctx context.Context) (usecase.HealthReport, error)
}

type handler struct {
	log     *slog.Logger
	checker Checker
}

// Register mounts GET /healthz and GET /readyz on the router.
func Register(r gin.IRouter, log *slog.Logger, checker Checker) {
	h := &handler{log: log, checker: checker}

	r.GET(PathLive, h.Healthz())
	r.GET(PathReady, h.Readyz())
}

// Healthz godoc
//
//	@Summary		Liveness probe
//	@Description	Always returns 200 while the process is running.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	responses.Response[map[string]string]
//	@Router			/healthz [get]
func (h *handler) Healthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		responses.OK(c, gin.H{"status": usecase.HealthStatusOK})
	}
}

// Readyz godoc
//
//	@Summary		Readiness probe
//	@Description	Pings every infrastructure dependency and reports their status. 503 when a required dependency (postgres) is down; optional ones (redis) are reported as "error" but do not affect readiness.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	responses.Response[usecase.HealthReport]
//	@Failure		503	{object}	responses.Response[usecase.HealthReport]
//	@Router			/readyz [get]
func (h *handler) Readyz() gin.HandlerFunc {
	return func(c *gin.Context) {
		report, err := h.checker.Check(c.Request.Context())
		if err == nil {
			responses.OK(c, report)
			return
		}

		responses.FailWithPayload(c, http.StatusServiceUnavailable, usecase.ErrUnavailable.Error(), report)
	}
}
