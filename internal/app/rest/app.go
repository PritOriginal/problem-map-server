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
	analyticsrest "github.com/PritOriginal/problem-map-server/internal/handler/analytics"
	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	checksrest "github.com/PritOriginal/problem-map-server/internal/handler/checks"
	"github.com/PritOriginal/problem-map-server/internal/handler/health"
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	notificationsrest "github.com/PritOriginal/problem-map-server/internal/handler/notifications"
	organizationsrest "github.com/PritOriginal/problem-map-server/internal/handler/organizations"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	webhooksrest "github.com/PritOriginal/problem-map-server/internal/handler/webhooks"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/middleware/idempotency"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/middleware/ratelimit"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/repository/redis"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
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
	// reverse (nats -> s3 -> redis -> database).
	var closers app.Closers

	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		log.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	log.Info("PostgreSQL connected!")
	closers.Add("database", postgresDB)
	trManager := manager.Must(trmsqlx.NewDefaultFactory(postgresDB.DB))

	// Redis is optional: the cache, the rate limiter, the refresh-token
	// store and the auth-version check all fail open without it. The client
	// reconnects on its own, and readyz reports "redis: error" meanwhile.
	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		log.Error("failed connection to redis, continuing without it", slogger.Err(err))
	} else {
		log.Info("Redis connected!")
	}
	closers.Add("redis", redisClient)

	// One cache of auth versions is shared by the middleware and the
	// usecases, so a bump made here is seen by the middleware at once.
	authVersions := middleware.NewVersionCache(redisClient, 0)
	authMiddleware, err := middleware.NewJWT(log, middleware.JWTParams{
		Key:      cfg.Auth.JWT.Access.Key,
		Versions: authVersions,
	})
	if err != nil {
		log.Error("failed create auth middleware", slogger.Err(err))
		panic(err)
	}

	m := metrics.New()
	router := handler.GetRouter(log, cfg.Env, cfg.REST.TrustedProxies, m)

	// Idempotency-Key support of the mutating routes; fails open without Redis.
	idempotencyMiddleware := idempotency.New(log, redisClient, idempotency.Config{
		TTL:     cfg.REST.Idempotency.TTL,
		LockTTL: cfg.REST.Idempotency.LockTTL,
	})

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

	publisher, publisherCloser := app.NewPublisher(log, cfg.Nats, m.Registry())
	closers.Add("nats", publisherCloser)

	mapUseCase := usecase.NewMap(log, usecase.MapRepositories{
		Map: mapRepo,
	})
	maprest.Register(router, log, mapUseCase, redisClient)

	analyticsRepo := postgres.NewAnalytics(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	analyticsUseCase := usecase.NewAnalytics(log, usecase.AnalyticsRepositories{
		Analytics: analyticsRepo,
	})
	analyticsrest.Register(router, log, analyticsUseCase)

	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksRepo := postgres.NewChecks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	organizationsRepo := postgres.NewOrganizations(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	organizationsUseCase := usecase.NewOrganizations(log, trManager, usecase.OrganizationsRepositories{
		Organizations: organizationsRepo,
		Marks:         marksRepo,
		Checks:        checksRepo,
		Photos:        photoRepo,
		Users:         usersRepo,
		RefreshTokens: redisClient,
		AuthVersions:  authVersions,
	}).WithEvents(publisher)
	organizationsrest.Register(router, log, authMiddleware, organizationsUseCase)

	// Confirmed marks are handed to the responsible city service.
	markStatusUpdater := usecase.NewUpdater(log, cfg.Rating, trManager, usecase.UpdaterRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Users:  usersRepo,
	}).WithEvents(publisher).WithAssigner(organizationsUseCase)
	marksUseCase := usecase.NewMarks(log, cfg.Marks, trManager, usecase.MarksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Photos: photoRepo,
	})
	exportUseCase := usecase.NewExport(log, cfg.Export, usecase.ExportRepositories{
		Marks: marksRepo,
	})
	marksrest.Register(router, log, marksrest.Params{
		AuthMiddleware: authMiddleware,
		Cacher:         redisClient,
		Usecase:        marksUseCase,
		StatusUpdater:  markStatusUpdater,
		Exporter:       exportUseCase,
		ExportRateLimit: ratelimit.New(log, redisClient, ratelimit.Config{
			Requests: cfg.Export.RateLimit.Requests,
			Window:   cfg.Export.RateLimit.Window,
		}),
		Idempotency: idempotencyMiddleware,
	})

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksUseCase := usecase.NewChecks(log, cfg.Rating, trManager, markStatusUpdater, usecase.ChecksRepositories{
		Marks:         marksRepo,
		Checks:        checksRepo,
		Tasks:         tasksRepo,
		Photos:        photoRepo,
		Users:         usersRepo,
		Organizations: organizationsRepo,
	}).WithEvents(publisher)
	checksrest.Register(router, log, authMiddleware, checksUseCase, idempotencyMiddleware)

	usersUseCase := usecase.NewUsers(log, usecase.UsersRepositories{
		Users:         usersRepo,
		RefreshTokens: redisClient,
		AuthVersions:  authVersions,
	})
	usersrest.Register(router, log, authMiddleware, usersUseCase)

	authUseCase := usecase.NewAuth(log, cfg.Auth, usecase.AuthRepositories{
		Users:         usersRepo,
		RefreshTokens: redisClient,
		AuthVersions:  authVersions,
	})
	authRateLimit := ratelimit.New(log, redisClient, ratelimit.Config{
		Requests: cfg.REST.RateLimit.Requests,
		Window:   cfg.REST.RateLimit.Window,
	})
	authrest.Register(router, log, authUseCase, authMiddleware, authRateLimit)

	tasksUseCase := usecase.NewTasks(log, usecase.TasksRepositories{
		Tasks: tasksRepo,
	}).WithEvents(publisher)
	tasksrest.Register(router, log, authMiddleware, tasksUseCase)

	notificationsRepo := postgres.NewNotifications(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	// The REST server only stores manual data and reads notifications; push
	// delivery happens in cmd/notifier, hence no PushSender here.
	notificationsUseCase := usecase.NewNotifications(log, nil, usecase.NotificationsRepositories{
		Notifications: notificationsRepo,
		Devices:       notificationsRepo,
	})
	notificationsrest.Register(router, log, authMiddleware, notificationsUseCase)

	// The REST server only manages webhooks and serves the test delivery;
	// events are delivered by cmd/notifier.
	webhookSender, webhookURLs := app.NewWebhookSender(log, cfg.Webhooks)
	webhooksUseCase := usecase.NewWebhooks(log, usecase.WebhooksDeps{
		Sender: webhookSender,
		URLs:   webhookURLs,
	}, usecase.WebhooksRepositories{
		Webhooks: postgres.NewWebhooks(postgresDB.DB, trmsqlx.DefaultCtxGetter),
	})
	webhooksrest.Register(router, log, authMiddleware, webhooksUseCase)

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
