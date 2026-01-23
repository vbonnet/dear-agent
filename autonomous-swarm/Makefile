.PHONY: build test install clean

# Version information
VERSION ?= 0.1.0-dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Ldflags for version injection
LDFLAGS = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

build:
	go build -ldflags="$(LDFLAGS)" -o bin/swarm-executor ./cmd/swarm-executor

test:
	go test -v -cover ./...

install: build
	@# Clean up old binaries from incorrect locations to prevent confusion
	@rm -f $(HOME)/bin/swarm-executor $(HOME)/.local/bin/swarm-executor 2>/dev/null || true
	@# Install to standard Go binary location
	mkdir -p $(HOME)/go/bin
	cp bin/swarm-executor $(HOME)/go/bin/swarm-executor
	@echo "✓ swarm-executor binary installed to $(HOME)/go/bin/swarm-executor"

clean:
	rm -rf bin/
