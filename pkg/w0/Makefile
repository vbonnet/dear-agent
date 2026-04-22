.PHONY: test lint all

all: test lint

test:
	go test -v .

lint:
	golangci-lint run ./...
