package handler

import (
	"log/slog"

	_ "github.com/PritOriginal/problem-map-server/docs"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/handler/health"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/middleware/requestid"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// GetRouter builds the base gin engine with request-id, logging, Prometheus
// metrics (exposed at GET /metrics) and recovery middleware. Probe and
// metrics routes are excluded from both the access log and the metrics.
func GetRouter(log *slog.Logger, env logger.Environment, m *metrics.Metrics) *gin.Engine {
	r := gin.New()

	if env == logger.Prod {
		gin.SetMode(gin.ReleaseMode)
	}

	quietPaths := []string{health.PathLive, health.PathReady, metrics.Path}

	r.Use(requestid.New())

	if env == logger.Local {
		r.Use(gin.Logger())
	} else {
		cfg := sloggin.DefaultConfig()
		cfg.WithRequestID = true
		cfg.Filters = []sloggin.Filter{sloggin.IgnorePath(quietPaths...)}
		r.Use(sloggin.NewWithConfig(log, cfg))
	}

	r.Use(m.Middleware(quietPaths...))
	r.Use(gin.Recovery())

	r.GET(metrics.Path, m.Handler())

	return r
}

func SetSwagger(r *gin.Engine, cfg *config.Config) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
