package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	grpcapp "github.com/PritOriginal/problem-map-server/internal/app/grpc"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

func main() {
	cfg := config.MustLoad()

	logger, err := slogger.SetupLogger(cfg.Env)
	if err != nil {
		log.Fatalf("error init logger: %v", err)
	}

	app := grpcapp.New(logger, cfg)

	go func() {
		app.MustRun()
	}()

	// Graceful shutdown

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	<-done

	app.Stop()

	logger.Info("server stopped")
}
