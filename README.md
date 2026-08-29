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

**Порядок координат в API.** Все геометрии хранятся в WGS84 (SRID 4326) в порядке GeoJSON/PostGIS: `X = longitude` (долгота), `Y = latitude` (широта). В `POST /marks` поля формы `longitude` и `latitude` передаются раздельно и в точку кладутся как `Point(longitude, latitude)`; клиенты не должны менять их местами. Для Тамбова: `longitude ≈ 41.4`, `latitude ≈ 52.7`.

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

### Запуск tasker (раздача заданий на проверку)

`cmd/tasker` — фоновая задача, которая раздаёт пользователям задания на
проверку неподтверждённых меток. Каждый прогон делает два шага:

1. `ExpireOverdue` — все задания в статусе «Выдано» с истёкшим `due_at`
   переводятся в «Просрочено» одним `UPDATE`;
2. `Update` — для меток в статусах «Неподтверждённая» / «На проверке»
   подбираются исполнители и новые задания записываются одной транзакцией
   с `due_at = now + tasker.task-ttl`.

Режимы:

```bash
make run-tasker                                   # один прогон (--once)
go run ./cmd/tasker --config=configs/config.yaml  # по расписанию, tasker.interval
go run ./cmd/tasker --config=configs/config.yaml --interval 5m
```

В режиме расписания первый прогон выполняется сразу, дальше — по тикеру;
`SIGINT`/`SIGTERM` завершают процесс с кодом 0 (текущий запрос отменяется
через контекст). Неудачный прогон логируется, расписание продолжается.

Конфигурация (`tasker:` в YAML, переменные окружения `TASKER_*`):

| Параметр | Env | По умолчанию | Описание |
|---|---|---|---|
| `interval` | `TASKER_INTERVAL` | `15m` | период запусков |
| `task-ttl` | `TASKER_TASK_TTL` | `72h` | срок выполнения задания (`due_at`) |
| `max-tasks-per-user` | `TASKER_MAX_TASKS_PER_USER` | `3` | лимит одновременно выданных заданий на пользователя |
| `required-checks` | `TASKER_REQUIRED_CHECKS` | `2` | сколько независимых проверок нужно метке |
| `target-probability` | `TASKER_TARGET_PROBABILITY` | `0.8` | целевая вероятность получить `required-checks` проверок |
| `max-radius-meters` | `TASKER_MAX_RADIUS_METERS` | `5000` | радиус от дома пользователя до метки |
| `distance-lambda` | `TASKER_DISTANCE_LAMBDA` | `0.05` | затухание по расстоянию (на км) |
| `load-delta` | `TASKER_LOAD_DELTA` | `0.3` | штраф за каждое выданное задание |
| `fatigue-beta` | `TASKER_FATIGUE_BETA` | `0.2` | штраф за каждое просроченное задание |

Вероятность того, что пользователь проверит метку на расстоянии `d` км от
дома:

```
p = (rating(r) + distance(d)) · load(l) · fatigue(o),   p ≤ 1
rating(r)   = 0.2 / (1 + 100·e^(−r/2))        r — рейтинг пользователя
distance(d) = 0.5 · e^(−λ·d)                   λ = distance-lambda
load(l)     = 1 / (1 + δ·(l + 1))              l — выданных заданий, δ = load-delta
fatigue(o)  = 1 / (1 + β·o)                    o — просроченных заданий, β = fatigue-beta
```

Метка считается покрытой, когда вероятность того, что хотя бы
`required-checks` из её исполнителей выполнят проверку (распределение
Пуассона-биномиальное), достигает `target-probability`. Пока есть
непокрытые метки и свободные кандидаты, каждой такой метке за раунд
добавляется лучший по `p` пользователь; пара «пользователь–метка», по которой
уже есть задание в любом статусе, повторно не выдаётся (дополнительно
частичный уникальный индекс `tasks(user_id, mark_id) WHERE status_id = 1`
защищает от гонки двух одновременных прогонов).

## Аутентификация

Сервер выдаёт пару JWT (HS256): **access** (`typ=access`) для заголовка
`Authorization: Bearer …` и **refresh** (`typ=refresh`, уникальный `jti`) для
`POST /auth/tokens/refresh`. Токены не взаимозаменяемы: refresh-токен не
проходит auth-middleware (REST и gRPC), access-токен не принимается на
refresh/logout. Оба несут `role` и `ver` (версию авторизации пользователя).

Состояние сессий хранится в Redis (миграций БД нет):

| Ключ | Значение | TTL |
|---|---|---|
| `refresh:<user_id>:<jti>` | активный refresh-токен | срок жизни refresh |
| `refresh:<user_id>` | SET активных `jti` пользователя (индекс для `logout-all`, без `SCAN`) | срок жизни последнего выданного refresh |
| `user:<user_id>:auth_version` | текущая версия авторизации (`ver`) | — |

Все операции с refresh-токенами — Lua-скрипты, т.е. атомарны: проверка и
удаление `jti` при ротации — один `DEL` внутри скрипта, поэтому из двух
параллельных refresh с одним токеном новую пару получает только один.

Эндпоинты:

