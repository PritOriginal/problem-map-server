    # problem-map-server

Backend проекта «Карта проблем»: Go 1.25, REST (Gin) + gRPC, PostgreSQL/PostGIS, Redis, NATS/RabbitMQ, S3.

## Команды
- `make run-rest` / `make run-grpc` / `make run-tasker` — запуск с `configs/config.yaml`
- `make test` — unit-тесты (`go test -tags=nomsgpack ./...`)
- `make test-functional-rest` / `make test-functional-grpc` — функциональные тесты (build-tags `functional,rest|grpc`, нужны поднятые БД/сервер, конфиг `configs/config-tests.yaml`)
- `make test-cover` — покрытие без моков
- `make swag` — регенерация Swagger (`docs/`), overrides в `.swaggo`
- `mockery` — регенерация моков по `.mockery.yaml`
- `make migrate NAME_MIGRATION=<name>` — новая миграция; `make migrate-up` / `migrate-down-1`
- `golangci-lint run ./...` — линтер (`.golangci.yml`)

## Структура
- `cmd/{rest,grpc,tasker,migrator,osm}` — точки входа (cobra)
- `internal/handler/<domain>` — HTTP-хендлеры, `dto.go`, swagger-аннотации; ответы через `pkg/responses`
- `internal/grpc/<domain>` — gRPC-хендлеры (`problem-map-protos`)
- `internal/usecase` — бизнес-логика; интерфейсы репозиториев объявлены здесь; ошибки — `usecase/errors.go`
- `internal/repository/{postgres,redis,s3,local}` — доступ к данным; ошибки — `repository/errors.go`
- `internal/models` — доменные модели (геометрия: `Point`, `Polygon`, `MultiPolygon` + `*JSON`)
- `internal/middleware`, `internal/nats`, `internal/broker` — инфраструктура
- `migrations/` — SQL-миграции golang-migrate (`NNNNNN_name.{up,down}.sql`)

## Правила
- Слои строго: handler → usecase → repository. Handler не ходит в БД напрямую.
- Репозиторные ошибки маппятся в usecase-ошибки (`ErrNotFound`, `ErrConflict`, `ErrUnauthorized`), хендлер — в HTTP-статус.
- Не редактировать вручную сгенерированное: `docs/swagger.*`, `docs/docs.go`, `*mocks*.go`, `trm_mocks.go` — только через `make swag` / `mockery`.
- После изменения интерфейсов usecase/handler или swagger-аннотаций — `/regen`.
- Тесты: testify `suite` + table-driven, моки mockery (`NewMockXxx(suite.T())`), логгер `slogdiscard`.
- Секреты и локальные конфиги (`configs/config*.yaml`, `.env*`) не трогать и не коммитить.
- Форматирование `gofmt`, перед PR — `go vet` и `golangci-lint`.
