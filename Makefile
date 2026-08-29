.PHONY: help run-rest run-grpc run-tasker run-osm \
	build build-rest build-grpc build-tasker build-migrator build-osm \
	docker-rest docker-grpc \
	test test-integration test-functional-rest test-functional-grpc test-cover test-cover-functional-rest \
	fmt vet lint vuln \
	migrate migrate-version migrate-force migrate-up migrate-up-1 migrate-down-1 migrate-drop \
	swag

CONFIG ?= ./configs/config.yaml

help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'

## --- Run ---------------------------------------------------------------------

run-rest: ## Run REST server with configs/config.yaml
	go run ./cmd/rest/ --config=$(CONFIG)
run-grpc: ## Run gRPC server with configs/config.yaml
	go run ./cmd/grpc/ --config=$(CONFIG)
run-tasker: ## Run tasker once with configs/config.yaml
	go run ./cmd/tasker/ --config=$(CONFIG)
run-osm: ## Run OSM importer
	go run ./cmd/osm/

## --- Build -------------------------------------------------------------------

build: build-rest build-grpc build-tasker build-migrator build-osm ## Build all binaries
build-rest: ## Build REST server binary
	go build ./cmd/rest/
build-grpc: ## Build gRPC server binary
	go build ./cmd/grpc/
build-tasker: ## Build tasker binary
	go build ./cmd/tasker/
build-migrator: ## Build migrator binary
	go build ./cmd/migrator/
build-osm: ## Build OSM importer binary
	go build ./cmd/osm/

## --- Docker ------------------------------------------------------------------

docker-rest: ## Start REST stack via docker compose
	docker compose -f docker/rest/compose.yaml --env-file configs/.env.docker --project-directory . up --build -d
docker-grpc: ## Start gRPC stack via docker compose
	docker compose -f docker/grpc/compose.yaml --env-file configs/.env.docker --project-directory . up --build -d

## --- Quality -----------------------------------------------------------------

fmt: ## Format sources with gofmt
	gofmt -s -w $$(git ls-files '*.go' | grep -v '^docs/')
vet: ## Run go vet
	go vet -tags=nomsgpack ./...
lint: ## Run golangci-lint
	golangci-lint run ./...
vuln: ## Run govulncheck (installs it if missing)
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck -tags=nomsgpack ./...

## --- Tests -------------------------------------------------------------------

test: ## Run unit tests
	go test -tags=nomsgpack ./...

test-integration: ## Run repository integration tests (Docker required, testcontainers)
	go test -tags=integration ./internal/repository/... -count 1

test-functional-rest: ## Run REST functional tests (server must be running)
	go test -tags=functional,rest ./tests/rest -count 1

test-functional-grpc: ## Run gRPC functional tests (server must be running)
	go test -tags=functional,grpc ./tests/grpc -count 1

test-cover: ## Unit test coverage without mocks
	go test ./... -coverprofile cover.test.tmp -coverpkg ./...
	cat cover.test.tmp | grep -v "mocks" > cover.test
	rm cover.test.tmp
	go tool cover -func cover.test

test-cover-functional-rest: ## Build REST server with coverage and run it (GOCOVERDIR=test-cover)
	mkdir -p test-cover
	go build -cover -o test-rest.test ./cmd/rest/
	GOCOVERDIR=test-cover ./test-rest.test --config=$(CONFIG)

## --- Migrations --------------------------------------------------------------

migrate: ## Create migration: make migrate NAME_MIGRATION=<name>
	migrate create -ext=sql -dir=./migrations -seq ${NAME_MIGRATION}
migrate-version: ## Show current migration version
	go run ./cmd/migrator version --migrations-path=./migrations --config=$(CONFIG)
migrate-force: ## Force migration version: make migrate-force MIGRATION_VERSION=<n>
	go run ./cmd/migrator force ${MIGRATION_VERSION} --migrations-path=./migrations --config=$(CONFIG)
migrate-up: ## Apply all pending migrations
	go run ./cmd/migrator up --migrations-path=./migrations --config=$(CONFIG)
migrate-up-1: ## Apply one migration
	go run ./cmd/migrator up --steps 1 --migrations-path=./migrations --config=$(CONFIG)
migrate-down-1: ## Roll back one migration
	go run ./cmd/migrator down --migrations-path=./migrations --config=$(CONFIG)
migrate-drop: ## Drop everything in the database
	go run ./cmd/migrator drop --migrations-path=./migrations --config=$(CONFIG)

## --- Docs --------------------------------------------------------------------

swag: ## Regenerate Swagger docs
	swag init -g ./cmd/rest/main.go --parseDependency --overridesFile .swaggo
	swag fmt
