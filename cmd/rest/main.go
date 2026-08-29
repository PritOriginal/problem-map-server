package main

import (
	"log"
	"os"

	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/app/rest"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

//	@title			Problem Map API
//	@version		1.0
//	@description	This is the API documentation for the "Problem Map" project.

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				JWT access token: "Bearer {token}"

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

//	@tag.name			notifications
//	@tag.description	User notifications and push devices

//	@tag.name			moderation
//	@tag.description	User reports and the moderation queue

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

	return app.Run(logger, rest.New(logger, cfg))
}
