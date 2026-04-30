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
	go test -tags=nomsgpack ./...

test-functional-rest:
	go test -tags=functional,rest ./tests/rest -count 1

test-functional-grpc:
	go test -tags=functional,grpc ./tests/grpc -count 1

test-cover:
	go test ./... -coverprofile cover.test.tmp -coverpkg ./...
	cat cover.test.tmp | grep -v "mocks" > cover.test 
	rm cover.test.tmp 
	go tool cover -func cover.test 

migrate:
	migrate create -ext=sql -dir=./migrations -seq ${NAME_MIGRATION}     
migrate-version:
	go run ./cmd/migrator version --migrations-path=./migrations --config=./configs/config.yaml
migrate-force:
	go run ./cmd/migrator force ${MIGRATION_VERSION} --migrations-path=./migrations --config=./configs/config.yaml
migrate-up:
	go run ./cmd/migrator up --migrations-path=./migrations --config=./configs/config.yaml
migrate-up-1:
	go run ./cmd/migrator up --steps 1 --migrations-path=./migrations --config=./configs/config.yaml
migrate-down-1:
	go run ./cmd/migrator down --migrations-path=./migrations --config=./configs/config.yaml
migrate-drop:
	go run ./cmd/migrator drop --migrations-path=./migrations --config=./configs/config.yaml

run-osm:
	go run ./cmd/osm/
build-osm:
	go build ./cmd/osm/

swag:
	swag init -g ./cmd/rest/main.go --parseDependency --overridesFile .swaggo
	swag fmt