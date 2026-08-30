---
name: regen
description: Перегенерировать Swagger-доки (swag) и mockery-моки после изменения хендлеров, интерфейсов usecase или swagger-аннотаций.
disable-model-invocation: true
allowed-tools: Bash(make swag:*), Bash(swag:*), Bash(mockery:*), Bash(go build:*), Bash(git status:*)
---

# Регенерация артефактов

1. `make swag` — обновляет `docs/docs.go`, `docs/swagger.{json,yaml}` (overrides в `.swaggo`) и форматирует аннотации.
2. `mockery` — обновляет моки по `.mockery.yaml` (`internal/usecase/mocks_test.go`, `trm_mocks.go`, `internal/handler/**/mocks_test.go`, `internal/middleware/cache/mocks.go`).
3. `go build ./... && go vet ./...` — убедиться, что всё компилируется.
4. Покажи `git status --short docs internal` и кратко перечисли, что изменилось.

Если `swag` падает на типах геометрии — проверь, что новые типы моделей добавлены в `.swaggo`.
