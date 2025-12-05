.PHONY: build test lint install clean

build:
	go build -ldflags="-s -w" -o bin/csm ./cmd/csm

test:
	go test -v -cover ./...

lint:
	go fmt ./...
	go vet ./...

install: build
	mkdir -p ~/.local/bin
	cp bin/csm ~/.local/bin/csm
	@echo "csm installed to ~/.local/bin/csm"
	@echo "Make sure ~/.local/bin is in your PATH"

clean:
	rm -rf bin/
