BINARY_NAME=byzclaw
BUILD_FLAGS=-ldflags="-s -w" -trimpath

.PHONY: build test vet tidy

tidy:
	go mod tidy

build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/$(BINARY_NAME) ./cmd/byzclaw

test:
	go test ./...

vet:
	go vet ./...
