# Makefile for diagram-as-code skills
# Task 9.2: Cross-CLI compatibility testing

.PHONY: all build test clean help

SKILLS := review-diagrams create-diagrams render-diagrams diagram-sync
BUILD_DIR := bin

all: build

help:
	@echo "Diagram-as-Code Skills - Build System"
	@echo ""
	@echo "Targets:"
	@echo "  make build        - Build all skills"
	@echo "  make test         - Run tests for all skills"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make help         - Show this help"
	@echo ""
	@echo "Individual builds:"
	@echo "  make review-diagrams"
	@echo "  make create-diagrams"
	@echo "  make render-diagrams"
	@echo "  make diagram-sync"

build: $(SKILLS)
	@echo "✓ All skills built successfully"

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

review-diagrams: $(BUILD_DIR)
	@echo "Building review-diagrams..."
	@go -C skills/review-diagrams/cmd/review-diagrams build -o $(PWD)/$(BUILD_DIR)/review-diagrams
	@echo "✓ review-diagrams built"

create-diagrams: $(BUILD_DIR)
	@echo "Building create-diagrams..."
	@go -C skills/create-diagrams/cmd/create-diagrams build -o $(PWD)/$(BUILD_DIR)/create-diagrams
	@echo "✓ create-diagrams built"

render-diagrams: $(BUILD_DIR)
	@echo "Building render-diagrams..."
	@go -C skills/render-diagrams/cmd/render-diagrams build -o $(PWD)/$(BUILD_DIR)/render-diagrams
	@echo "✓ render-diagrams built"

diagram-sync: $(BUILD_DIR)
	@echo "Building diagram-sync..."
	@go -C skills/diagram-sync/cmd/diagram-sync build -o $(PWD)/$(BUILD_DIR)/diagram-sync
	@echo "✓ diagram-sync built"

test:
	@echo "Running tests..."
	@go -C lib/diagram/c4model test ./...
	@echo "✓ Tests passed"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@find skills -name "*.test" -delete
	@echo "✓ Clean complete"

.PHONY: $(SKILLS)
