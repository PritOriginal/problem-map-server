package handler

import (
	"log/slog"

	_ "github.com/PritOriginal/problem-map-server/docs"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/middleware/requestid"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// GetRouter builds the base gin engine with request-id, logging, Prometheus
// metrics (exposed at GET /metrics) and recovery middleware.
func GetRouter(log *slog.Logger, env logger.Environment) *gin.Engine {
	r := gin.New()

	if env == logger.Prod {
		gin.SetMode(gin.ReleaseMode)
	}

	r.Use(requestid.New(log))

	if env == logger.Local {
		r.Use(gin.Logger())
	} else {
		r.Use(sloggin.NewWithConfig(log, sloggin.Config{
			DefaultLevel:      slog.LevelInfo,
			ClientErrorLevel:  slog.LevelWarn,
			ServerErrorLevel:  slog.LevelError,
			WithRequestID:     true,
			WithRequestHeader: false,
			Filters:           []sloggin.Filter{sloggin.IgnorePath("/healthz", "/readyz", "/metrics")},
		}))
	}

	m := metrics.New()
	r.Use(m.Middleware())
	r.Use(gin.Recovery())

	r.GET("/metrics", m.Handler())

	return r
}

func SetSwagger(r *gin.Engine, cfg *config.Config) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