| Метод | Путь | Доступ | Действие |
|---|---|---|---|
| `POST` | `/auth/signin` | — | выдаёт пару токенов, регистрирует `jti` |
| `POST` | `/auth/tokens/refresh` | — | **одноразовая ротация**: старый `jti` удаляется, выдаётся новая пара. Повторное предъявление уже использованного (или отозванного) refresh-токена трактуется как кража: `401` и отзыв всех refresh-токенов пользователя |
| `POST` | `/auth/logout` | JWT | отзывает переданный в теле refresh-токен (`{"refresh_token": …}`), access-токен живёт до `exp` |
| `POST` | `/auth/logout-all` | JWT | отзывает все refresh-токены и инкрементит `auth_version` — все access-токены сразу становятся недействительными |
| `POST` | `/users/me/password` | JWT | смена пароля (`{"old_password", "new_password"}`, новый ≥ 8 символов, bcrypt cost 12); неверный старый пароль — `403`; все сессии сбрасываются |
| `PATCH` | `/users/{id}/role` | admin | смена роли (`{"role": "user"|"moderator"|"admin"}`); сессии пользователя сбрасываются, новая роль действует сразу. Последний admin не может снять роль admin с самого себя (`403`) |

**Отзыв при повторном использовании — осознанный DoS-вектор.** Если старый
(уже использованный) refresh-токен утёк, злоумышленник может предъявлять
его и каждый раз разлогинивать все устройства жертвы. Это принято как
плата за детекцию кражи: доступ к аккаунту при этом не получить, а
пользователь замечает проблему по внезапным выходам.

Актуальность роли: auth-middleware сравнивает `ver` из токена с
`user:<id>:auth_version` в Redis (значение кэшируется в памяти процесса на
5 с), при расхождении — `401`. Ротация refresh-токена тоже проверяет `ver`.
Так смена роли, пароля и `logout-all` вступают в силу не дожидаясь
истечения access-токена: на инстансе, обработавшем отзыв, кэш сбрасывается
сразу, на остальных — с задержкой до 5 с.

Политика версий: отсутствие ключа `auth_version` означает версию `0`;
если ключ есть — `ver` токена должен совпадать с ним точно. Токены,
выданные с `ver=0` пользователю, чья версия уже ≥ 1 (см. fail-open ниже),
отвергаются.

gRPC-интерцептор проверяет подпись, `exp` и `typ`, но **не** `ver`: у gRPC
нет Redis, поэтому отозванный access-токен там принимается до истечения
`exp` (по умолчанию короткого). Это осознанное ограничение.

**Redis недоступен — fail-open.** Как и кэш с rate-limit'ом, все проверки
сессий деградируют мягко: сервер стартует без Redis (в лог пишется error,
`readyz` показывает `redis: error`), клиент переподключается сам. Пока
Redis недоступен:

- вход и ротация работают, но `jti` не сохраняется/не проверяется
  (защита от повторного использования refresh-токена не действует);
- проверка `ver` пропускается с предупреждением в логе; токены выдаются с
  `ver=0`;
- `logout`/`logout-all`/смена пароля и роли не могут отозвать токены — в
  лог пишется warn.

После восстановления Redis токены, выданные во время простоя, отвергаются
(пользователю нужно войти заново): refresh-токены не найдутся в хранилище,
а `ver=0` не совпадёт с хранимой версией у всех, кто хоть раз отзывал
сессии. У пользователей без ключа `auth_version` (версия `0`) такие токены
продолжают работать.
### Рейтинг и антифрод

Рейтинг пользователя (`users.rating`, его читает tasker) меняется только
через журнал `rating_events` — каждое начисление записывается вместе с
причиной, меткой и проверкой, а `users.rating` обновляется тем же запросом.

Начисления (секция `rating:` в YAML, переменные `RATING_*`):

| Параметр | Env | По умолчанию | Когда |
|---|---|---|---|
| `check-correct` | `RATING_CHECK_CORRECT` | `2` | голос проверяющего совпал с итогом этапа |
| `check-wrong` | `RATING_CHECK_WRONG` | `-1` | голос проверяющего не совпал с итогом |
| `mark-confirmed` | `RATING_MARK_CONFIRMED` | `3` | автору — метка подтверждена (`Неподтверждённая → Подтверждена`) |
| `mark-refuted` | `RATING_MARK_REFUTED` | `-2` | автору — метка опровергнута (`Неподтверждённая → Опровергнута`) |
| `task-completed` | `RATING_TASK_COMPLETED` | `1` | проверка закрыла выданное задание |
| `max-checks-per-day` | `RATING_MAX_CHECKS_PER_DAY` | `50` | лимит проверок на пользователя за скользящие 24 часа |

Этап голосования закрывается, когда сумма голосов (`±1`) по текущему
элементу `mark_status_history` достигает `±3` — или решением модератора
(`POST /marks/{id}/confirm|reject`). Начисления и смена статуса пишутся в
одной транзакции.

Антифрод в `POST /checks`: автор не может проверять свою метку (`403`),
одна проверка на этап (`409`), не больше `max-checks-per-day` проверок в
сутки (`429`).

Эндпоинты:

- `GET /users/{id}/stats`, `GET /users/me/stats` — `{rating, marks_total, marks_confirmed, marks_refuted, checks_total, checks_correct, tasks_completed}`;
- `GET /leaderboard?limit=&offset=` — `{user_id, username, rating}` по убыванию рейтинга;
- `GET /users/{id}/rating-events?limit=&offset=` — журнал начислений (владельцу и модератору).

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
make test-functional-grpc
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
make migrate-down-1
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
