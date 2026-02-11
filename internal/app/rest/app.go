package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/handler"
	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	checksrest "github.com/PritOriginal/problem-map-server/internal/handler/checks"
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/internal/storage/local"
	"github.com/PritOriginal/problem-map-server/internal/storage/postgres"
	"github.com/PritOriginal/problem-map-server/internal/storage/redis"
	"github.com/PritOriginal/problem-map-server/internal/storage/s3"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/docgen"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-playground/validator/v10"
)

type App struct {
	server *http.Server
	log    *slog.Logger
	db     *postgres.Postgres
	router *chi.Mux
	port   int
}

func New(log *slog.Logger, cfg *config.Config) *App {
	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		log.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	log.Info("PostgreSQL connected!")

	redis, err := redis.New(cfg.Redis)
	if err != nil {
		log.Error("failed connection to redis", slogger.Err(err))
		panic(err)
	}
	log.Info("Redis connected!")

	accessAuth := jwtauth.New("HS256", []byte(cfg.Auth.JWT.Access.Key), nil)

	validate := validator.New()

	router := handler.GetRouter(log)

	handler.SetSwagger(router, &cfg.REST)

	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	mapRepo := postgres.NewMap(postgresDB.DB)

	var photoRepo usecase.PhotosRepository
	switch cfg.PhotoStorage {
	case config.Local:
		photoRepo = local.NewPhotos()
	case config.S3:
		s3Client, err := s3.New(log, cfg.Aws)
		if err != nil {
			log.Error("failed connection to s3", slogger.Err(err))
			panic(err)
		}
		log.Info("s3 connected!")

		photoRepo = s3.NewPhotos(s3Client)
	}

	mapUseCase := usecase.NewMap(log, usecase.MapRepositories{
		Map: mapRepo,
	})
	maprest.Register(router, mapUseCase, redis, baseHandler)

	marksRepo := postgres.NewMarks(postgresDB.DB)
	checksRepo := postgres.NewChecks(postgresDB.DB)
	marksUseCase := usecase.NewMarks(log, marksRepo, checksRepo, photoRepo)
	marksrest.Register(router, accessAuth, marksUseCase, redis, baseHandler)

	markStatusUpdater := usecase.NewUpdater(log, usecase.UpdaterRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
	})

	checksUseCase := usecase.NewChecks(log, markStatusUpdater, usecase.ChecksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Photos: photoRepo,
	})
	checksrest.Register(router, accessAuth, checksUseCase, baseHandler)

	usersRepo := postgres.NewUsers(postgresDB.DB)
	usersUseCase := usecase.NewUsers(log, usersRepo)
	usersrest.Register(router, usersUseCase, baseHandler)

	authUseCase := usecase.NewAuth(log, cfg.Auth, usecase.AuthRepositories{
		Users: usersRepo,
	})
	authrest.Register(router, authUseCase, baseHandler)

	tasksRepo := postgres.NewTasks(postgresDB.DB)
	tasksUseCase := usecase.NewTasks(log, tasksRepo)
	tasksrest.Register(router, tasksUseCase, baseHandler)

	server := &http.Server{
		Addr:         cfg.REST.Host + ":" + strconv.Itoa(cfg.REST.Port),
		Handler:      router,
		ReadTimeout:  cfg.REST.Timeout.Read,
		WriteTimeout: cfg.REST.Timeout.Write,
		IdleTimeout:  cfg.REST.Timeout.Idle,
	}

	return &App{
		server: server,
		log:    log,
		db:     postgresDB,
		router: router,
		port:   cfg.REST.Port,
	}
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil {
		panic(err)
	}
}

func (a *App) Run() error {
	const op = "rest.Run"

	a.log.Info("server started", slog.String("address", ":"+strconv.Itoa(a.port)))
	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		a.log.Error("failed to start server")
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (a *App) Stop() {
	const op = "rest.Stop"

	a.log.With(slog.String("op", op)).
		Info("stopping REST server", slog.Int("port", a.port))

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.log.Error("an error occurred while stopping the server", slogger.Err(err))
	}

	if err := a.db.DB.Close(); err != nil {
		a.log.Error("an error occurred while closing the connection to the database", slogger.Err(err))
	}
}

func (a *App) MustGenerateRoutesDoc() {
	if err := a.GenerateRoutesDoc(); err != nil {
		panic(err)
	}
}

func (a *App) GenerateRoutesDoc() error {
	data := docgen.MarkdownRoutesDoc(a.router, docgen.MarkdownOpts{
		ProjectPath: "github.com/PritOriginal/problem-map-server",
		Intro:       "REST generated docs.",
	})

	err := os.WriteFile("routes.md", []byte(data), 0644)

	return err
}
