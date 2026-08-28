package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	grpcapp "github.com/PritOriginal/problem-map-server/internal/app/grpc"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
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

	app := grpcapp.New(logger, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run()
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			logger.Error("server failed", slogger.Err(err))
			exitCode = 1
		}
	}

	app.Stop()

	logger.Info("server stopped")
	return exitCode
}
