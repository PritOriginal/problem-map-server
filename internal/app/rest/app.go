package rest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/handler"
	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	checksrest "github.com/PritOriginal/problem-map-server/internal/handler/checks"
	"github.com/PritOriginal/problem-map-server/internal/handler/health"
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/middleware/ratelimit"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/repository/redis"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	jwt "github.com/appleboy/gin-jwt/v3"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

type App struct {
	server          *http.Server
	log             *slog.Logger
	closers         app.Closers
	shutdownTimeout time.Duration
	port            int
}

func New(log *slog.Logger, cfg *config.Config) *App {
	// Clients are registered in dependency order; app.Closers closes them in
	// reverse (s3 -> redis -> database).
	var closers app.Closers

	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		log.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	log.Info("PostgreSQL connected!")
	closers.Add("database", postgresDB)
	trManager := manager.Must(trmsqlx.NewDefaultFactory(postgresDB.DB))

	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		log.Error("failed connection to redis", slogger.Err(err))
		panic(err)
	}
	log.Info("Redis connected!")
	closers.Add("redis", redisClient)

	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Key: []byte(cfg.Auth.JWT.Access.Key),
	})
	if err != nil {
		log.Error("failed create auth middleware", slogger.Err(err))
		panic(err)
	}
	errInit := authMiddleware.MiddlewareInit()
	if errInit != nil {
		log.Error("failed init auth middleware", slogger.Err(errInit))
		panic(errInit)
	}

	router := handler.GetRouter(log, cfg.Env, cfg.REST.TrustedProxies, metrics.New())

	handler.SetSwagger(router)

	healthUseCase := usecase.NewHealth(log, cfg.Health, usecase.HealthDependencies{
		Required: map[string]usecase.Pinger{"postgres": postgresDB},
		// Cache and rate limiting fail open without Redis, so its loss is
		// reported but does not take the service out of rotation.
		Optional: map[string]usecase.Pinger{"redis": redisClient},
	})
	health.Register(router, log, healthUseCase)

	mapRepo := postgres.NewMap(postgresDB.DB, trmsqlx.DefaultCtxGetter)

	photoRepo, photoCloser := app.NewPhotosRepository(log, cfg)
	closers.Add("s3", photoCloser)

	mapUseCase := usecase.NewMap(log, usecase.MapRepositories{
		Map: mapRepo,
	})
	maprest.Register(router, log, mapUseCase, redisClient)

	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksRepo := postgres.NewChecks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	markStatusUpdater := usecase.NewUpdater(log, cfg.Rating, trManager, usecase.UpdaterRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Users:  usersRepo,
	})
	marksUseCase := usecase.NewMarks(log, trManager, usecase.MarksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Photos: photoRepo,
	})
	marksrest.Register(router, log, marksrest.Params{
		AuthMiddleware: authMiddleware,
		Cacher:         redisClient,
		Usecase:        marksUseCase,
		StatusUpdater:  markStatusUpdater,
	})

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksUseCase := usecase.NewChecks(log, cfg.Rating, trManager, markStatusUpdater, usecase.ChecksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Tasks:  tasksRepo,
		Photos: photoRepo,
		Users:  usersRepo,
	})
	checksrest.Register(router, log, authMiddleware, checksUseCase)

	usersUseCase := usecase.NewUsers(log, usecase.UsersRepositories{
		Users: usersRepo,
	})
	usersrest.Register(router, log, authMiddleware, usersUseCase)

	authUseCase := usecase.NewAuth(log, cfg.Auth, usecase.AuthRepositories{
		Users: usersRepo,
	})
	authRateLimit := ratelimit.New(log, redisClient, ratelimit.Config{
		Requests: cfg.REST.RateLimit.Requests,
		Window:   cfg.REST.RateLimit.Window,
	})
	authrest.Register(router, log, authUseCase, authRateLimit)

	tasksUseCase := usecase.NewTasks(log, usecase.TasksRepositories{
		Tasks: tasksRepo,
	})
	tasksrest.Register(router, log, authMiddleware, tasksUseCase)

	server := &http.Server{
		Addr:         cfg.REST.Host + ":" + strconv.Itoa(cfg.REST.Port),
		Handler:      router,
		ReadTimeout:  cfg.REST.Timeout.Read,
		WriteTimeout: cfg.REST.Timeout.Write,
		IdleTimeout:  cfg.REST.Timeout.Idle,
	}

	return &App{
		server:          server,
		log:             log,
		closers:         closers,
		shutdownTimeout: cfg.ShutdownTimeout,
		port:            cfg.REST.Port,
	}
}

// Run starts the HTTP server and blocks until it stops. A clean shutdown
// (http.ErrServerClosed) is reported as nil.
func (a *App) Run() error {
	const op = "rest.Run"

	a.log.Info("server started", slog.String("address", ":"+strconv.Itoa(a.port)))
	if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.log.Error("failed to start server", slogger.Err(err))
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// Stop gracefully shuts down the HTTP server and closes every infrastructure
// client (S3, Redis, PostgreSQL) in that order.
func (a *App) Stop() {
	const op = "rest.Stop"

	log := a.log.With(slog.String("op", op))
	log.Info("stopping REST server", slog.Int("port", a.port))

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer shutdownRelease()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		log.Error("an error occurred while stopping the server", slogger.Err(err))
	}

	a.closers.Close(log)
}
