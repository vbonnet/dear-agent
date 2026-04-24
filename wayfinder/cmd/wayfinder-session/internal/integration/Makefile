# Makefile for Wayfinder V2 Integration Tests

.PHONY: all test test-short test-e2e test-d4 test-s6 test-s8 test-risk test-transitions test-schema build clean coverage help

# Default target
all: build test

# Build wayfinder-session binary
build:
	@echo "Building wayfinder-session..."
	@cd ../.. && go build -o /tmp/wayfinder-session ./cmd/wayfinder-session
	@echo "✅ Build complete: /tmp/wayfinder-session"

# Run all integration tests
test: build
	@echo "Running all integration tests..."
	@export PATH="/tmp:$$PATH" && go test -v ./...

# Run tests in short mode (skip integration)
test-short:
	@echo "Running tests in short mode..."
	@go test -short -v ./...

# Run specific test suites
test-e2e: build
	@echo "Running E2E full workflow test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestE2E_V2FullWorkflow

test-d4: build
	@echo "Running D4 stakeholder approval test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestD4_StakeholderApproval

test-s6: build
	@echo "Running S6 TESTS.feature generation test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestS6_TestsFeatureGeneration

test-s8: build
	@echo "Running S8 BUILD loop test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestS8_BuildLoop

test-risk: build
	@echo "Running risk-adaptive review test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestRiskAdaptiveReview

test-transitions: build
	@echo "Running phase transitions test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestPhaseTransitions

test-schema: build
	@echo "Running schema validation test..."
	@export PATH="/tmp:$$PATH" && go test -v ./... -run TestSchemaValidation

# Run tests with coverage
coverage: build
	@echo "Running tests with coverage..."
	@export PATH="/tmp:$$PATH" && go test -v -cover -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f /tmp/wayfinder-session
	@rm -f coverage.out coverage.html
	@echo "✅ Clean complete"

# Show help
help:
	@echo "Wayfinder V2 Integration Tests - Available Targets:"
	@echo ""
	@echo "  make              - Build and run all tests"
	@echo "  make build        - Build wayfinder-session binary"
	@echo "  make test         - Run all integration tests"
	@echo "  make test-short   - Run tests in short mode (skip integration)"
	@echo "  make test-e2e     - Run E2E full workflow test"
	@echo "  make test-d4      - Run D4 stakeholder approval test"
	@echo "  make test-s6      - Run S6 TESTS.feature generation test"
	@echo "  make test-s8      - Run S8 BUILD loop test"
	@echo "  make test-risk    - Run risk-adaptive review test"
	@echo "  make test-transitions - Run phase transitions test"
	@echo "  make test-schema  - Run schema validation test"
	@echo "  make coverage     - Run tests with coverage report"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make help         - Show this help message"
