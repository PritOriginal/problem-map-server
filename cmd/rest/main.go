package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PritOriginal/problem-map-server/internal/app/rest"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

func main() {
	cfg := config.MustLoad()

	var logFIle *os.File
	if cfg.Env != slogger.Local {
		logFIle, err := os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("error opening file: %v", err)
		}
		defer logFIle.Close()
	}

	logger, err := slogger.SetupLogger(cfg.Env, logFIle)
	if err != nil {
		log.Fatalf("error init logger: %v", err)
	}

	app := rest.New(logger, cfg)

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
