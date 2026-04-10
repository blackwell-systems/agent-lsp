.PHONY: build test lint vet

build:
	GOWORK=off go build ./...

test:
	GOWORK=off go test ./internal/... ./cmd/...

lint:
	GOWORK=off go vet ./...

vet: lint
