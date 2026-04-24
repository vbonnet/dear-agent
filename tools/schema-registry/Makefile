.PHONY: all build test clean install lint

all: build

build:
	go build -o cc ./cmd/cc
	go build -o cc-mcp-server ./cmd/cc-mcp-server

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f cc cc-mcp-server coverage.out coverage.html
	rm -rf ~/.config/corpus-callosum/test*

install: build
	cp cc $(GOPATH)/bin/
	cp cc-mcp-server $(GOPATH)/bin/

lint:
	golangci-lint run ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
