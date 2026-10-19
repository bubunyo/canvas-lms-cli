VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/bubunyo/canvas-lms-cli/internal/cli.Version=$(VERSION)

.PHONY: install build test

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/canvas-cli

build:
	go build -ldflags "$(LDFLAGS)" -o canvas-cli ./cmd/canvas-cli

test:
	go test ./...
