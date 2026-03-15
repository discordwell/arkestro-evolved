SHELL := /bin/bash

BINARY := arkessro

.PHONY: dev

dev:
	@mkdir -p data
	@go run ./cmd/arkessro -db ./data/dev.db -addr 127.0.0.1:8080

.PHONY: test

test:
	@go test ./...

.PHONY: build

build:
	@go build -o $(BINARY) ./cmd/arkessro

.PHONY: fmt

fmt:
	@go fmt ./...
