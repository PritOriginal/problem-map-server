package grpcapp

import (
	"context"
	"fmt"
	"log/slog"
	"net"

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
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type App struct {
	gRPCServer *grpc.Server
	log        *slog.Logger
	db         *postgres.Postgres
	port       int
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
		logging.WithLogOnEvents(
			//logging.StartCall, logging.FinishCall,
			logging.PayloadReceived, logging.PayloadSent,
		),
		// Add any other option (check functions starting with logging.With).
	}

	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandler(func(p interface{}) (err error) {
			log.Error("Recovered from panic", slog.Any("panic", p))

			return status.Errorf(codes.Internal, "internal error")
		}),
	}

	gRPCServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		recovery.UnaryServerInterceptor(recoveryOpts...),
		logging.UnaryServerInterceptor(InterceptorLogger(log), loggingOpts...),
	))

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

	return &App{
		gRPCServer: gRPCServer,
		log:        log,
		db:         postgresDB,
		port:       cfg.GRPC.Port,
	}
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

func (a *App) Stop() {
	const op = "grpcapp.Stop"

	a.log.With(slog.String("op", op)).
		Info("stopping gRPC server", slog.Int("port", a.port))

	a.gRPCServer.GracefulStop()

	if err := a.db.DB.Close(); err != nil {
		a.log.Error("an error occurred while closing the connection to the database", slogger.Err(err))
	}
}
