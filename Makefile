
test:
	go test ./... -count 1 -race

test-coverage:
	go test -coverprofile=coverage.txt ./... -count 1 -race

vet:
	go vet ./...

cover:
	go test ./... -count 1 -race -cover

gen:
	go generate ./...

lint:
	golangci-lint run ./...
