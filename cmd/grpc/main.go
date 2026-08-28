package main

import (
	"log"
	"os"

	"github.com/PritOriginal/problem-map-server/internal/app"
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

	return app.Run(logger, grpcapp.New(logger, cfg))
}
