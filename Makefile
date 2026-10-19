.PHONY: install build test

install:
	go install .

build:
	go build -o canvas .

test:
	go test ./...
