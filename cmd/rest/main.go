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

//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-Api-Key
//	@description				Open-data API key "pm_live_…" (POST /api-keys); also accepted as "Authorization: ApiKey {key}". Optional on public GET routes: with a key the per-key quota applies and X-RateLimit-* headers are returned; ignored when a valid Bearer JWT is sent as well (the JWT identity wins)

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

//	@tag.name			api-keys
//	@tag.description	API keys of the open-data endpoints

//	@tag.name			open
//	@tag.description	Open data: public aggregates

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
