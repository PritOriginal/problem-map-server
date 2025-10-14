# problem-map-server

[![wakatime](https://wakatime.com/badge/user/b2a0c08d-61f2-4144-ba78-aab13a59cb9f/project/62d78167-daec-4c9e-a232-ffef6036e9c7.svg)](https://wakatime.com/badge/user/b2a0c08d-61f2-4144-ba78-aab13a59cb9f/project/62d78167-daec-4c9e-a232-ffef6036e9c7)

В даннои репозитории представлены наработки Golang REST API и gRPC серверов дипломной работы по теме "Разработка информационного обеспечения системы анализа и визуализации проблемных мест города".

[problem-map.pritoriginal.ru](https://problem-map.pritoriginal.ru/) - сайт, на котором можно посмотреть визуализацию. 
`(Находится в актиной разработке)`

[problem-map-react](https://github.com/PritOriginal/problem-map-react) - репозиторий фронта.

> [!NOTE]  
> Этот проект находится в стадии активной разработки, и на данный момент в нём ещё много чего не реализовано, поэтому не исключены ошибки.

### Работа с геоданными

Для работы с геоданными и PostGIS были написаны структуры-обёртки для пакета [github.com/twpayne/go-geom](https://github.com/twpayne/go-geom).

А именно для пакетов [ewkb](https://github.com/twpayne/go-geom) и [geojson](github.com/twpayne/go-geom/encoding/geojson).

## Подготовка

### Для локального запуска

Создайте конфиг

Для `.yaml`

```bash
cp ./configs/config-docker.yaml ./configs/config.yaml
```

Для `.env` (если предпочитаете переменные окружения)

```bash
cp ./configs/.env.example ./configs/.env
```

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

```bash
make test
```

или

```bash
make test-cover
```

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
