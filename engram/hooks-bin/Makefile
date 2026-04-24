.PHONY: build-linux build-macos test bench clean lint all generate check-generate lint-patterns

all: build-linux build-macos

build-linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-o bin/posttool-error-collector \
		cmd/posttool-error-collector/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-o bin/prepush-act-validator \
		cmd/prepush-act-validator/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-o bin/sessionstart-bead-coverage \
		cmd/sessionstart-bead-coverage/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-o bin/sessionend-bead-coverage \
		cmd/sessionend-bead-coverage/main.go

build-macos:
	@echo "Building for macOS (amd64)..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-o bin/posttool-error-collector-darwin-amd64 \
		cmd/posttool-error-collector/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-o bin/prepush-act-validator-darwin-amd64 \
		cmd/prepush-act-validator/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-o bin/sessionstart-bead-coverage-darwin-amd64 \
		cmd/sessionstart-bead-coverage/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-o bin/sessionend-bead-coverage-darwin-amd64 \
		cmd/sessionend-bead-coverage/main.go
	@echo "Building for macOS (arm64)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-o bin/posttool-error-collector-darwin-arm64 \
		cmd/posttool-error-collector/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-o bin/prepush-act-validator-darwin-arm64 \
		cmd/prepush-act-validator/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-o bin/sessionstart-bead-coverage-darwin-arm64 \
		cmd/sessionstart-bead-coverage/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-o bin/sessionend-bead-coverage-darwin-arm64 \
		cmd/sessionend-bead-coverage/main.go

test:
	@echo "Running tests with coverage..."
	go test ./cmd/sessionstart-bead-coverage -v -cover
	go test ./cmd/sessionend-bead-coverage -v -cover
	go test ./cmd/generate-patterns -v -cover
	go test ./cmd/integration_test -v -cover
	go test ./internal/... -v -cover

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/

generate:
	@echo "Generating patterns.go from YAML..."
	go generate ./internal/validator/

check-generate: generate
	@echo "Checking generated patterns.go is up to date..."
	git diff --exit-code internal/validator/patterns.go

lint-patterns:
	@echo "Linting pattern database..."
	go run ./cmd/generate-patterns -yaml ../patterns/bash-anti-patterns.yaml --lint

lint:
	@echo "Running linters..."
	go vet ./cmd/...
	gofmt -l ./cmd/
