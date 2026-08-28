package grpcapp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	mapgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/map"
	marksgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/marks"
	tasksgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/tasks"
	usersgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/users"
	"github.com/PritOriginal/problem-map-server/internal/repository/local"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/repository/s3"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const (
	defaultConnectionTimeout = 120 * time.Second
	gracefulStopTimeout      = 10 * time.Second
)

type App struct {
	gRPCServer *grpc.Server
	health     *health.Server
	log        *slog.Logger
	db         *postgres.Postgres
	closers    []namedCloser
	metrics    *prometheus.Registry
	port       int
}

type namedCloser struct {
	name string
	c    io.Closer
}

func New(log *slog.Logger, cfg *config.Config) *App {
	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		log.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	log.Info("PostgreSQL connected!")
	trManager := manager.Must(trmsqlx.NewDefaultFactory(postgresDB.DB))

	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
	}

	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandler(func(p interface{}) (err error) {
			log.Error("Recovered from panic", slog.Any("panic", p))

			return status.Errorf(codes.Internal, "internal error")
		}),
	}

	registry := prometheus.NewRegistry()
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	registry.MustRegister(srvMetrics)

	connTimeout := cfg.GRPC.Timeout
	if connTimeout <= 0 {
		connTimeout = defaultConnectionTimeout
	}

	gRPCServer := grpc.NewServer(
		grpc.ConnectionTimeout(connTimeout),
		grpc.ChainUnaryInterceptor(
			srvMetrics.UnaryServerInterceptor(),
			recovery.UnaryServerInterceptor(recoveryOpts...),
			logging.UnaryServerInterceptor(InterceptorLogger(log), loggingOpts...),
		),
		grpc.ChainStreamInterceptor(
			srvMetrics.StreamServerInterceptor(),
			recovery.StreamServerInterceptor(recoveryOpts...),
			logging.StreamServerInterceptor(InterceptorLogger(log), loggingOpts...),
		),
	)

	var closers []namedCloser

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
		closers = append(closers, namedCloser{name: "s3", c: s3Client})
	}
	closers = append(closers, namedCloser{name: "database", c: postgresDB})

	mapRepo := postgres.NewMap(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	mapUseCase := usecase.NewMap(log, usecase.MapRepositories{
		Map: mapRepo,
	})
	mapgrpc.Register(gRPCServer, mapUseCase)

	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	checksRepo := postgres.NewChecks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	marksUseCase := usecase.NewMarks(log, trManager, usecase.MarksRepositories{
		Marks:  marksRepo,
		Checks: checksRepo,
		Photos: photoRepo,
	})
	marksgrpc.Register(gRPCServer, marksUseCase)

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	tasksUseCase := usecase.NewTasks(log, usecase.TasksRepositories{
		Tasks: tasksRepo,
	})
	tasksgrpc.Register(gRPCServer, tasksUseCase)

	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersUseCase := usecase.NewUsers(log, usecase.UsersRepositories{
		Users: usersRepo,
	})
	usersgrpc.Register(gRPCServer, usersUseCase)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(gRPCServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	srvMetrics.InitializeMetrics(gRPCServer)

	return &App{
		gRPCServer: gRPCServer,
		health:     healthServer,
		log:        log,
		db:         postgresDB,
		closers:    closers,
		metrics:    registry,
		port:       cfg.GRPC.Port,
	}
}

// MetricsRegistry returns the Prometheus registry holding gRPC server metrics.
func (a *App) MetricsRegistry() *prometheus.Registry {
	return a.metrics
}

// InterceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil {
		panic(err)
	}
}

// Run starts the gRPC server and blocks until it stops.
func (a *App) Run() error {
	const op = "grpcapp.Run"

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", a.port))
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	a.log.Info("grpc server started", slog.String("address", l.Addr().String()))

	if err := a.gRPCServer.Serve(l); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// Stop marks the server NOT_SERVING, drains in-flight RPCs with a bounded
// GracefulStop (falling back to a hard Stop on timeout) and closes clients.
func (a *App) Stop() {
	const op = "grpcapp.Stop"

	log := a.log.With(slog.String("op", op))
	log.Info("stopping gRPC server", slog.Int("port", a.port))

	a.health.Shutdown()

	forceStop := time.AfterFunc(gracefulStopTimeout, func() {
		log.Warn("graceful stop timed out, forcing stop")
		a.gRPCServer.Stop()
	})
	a.gRPCServer.GracefulStop()
	forceStop.Stop()

	for _, nc := range a.closers {
		if err := nc.c.Close(); err != nil {
			log.Error("an error occurred while closing the client", slog.String("client", nc.name), slogger.Err(err))
		}
	}
}
