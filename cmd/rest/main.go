package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PritOriginal/problem-map-server/internal/app/rest"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

//	@title			Problem Map API
//	@version		1.0
//	@description	This is the API documentation for the "Problem Map" project.

//	@tag.name			auth
//	@tag.description	Authorization and authentication

//	@tag.name			map
//	@tag.description	Operations with geodata

//	@tag.name			marks
//	@tag.description	Operations with markers

//	@tag.name			checks
//	@tag.description	Operations with checks

//	@tag.name			tasks
//	@tag.description	Operations with tasks

//	@tag.name			users
//	@tag.description	Operations with users

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

	app := rest.New(logger, cfg)

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
