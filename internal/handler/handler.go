package handler

import (
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/docs"
	"github.com/PritOriginal/problem-map-server/internal/config"
	mwLogger "github.com/PritOriginal/problem-map-server/internal/middleware/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
)

func GetRouter(log *slog.Logger) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(mwLogger.New(log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)

	return r
}

func SetSwagger(r *chi.Mux, cfg *config.RESTConfig) {
	docs.SwaggerInfo.Host = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	docs.SwaggerInfo.BasePath = "/"
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("http://%s:%d/swagger/doc.json", cfg.Host, cfg.Port)),
	))
}
