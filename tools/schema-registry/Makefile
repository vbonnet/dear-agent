include ../../mk/install-go-bin.mk

# GOPATH is frequently unset OR set to an empty string, in which case
# $(GOPATH)/bin expands to the bare "/bin" -- a system directory this target
# must never write to. `?=` only covers the undefined case, so normalise the
# empty one too, the way the go tool defaults it.
# `override` so a command-line `GOPATH=` or `make -e` with an empty value
# cannot put the bare "/bin" back: a plain assignment loses to both.
override GOPATH := $(if $(strip $(GOPATH)),$(strip $(GOPATH)),$(HOME)/go)

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
	$(call install-go-bin,cc,$(GOPATH)/bin)
	$(call install-go-bin,cc-mcp-server,$(GOPATH)/bin)

lint:
	golangci-lint run ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
