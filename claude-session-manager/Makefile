.PHONY: build test lint install clean

build:
	go build -ldflags="-s -w" -o bin/csm ./cmd/csm

test:
	go test -v -cover ./...

lint:
	go fmt ./...
	go vet ./...

install: build
	mkdir -p $(HOME)/.local/bin
	mkdir -p $(HOME)/go/bin
	cp bin/csm $(HOME)/.local/bin/csm
	cp bin/csm $(HOME)/go/bin/csm
	@echo "✓ csm binary installed to $(HOME)/.local/bin/csm and $(HOME)/go/bin/csm"
	@echo ""
	@echo "To install slash commands, add this repo as a plugin marketplace:"
	@echo "  /plugin marketplace add $(shell pwd)"
	@echo "  /plugin install csm-tools@ai-tools"
	@echo ""
	@echo "See PLUGIN-INSTALLATION.md for details"
	@echo ""
	@echo "Make sure $(HOME)/.local/bin or $(HOME)/go/bin is in your PATH"

clean:
	rm -rf bin/
