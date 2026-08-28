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

	tasker := usecase.NewTaskser(logger, usecase.TaskerRepositories{
		Tasks: tasksRepo,
		Marks: marksRepo,
		Users: usersRepo,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- tasker.Update()
	}()

	select {
	case <-ctx.Done():
		logger.Warn("shutdown signal received, aborting update")
		return 1
	case err := <-errCh:
		if err != nil {
			logger.Error("error update", slogger.Err(err))
			return 1
		}
	}

	logger.Info("update finished")
	return 0
}
