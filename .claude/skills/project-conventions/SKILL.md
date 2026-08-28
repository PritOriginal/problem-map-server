---
name: project-conventions
description: Конвенции кода problem-map-server — слои, обработка ошибок, ответы API, тесты. Применять при написании или ревью Go-кода в этом репозитории.
user-invocable: false
---

# Конвенции problem-map-server

## Слои
- `handler` (Gin/gRPC) → `usecase` → `repository`. Интерфейсы репозиториев объявляются в пакете `usecase` рядом с потребителем; хендлер зависит от интерфейса usecase из своего пакета.
- Конструкторы: `NewXxx(log *slog.Logger, deps XxxRepositories)`; зависимости — структурой.
- Хендлеры принимают DTO из `dto.go`, валидируют через binding-теги Gin, отвечают через `pkg/responses` (`OK`, `Created`, `Fail`).

## Ошибки
- Repository возвращает `repository.ErrNotFound` / `repository.ErrExists`; usecase оборачивает в `usecase.ErrNotFound` / `ErrConflict` / `ErrUnauthorized` (`errors.Is`), остальное — `fmt.Errorf("op: %w", err)`.
- Хендлер маппит usecase-ошибки в HTTP-статусы; внутренние ошибки логирует, наружу — 500 без деталей.

## Данные
- SQL через `sqlx`, транзакции через `go-transaction-manager` (`trm.Manager`).
- Геометрия — типы из `internal/models` (`Point`, `Polygon`, `MultiPolygon`) с `*JSON`-двойниками для Swagger.
- Nullable — `github.com/guregu/null/v6`.

## Тесты
- Пакет `xxx_test`, testify `suite.Suite`, table-driven кейсы (`name`, входы, ожидаемые `data/err`), см. `internal/usecase/tasks_test.go`.
- Моки — mockery (`NewMockXxx(suite.T())`, `.On(...).Once().Return(...)`), не писать вручную.
- Логгер в тестах — `pkg/logger/slogdiscard`.
- Функциональные тесты — `tests/{rest,grpc}` с build-tags `functional,rest|grpc`; данные — `gofakeit`.

## Прочее
- Swagger-аннотации на каждом хендлере (`@Summary`, `@Tags`, `@Router`); после правок — `/regen`.
- Логирование — `log/slog` с полями `slog.String("op", op)`.
