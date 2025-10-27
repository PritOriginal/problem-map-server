package main

import (
	"log"

	"github.com/PritOriginal/problem-map-server/internal/app/rest"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

func main() {
	cfg := config.MustLoad()

	logger, err := slogger.SetupLogger(cfg.Env)
	if err != nil {
		log.Fatalf("error init logger: %v", err)
	}

	app := rest.New(logger, cfg)

	app.MustGenerateRoutesDoc()

	logger.Info("documentation generated")
}
