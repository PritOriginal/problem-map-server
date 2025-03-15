package handler

import (
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/storage/db"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"
)

func GetRoute(log *slog.Logger, dbConn *sqlx.DB) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)

	mapRepo := db.NewMap(dbConn)
	mapUseCase := usecase.NewMap(mapRepo)
	mapHandler := NewMap(mapUseCase)

	r.Route("/map", func(r chi.Router) {
		// r.Get("/regions", Handler)
		r.Get("/districts", mapHandler.GetDistricts())
		r.Get("/marks", mapHandler.GetMarks())
		r.Post("/marks", mapHandler.AddMark())
		r.Post("/photos", mapHandler.AddPhotos())
	})

	return r
}
