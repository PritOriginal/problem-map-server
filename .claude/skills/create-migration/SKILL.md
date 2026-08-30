---
name: create-migration
description: Создать новую SQL-миграцию golang-migrate в ./migrations (up/down) с последовательным номером. Использовать при изменении схемы БД.
disable-model-invocation: true
argument-hint: <snake_case_name>
allowed-tools: Bash(migrate create:*), Read, Edit, Write
---

# Создание миграции

Имя: `$ARGUMENTS` (snake_case, глагол + объект: `add_x_to_y`, `rename_a_to_b`).

1. Выполни: `migrate create -ext=sql -dir=./migrations -seq $ARGUMENTS`
2. Заполни оба файла `migrations/NNNNNN_$ARGUMENTS.{up,down}.sql`:
   - `down.sql` обязан полностью откатывать `up.sql`.
   - Оборачивай в `BEGIN; ... COMMIT;`, если несколько операций.
   - Геометрия — PostGIS (`geometry(Point, 4326)` и т. п.); для геоколонок добавляй GiST-индекс (образец: `migrations/000022_add_spatial_indexes.up.sql`).
   - Новые таблицы: `created_at`/`updated_at TIMESTAMP DEFAULT NOW()` (образец: `000025`).
   - Справочные данные (статусы, типы) — отдельным `INSERT`, с откатом через `DELETE` в down.
3. Напомни пользователю обновить `internal/models` и `internal/repository/postgres`, затем `make migrate-up`.
