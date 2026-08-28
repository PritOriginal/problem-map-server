package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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
	"github.com/PritOriginal/problem-map-server/internal/middleware/ratelimit"
	"github.com/PritOriginal/problem-map-server/internal/repository/local"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/repository/redis"
	"github.com/PritOriginal/problem-map-server/internal/repository/s3"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	jwt "github.com/appleboy/gin-jwt/v3"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/gin-gonic/gin"
)

type App struct {
	server *http.Server
	log    *slog.Logger
	db     *postgres.Postgres
	router *gin.Engine
	port   int
}

func New(log *slog.Logger, cfg *config.Config) *App {
	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		log.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	log.Info("PostgreSQL connected!")
	trManager := manager.Must(trmsqlx.NewDefaultFactory(postgresDB.DB))

	redis, err := redis.New(cfg.Redis)
	if err != nil {
		log.Error("failed connection to redis", slogger.Err(err))
		panic(err)
	}
	log.Info("Redis connected!")

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

	router := handler.GetRouter(log, cfg.Env)

	handler.SetSwagger(router, cfg)

	mapRepo := postgres.NewMap(postgresDB.DB, trmsqlx.DefaultCtxGetter)

	photoRepo := initPhotosRepository(log, cfg)

	mapUseCase := usecase.NewMap(log, usecase.MapRepositories{
		Map: mapRepo,
	})
	maprest.Register(router, log, mapUseCase, redis)

	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksRepo := postgres.NewChecks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	markStatusUpdater := usecase.NewUpdater(log, usecase.UpdaterRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
	})
	marksUseCase := usecase.NewMarks(log, trManager, usecase.MarksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Photos: photoRepo,
	})
	marksrest.Register(router, log, marksrest.Params{
		AuthMiddleware: authMiddleware,
		Cacher:         redis,
		Usecase:        marksUseCase,
		StatusUpdater:  markStatusUpdater,
	})

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksUseCase := usecase.NewChecks(log, trManager, markStatusUpdater, usecase.ChecksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Tasks:  tasksRepo,
		Photos: photoRepo,
	})
	checksrest.Register(router, log, authMiddleware, checksUseCase)

	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersUseCase := usecase.NewUsers(log, usecase.UsersRepositories{
		Users: usersRepo,
	})
	usersrest.Register(router, log, authMiddleware, usersUseCase)

	authUseCase := usecase.NewAuth(log, cfg.Auth, usecase.AuthRepositories{
		Users: usersRepo,
	})
	authRateLimit := ratelimit.New(log, redis, ratelimit.Config{
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
		server: server,
		log:    log,
		db:     postgresDB,
		router: router,
		port:   cfg.REST.Port,
	}
}

func initPhotosRepository(log *slog.Logger, cfg *config.Config) usecase.PhotosRepository {
	switch cfg.PhotoStorage {
	case config.S3:
		s3Client, err := s3.New(log, cfg.Aws)
		if err != nil {
			log.Error("failed connection to s3", slogger.Err(err))
			panic(err)
		}
		log.Info("s3 connected!")

		return s3.NewPhotos(s3Client)
	default:
		return local.NewPhotos()
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
