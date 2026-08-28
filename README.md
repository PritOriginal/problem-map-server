# problem-map-server

[![wakatime](https://wakatime.com/badge/user/b2a0c08d-61f2-4144-ba78-aab13a59cb9f/project/62d78167-daec-4c9e-a232-ffef6036e9c7.svg)](https://wakatime.com/badge/user/b2a0c08d-61f2-4144-ba78-aab13a59cb9f/project/62d78167-daec-4c9e-a232-ffef6036e9c7)

В даннои репозитории представлены наработки Golang REST API и gRPC серверов дипломной работы по теме "Разработка краудсорсинговой системы мониторинга городских проблем с оптимизацией процессов модерации".

<img src="./docs/app-screenshot.png" width="49%" alt="App screenshot" />

<p>
  <img src="./docs/app-mobile-screenshot-1.png" width="24%" alt="App screenshot" />
  <img src="./docs/app-mobile-screenshot-2.png" width="24%" alt="App screenshot" />
</p>

## О проекте

[problem-map.pritoriginal.ru](https://problem-map.pritoriginal.ru/) - сайт, на котором можно посмотреть визуализацию.
`(Находится в активной разработке)`

[problem-map-react](https://github.com/PritOriginal/problem-map-react) - репозиторий фронта.

[Swagger документация](./docs/swagger.json) - доступна по адресу `http://[host]:[port]/swagger/index.html`

> [!NOTE]  
> Этот проект находится в процессе активной разработки. В настоящее время в нём реализовано не всё запланированное, поэтому не исключено наличие ошибок.

### Работа с геоданными

Для работы с геоданными и PostGIS были написаны структуры-обёртки для пакета [github.com/twpayne/go-geom](https://github.com/twpayne/go-geom).

А именно для пакетов [ewkb](https://github.com/twpayne/go-geom) и [geojson](github.com/twpayne/go-geom/encoding/geojson).

### Стек

- [`Gin`](https://github.com/gin-gonic/gin) - Веб-фреймворк
- `PostgreSQL` - БД
- `PostGIS` - Для поддержки хранения геоданных
- [`migrate`](https://github.com/golang-migrate/migrate) - Миграции
- `Redis` - Кеширование
- [`go-transaction-manager`](https://github.com/avito-tech/go-transaction-manager) - Менеджер транзакций
- `S3` - Для хранения фото меток
- `Docker` - Контейнеризация
- `log/slog` - Логгер
- `GitHub Actions` - CI/CD  
- [`swaggo/swag`](https://github.com/swaggo/swag) - OpenAPI (Swagger)
- [`OpenStreetMap`](https://www.openstreetmap.org/) - Источник пространственных данных (административных границ)
- [`Overpass QL`](https://wiki.openstreetmap.org/wiki/Overpass_API/Overpass_QL) - Язык запросов для работы с данными OpenStreetMap
- [`osm2pgsql`](https://osm2pgsql.org/) - Инструмент для импорта данных OpenStreetMap

API:

- `REST` (Основа)
- `gRPC`

## Подготовка

### Для локального запуска

Создайте конфиг

Для `.yaml`

```bash
cp ./configs/config.yaml.example ./configs/config.yaml
```

Для `.env` (если предпочитаете переменные окружения)

```bash
cp ./configs/.env.example ./configs/.env
```

### Переменные окружения и секреты

Конфиг читается из файла, путь к которому передаётся флагом `--config=<path>`
или переменной `CONFIG_PATH`. Значения из файла можно переопределить
переменными окружения (имена указаны в `configs/.env.example`).

Локальные конфиги (`configs/config*.yaml`, `configs/.env*`) игнорируются git —
в репозитории лежат только шаблоны `config.yaml.example` / `.env.example`
и `.env.docker` для compose.

**JWT-ключи в шаблонах — плейсхолдеры.** При старте сервер проверяет, что
`JWT_ACCESS_TOKEN_KEY` / `JWT_REFRESH_TOKEN_KEY` не пустые и не короче
32 байт, а `POSTGRES_PASSWORD` задан; иначе запуск завершится с понятной ошибкой.
Сгенерируйте собственные ключи и подставьте их в конфиг:

```bash
openssl rand -base64 32   # JWT_ACCESS_TOKEN_KEY
openssl rand -base64 32   # JWT_REFRESH_TOKEN_KEY
```

Основные переменные:

| Переменная | Описание | По умолчанию |
|---|---|---|
| `JWT_ACCESS_TOKEN_KEY` / `JWT_REFRESH_TOKEN_KEY` | ключи подписи JWT (>= 32 байт) | — |
| `JWT_ACCESS_TOKEN_EXPIRED_IN` / `JWT_REFRESH_TOKEN_EXPIRED_IN` | время жизни токенов | — |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | подключение к PostgreSQL | — |
| `POSTGRES_SSLMODE` | `sslmode` для PostgreSQL | `disable` |
| `POSTGRES_MAX_OPEN_CONNS` / `POSTGRES_MAX_IDLE_CONNS` / `POSTGRES_CONN_MAX_LIFETIME` | пул соединений | `25` / `5` / `5m` |
| `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` | подключение к Redis | — |
| `AWS_KEY`, `AWS_SECRET_KEY`, `AWS_ENDPOINT` | S3-хранилище фото (при `PHOTO_STORAGE=s3`) | — |

В Docker конфиг не копируется в образ: `docker/*/compose.yaml` монтирует
`configs/.env.docker` в контейнер (`/etc/problem-map/.env`) и передаёт его же
через `env_file`.

## Запуск

### Запуск REST API сервера

```bash
make run-rest
```

Docker:

```bash
make docker-rest
```

### Запуск gRPC сервера

```bash
make run-grpc
```

Docker:

```bash
make docker-grpc
```

## Тесты

### Unit-тесты

Простой прогон тестов:

```bash
make test
```

Прогон тестов с выводом покрытия:

```bash
make test-cover
```

### Функциональные тесты

Запуск функциональных тестов

Для REST:

```bash
make test-functional-rest
```

Для gRPC (`В РАЗРАБОТКЕ`):

```bash
make test-functional-rest
```

> [!NOTE]  
> Перед запуском функциональных тестов убедитесь, что тестируемый сервис запущен.

## Миграции

`migrate create`:

```bash
make migrate NAME_MIGRATION="name_migration" 
```

`migrate up`:

```bash
make migrate-up
```

`migrate down`:

```bash
make migrate-down
```

`migrate drop`:

```bash
make migrate-drop
```

`migrate version`:

```bash
make migrate-version
```

`migrate force`:

```bash
make migrate-force MIGRATION_VERSION=<migration-version> 
```

## Примечание

Если в качестве конфигурационного файла был выбран `.env`, то замените путь к конфигурационному файлу в `Makefile` либо запускайте приложение командой:

Для REST API:

```bash
go run ./cmd/rest/ --config=./configs/.env
```

Для gRPC:

```bash
go run ./cmd/grpc/ --config=./configs/.env
```
