package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := config.MustLoad()

	logger, err := slogger.SetupLogger(cfg.Env)
	if err != nil {
		log.Printf("error init logger: %v", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		logger.Error("failed connection to database", slogger.Err(err))
		return 1
	}
	logger.Info("PostgreSQL connected!")
	defer func() {
		if err := postgresDB.Close(); err != nil {
			logger.Error("an error occurred while closing the connection to the database", slogger.Err(err))
		}
	}()

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)

	trManager := manager.Must(trmsqlx.NewDefaultFactory(postgresDB.DB))
	tasker := usecase.NewTaskser(logger, trManager, usecase.TaskerRepositories{
		Tasks: tasksRepo,
		Marks: marksRepo,
		Users: usersRepo,
	})

	// Update observes ctx: on SIGINT/SIGTERM the in-flight repository call
	// is cancelled and Update returns, so the deferred DB close never races
	// with a running query.
	if err := tasker.Update(ctx); err != nil {
		if ctx.Err() != nil {
			logger.Warn("shutdown signal received, update aborted", slogger.Err(err))
			return 0
		}
		logger.Error("error update", slogger.Err(err))
		return 1
	}

	logger.Info("update finished")
	return 0
}
