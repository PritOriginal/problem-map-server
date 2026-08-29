package grpcapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/config"
	grpcauth "github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	mapgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/map"
	marksgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/marks"
	tasksgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/tasks"
	usersgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/users"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/repository/redis"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// healthCheckInterval is how often dependencies are re-checked to update the
// gRPC health service status.
const healthCheckInterval = 5 * time.Second

// protectedMethods require an authenticated caller.
var protectedMethods = []string{
	pb.Marks_AddMark_FullMethodName,
	pb.Tasks_AddTask_FullMethodName,
}

// moderationMethods additionally require the moderator or admin role.
var moderationMethods = []string{
	pb.Tasks_AddTask_FullMethodName,
}

type App struct {
	gRPCServer *grpc.Server
	health     *health.Server
	healthUC   *usecase.Health
	log        *slog.Logger
	closers    app.Closers
	// metricsServer serves the Prometheus registry over HTTP; nil when
	// cfg.GRPC.MetricsPort is 0.
	metricsServer   *http.Server
	shutdownTimeout time.Duration
	port            int

	// backgroundCtx/stopBackground bound the health-check loop started by
	// Run. They are created in New so that Stop is safe to call at any time,
	// including before or concurrently with Run.
	backgroundCtx  context.Context
	stopBackground context.CancelFunc
	// backgroundMu serialises "start the loop" against "cancel the loop" so
	// Stop never returns from background.Wait before Run registers the loop.
	backgroundMu sync.Mutex
	background   sync.WaitGroup
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

	// Redis is optional, as in the REST app: the auth-version check fails
	// open without it (a revoked access token is then accepted until exp).
	// The client reconnects on its own; the health service reports the
	// outage without taking the server out of rotation.
	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		log.Warn("failed connection to redis, continuing without it", slogger.Err(err))
	} else {
		log.Info("Redis connected!")
	}
	closers.Add("redis", redisClient)

	// Same cache of auth versions as the REST app, so that every call does
	// not hit Redis and a bump is picked up within the cache TTL.
	authVersions := middleware.NewVersionCache(redisClient, 0)

	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(logging.FinishCall),
	}
	// Health probes are frequent and uninteresting; keep them out of the log.
	notHealth := selector.MatchFunc(func(_ context.Context, c interceptors.CallMeta) bool {
		return !strings.HasPrefix(c.FullMethod(), "/"+healthpb.Health_ServiceDesc.ServiceName+"/")
	})

	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandler(func(p interface{}) (err error) {
			log.Error("Recovered from panic", slog.Any("panic", p))

			return status.Errorf(codes.Internal, "internal error")
		}),
	}

	// Same registry layout as the REST app (Go/process collectors included).
	m := metrics.New()
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}),
		),
	)
	m.Registry().MustRegister(srvMetrics)

	// Bearer tokens are parsed on every call; only the listed methods require
	// them (see protectedMethods / moderationMethods).
	authInterceptor := grpcauth.NewAuth(log, cfg.Auth.JWT.Access.Key, authVersions)

	gRPCServer := grpc.NewServer(
		grpc.ConnectionTimeout(cfg.GRPC.ConnectionTimeout),
		grpc.ChainUnaryInterceptor(
			srvMetrics.UnaryServerInterceptor(),
			recovery.UnaryServerInterceptor(recoveryOpts...),
			selector.UnaryServerInterceptor(
				logging.UnaryServerInterceptor(InterceptorLogger(log), loggingOpts...), notHealth),
			authInterceptor.Unary(),
			grpcauth.RequireAuth(protectedMethods...),
			grpcauth.RequireRole([]models.Role{models.RoleModerator, models.RoleAdmin}, moderationMethods...),
		),
		grpc.ChainStreamInterceptor(
			srvMetrics.StreamServerInterceptor(),
			recovery.StreamServerInterceptor(recoveryOpts...),
			selector.StreamServerInterceptor(
				logging.StreamServerInterceptor(InterceptorLogger(log), loggingOpts...), notHealth),
			authInterceptor.Stream(),
		),
	)

	photoRepo, photoCloser := app.NewPhotosRepository(log, cfg)
	closers.Add("s3", photoCloser)

	publisher, publisherCloser := app.NewPublisher(log, cfg.Nats)
	closers.Add("nats", publisherCloser)

	mapRepo := postgres.NewMap(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	mapUseCase := usecase.NewMap(log, usecase.MapRepositories{
		Map: mapRepo,
	})
	mapgrpc.Register(gRPCServer, log, mapUseCase)

	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksRepo := postgres.NewChecks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	marksUseCase := usecase.NewMarks(log, cfg.Marks, trManager, usecase.MarksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Photos: photoRepo,
	})
	marksgrpc.Register(gRPCServer, log, marksUseCase)

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	tasksUseCase := usecase.NewTasks(log, usecase.TasksRepositories{
		Tasks: tasksRepo,
	}).WithEvents(publisher)
	tasksgrpc.Register(gRPCServer, log, tasksUseCase)

	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersUseCase := usecase.NewUsers(log, usecase.UsersRepositories{
		Users: usersRepo,
	})
	usersgrpc.Register(gRPCServer, log, usersUseCase)

	healthUseCase := usecase.NewHealth(log, cfg.Health, usecase.HealthDependencies{
		Required: map[string]usecase.Pinger{"postgres": postgresDB},
		// The auth-version check fails open without Redis, so its loss is
		// reported but does not flip the health status.
		Optional: map[string]usecase.Pinger{"redis": redisClient},
	})
	// Start NOT_SERVING: the first dependency check in watchHealth flips the
	// status, so a probe that lands before it never sees a false SERVING.
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(gRPCServer, healthServer)

	srvMetrics.InitializeMetrics(gRPCServer)

	var metricsServer *http.Server
	if cfg.GRPC.MetricsPort > 0 {
		metricsServer = m.Server(":" + strconv.Itoa(cfg.GRPC.MetricsPort))
	}

	backgroundCtx, stopBackground := context.WithCancel(context.Background())

	return &App{
		gRPCServer:      gRPCServer,
		health:          healthServer,
		healthUC:        healthUseCase,
		log:             log,
		closers:         closers,
		metricsServer:   metricsServer,
		shutdownTimeout: cfg.ShutdownTimeout,
		port:            cfg.GRPC.Port,
		backgroundCtx:   backgroundCtx,
		stopBackground:  stopBackground,
	}
}

// InterceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

// Run starts the gRPC server, the health-check loop and the metrics HTTP
// server (when configured) and blocks until the gRPC server stops. A failure
// of the metrics server is logged but does not stop the gRPC server.
func (a *App) Run() error {
	const op = "grpcapp.Run"

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", a.port))
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	// If Stop already ran (signal during startup) do not start the loop at
	// all; otherwise Stop would have returned from background.Wait before
	// this goroutine was registered.
	if !a.startHealthLoop() {
		_ = l.Close()
		return nil
	}

	if a.metricsServer != nil {
		go func() {
			a.log.Info("grpc metrics server started", slog.String("address", a.metricsServer.Addr))
			if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.log.Error("grpc metrics server failed", slogger.Err(err))
			}
		}()
	}

	a.log.Info("grpc server started", slog.String("address", l.Addr().String()))

	if err := a.gRPCServer.Serve(l); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// startHealthLoop starts watchHealth unless Stop has already been called.
func (a *App) startHealthLoop() bool {
	a.backgroundMu.Lock()
	defer a.backgroundMu.Unlock()

	if a.backgroundCtx.Err() != nil {
		return false
	}
	a.background.Go(func() { a.watchHealth(a.backgroundCtx) })
	return true
}

// watchHealth mirrors the dependency readiness into the gRPC health service
// until ctx is cancelled.
func (a *App) watchHealth(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		st := healthpb.HealthCheckResponse_SERVING
		if _, err := a.healthUC.Check(ctx); err != nil {
			st = healthpb.HealthCheckResponse_NOT_SERVING
		}
		a.health.SetServingStatus("", st)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Stop marks the server NOT_SERVING, drains in-flight RPCs with a bounded
// GracefulStop (falling back to a hard Stop on timeout), shuts the metrics
// server down within the same deadline and closes clients.
func (a *App) Stop() {
	const op = "grpcapp.Stop"

	log := a.log.With(slog.String("op", op))
	log.Info("stopping gRPC server", slog.Int("port", a.port))

	a.backgroundMu.Lock()
	a.stopBackground()
	a.backgroundMu.Unlock()
	a.background.Wait()
	a.health.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	if a.metricsServer != nil {
		wg.Go(func() {
			if err := a.metricsServer.Shutdown(shutdownCtx); err != nil {
				log.Error("an error occurred while stopping the metrics server", slogger.Err(err))
			}
		})
	}

	forceStop := context.AfterFunc(shutdownCtx, func() {
		log.Warn("graceful stop timed out, forcing stop")
		a.gRPCServer.Stop()
	})
	a.gRPCServer.GracefulStop()
	forceStop()
	wg.Wait()

	a.closers.Close(log)
}
