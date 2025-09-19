run-rest:
	go run ./cmd/rest/ --config=./configs/
build-rest:
	go build ./cmd/rest/

docker-rest:
	docker compose -f docker/rest/compose.yaml --project-directory . up --build -d
docker-grpc:
	docker compose -f docker/grpc/compose.yaml --project-directory . up --build -d   

run-grpc:
	go run ./cmd/grpc/ --config=./configs/
build-grpc:
	go build ./cmd/grpc/

test:
	go test ./...

test-cover:
	go test ./... -coverprofile cover.test.tmp -coverpkg ./...
	type cover.test.tmp | findstr -v "mocks" > cover.test 
	del cover.test.tmp 
	go tool cover -func cover.test 

migrate:
	migrate create -ext=sql -dir=./migrations -seq ${NAME_MIGRATION}     
migrate-up:
	migrate -path ./migrations/ -database postgres://postgres:postgres@localhost/problem_map?sslmode=disable up
migrate-down:
	migrate -path ./migrations/ -database postgres://postgres:postgres@localhost/problem_map?sslmode=disable down
