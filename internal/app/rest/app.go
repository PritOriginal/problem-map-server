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
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/internal/storage/local"
	"github.com/PritOriginal/problem-map-server/internal/storage/postgres"
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

	_, err = s3.New(log, cfg.Aws)
	if err != nil {
		log.Error("failed connection to s3", slogger.Err(err))
		panic(err)
	}
	log.Info("s3 connected!")

	accessAuth := jwtauth.New("HS256", []byte(cfg.Auth.JWT.Access.Key), nil)

	validate := validator.New()

	router := handler.GetRouter(log)

	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	mapRepo := postgres.NewMap(postgresDB.DB)
	photoRepo := local.NewPhotos()
	mapUseCase := usecase.NewMap(log, mapRepo, photoRepo)
	maprest.Register(router, accessAuth, mapUseCase, baseHandler)

	usersRepo := postgres.NewUsers(postgresDB.DB)
	usersUseCase := usecase.NewUsers(log, usersRepo)
	usersrest.Register(router, usersUseCase, baseHandler)

	authUseCase := usecase.NewAuth(log, usersRepo, cfg.Auth)
	authrest.Register(router, authUseCase, baseHandler)

	tasksRepo := postgres.NewTasks(postgresDB.DB)
	taksUseCase := usecase.NewTasks(log, tasksRepo)
	tasksrest.Register(router, taksUseCase, baseHandler)

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
