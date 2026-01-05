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

//	@title			Problem Map API
//	@version		1.0
//	@description	This is the API documentation for the "Problem Map" project.

//	@host		localhost:3333
//	@BasePath	/

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

//	@externalDocs.description	OpenAPI
//	@externalDocs.url			https://swagger.io/resources/open-api/

func main() {
	cfg := config.MustLoad()

	logger, err := slogger.SetupLogger(cfg.Env)
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
