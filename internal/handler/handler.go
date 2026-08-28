package handler

import (
	"log/slog"

	_ "github.com/PritOriginal/problem-map-server/docs"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
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
	// MaxBodySize is the maximum size of a request body accepted by the server.
	// It is large enough for handlers.MaxPhotos photos of handlers.MaxPhotoSize.
	MaxBodySize = 40 << 20 // 40 MiB
)

func GetRouter(log *slog.Logger, env logger.Environment) *gin.Engine {
	r := gin.New()
	r.MaxMultipartMemory = MaxMultipartMemory

	if env == logger.Prod {
		gin.SetMode(gin.ReleaseMode)
	}

	if env == logger.Local {
		r.Use(gin.Logger())
	} else {
		r.Use(sloggin.New(log))
	}

	r.Use(gin.Recovery())
	r.Use(middleware.MaxBodySize(MaxBodySize))

	return r
}

func SetSwagger(r *gin.Engine, cfg *config.Config) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
