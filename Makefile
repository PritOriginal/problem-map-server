run-rest:
	go run ./cmd/rest/ --config=./configs/
build-rest:
	go build ./cmd/rest/


test:
	go test ./...

test-cover:
	go test ./... -coverprofile cover.test.tmp -coverpkg ./...
	type cover.test.tmp | findstr -v "mocks" > cover.test 
	del cover.test.tmp 
	go tool cover -func cover.test 