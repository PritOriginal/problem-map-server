include configs/.env

run-rest:
	go run ./cmd/rest/ --config=./configs/config.yaml
build-rest:
	go build ./cmd/rest/

docker-rest:
	docker compose -f docker/rest/compose.yaml --env-file configs/.env.docker --project-directory . up --build -d
docker-grpc:
	docker compose -f docker/grpc/compose.yaml --env-file configs/.env.docker --project-directory . up --build -d   

run-grpc:
	go run ./cmd/grpc/ --config=./configs/config.yaml
build-grpc:
	go build ./cmd/grpc/

test:
	go test ./...

test-cover:
	go test ./... -coverprofile cover.test.tmp -coverpkg ./...
	cat cover.test.tmp | grep -v "mocks" > cover.test 
	rm cover.test.tmp 
	go tool cover -func cover.test 

migrate:
	migrate create -ext=sql -dir=./migrations -seq ${NAME_MIGRATION}     
migrate-up:
	migrate -path ./migrations/ -database postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}/${POSTGRES_DB}?sslmode=disable up
migrate-up-1:
	migrate -path ./migrations/ -database postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}/${POSTGRES_DB}?sslmode=disable up 1
migrate-down:
	migrate -path ./migrations/ -database postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}/${POSTGRES_DB}?sslmode=disable down
migrate-down-1:
	migrate -path ./migrations/ -database postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}/${POSTGRES_DB}?sslmode=disable down 1

run-osm:
	go run ./cmd/osm/
build-osm:
	go build ./cmd/osm/

swag:
	swag init -g ./cmd/rest/main.go --parseDependency --overridesFile .swaggo
	swag fmt