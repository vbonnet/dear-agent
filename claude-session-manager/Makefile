.PHONY: build test lint install clean

build:
	go build -ldflags="-s -w" -o bin/csm ./cmd/csm

test:
	go test -v -cover ./...

lint:
	go fmt ./...
	go vet ./...

install: build
	@# Clean up old binaries from incorrect locations to prevent confusion
	@rm -f $(HOME)/bin/csm $(HOME)/.local/bin/csm 2>/dev/null || true
	@# Install to standard Go binary location
	mkdir -p $(HOME)/go/bin
	cp bin/csm $(HOME)/go/bin/csm
	@echo "✓ csm binary installed to $(HOME)/go/bin/csm"
	@echo ""
	@echo "Cleaned up old binaries from:"
	@echo "  - $(HOME)/bin/csm (removed if existed)"
	@echo "  - $(HOME)/.local/bin/csm (removed if existed)"
	@echo ""
	@echo "To install slash commands, add this repo as a plugin marketplace:"
	@echo "  /plugin marketplace add $(shell pwd)"
	@echo "  /plugin install csm-tools@ai-tools"
	@echo ""
	@echo "See PLUGIN-INSTALLATION.md for details"
	@echo ""
	@echo "Make sure $(HOME)/go/bin is in your PATH"

clean:
	rm -rf bin/
