package handler

import (
	"log/slog"

	mwLogger "github.com/PritOriginal/problem-map-server/internal/middleware/logger"
	"github.com/PritOriginal/problem-map-server/internal/storage/local"
	"github.com/PritOriginal/problem-map-server/internal/storage/postgres"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"
)

func GetRoute(log *slog.Logger, dbConn *sqlx.DB) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(mwLogger.New(log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)

	mapRepo := postgres.NewMap(dbConn)
	photoRepo := local.NewPhotos()
	mapUseCase := usecase.NewMap(log, mapRepo, photoRepo)
	mapHandler := NewMap(log, mapUseCase)

	r.Route("/map", func(r chi.Router) {
		r.Get("/regions", mapHandler.GetRegions())
		r.Get("/cities", mapHandler.GetCities())
		r.Get("/districts", mapHandler.GetDistricts())
		r.Get("/marks", mapHandler.GetMarks())
		r.Post("/marks", mapHandler.AddMark())
		r.Post("/photos", mapHandler.AddPhotos())
	})

	usersRepo := postgres.NewUsers(dbConn)
	usersUseCase := usecase.NewUsers(usersRepo)
	usersHandler := NewUsers(log, usersUseCase)

	r.Route("/users", func(r chi.Router) {
		r.Get("/", usersHandler.GetUsers())
		r.Get("/{id}", usersHandler.GetUserById())
		r.Post("/", usersHandler.AddUser())
	})

	tasksRepo := postgres.NewTasks(dbConn)
	taksUseCase := usecase.NewTasks(tasksRepo)
	tasksHandler := NewTasks(log, taksUseCase)

	r.Route("/tasks", func(r chi.Router) {
		r.Get("/", tasksHandler.GetTasks())
		r.Get("/{id}", tasksHandler.GetTaskById())
		r.Get("/user/{id}", tasksHandler.GetTasksByUserId())
		r.Post("/", tasksHandler.AddTask())
	})

	return r
}
