package handler

import (
	"log/slog"

	_ "github.com/PritOriginal/problem-map-server/docs"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/handler/health"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/middleware/requestid"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

const (
	// MaxMultipartMemory is the amount of multipart form data kept in memory;
	// the rest is spilled to temporary files.
	MaxMultipartMemory = 32 << 20 // 32 MiB
	// MaxBodySize is the maximum size of a request body accepted by the server:
	// handlers.MaxPhotos photos of handlers.MaxPhotoSize plus room for the
	// remaining form fields and multipart framing.
	MaxBodySize = handlers.MaxPhotos*handlers.MaxPhotoSize + 1<<20
)

// GetRouter builds the base router. trustedProxies lists the CIDRs/IPs of
// reverse proxies whose X-Forwarded-For header may be used to derive the
// client IP; with an empty list the remote address is used as is.
// Prometheus metrics are exposed at GET /metrics; probe and metrics routes
// are excluded from both the access log and the metrics.
func GetRouter(log *slog.Logger, env logger.Environment, trustedProxies []string, m *metrics.Metrics) *gin.Engine {
	r := gin.New()
	r.MaxMultipartMemory = MaxMultipartMemory
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		log.Error("failed set trusted proxies", logger.Err(err))
		panic(err)
	}

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
	r.Use(middleware.MaxBodySize(MaxBodySize))

	r.GET(metrics.Path, m.Handler())

	return r
}

func SetSwagger(r *gin.Engine, cfg *config.Config) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
