package main

import (
	"log"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
)

func main() {
	cfg := config.MustLoad()

	logger, err := slogger.SetupLogger(cfg.Env)
	if err != nil {
		log.Fatalf("error init logger: %v", err)
	}

	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		logger.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	logger.Info("PostgreSQL connected!")

	tasksRepo := postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	usersRepo := postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter)

	tasker := usecase.NewTaskser(logger, usecase.TaskerRepositories{
		Tasks: tasksRepo,
		Marks: marksRepo,
		Users: usersRepo,
	})

	err = tasker.Update()
	if err != nil {
		logger.Error("error update", slogger.Err(err))
	}
}
