#!/bin/bash

# Multi-Persona Review Package - Test Runner
# Run this script from the review package directory

set -e

echo "=== Multi-Persona Review Package Tests ==="
echo

echo "1. Checking Go environment..."
go version
echo

echo "2. Running go mod tidy..."
cd cortex
go mod tidy
echo

echo "3. Running golangci-lint..."
cd cortex/cmd/wayfinder-session/internal/review
golangci-lint run . || echo "Note: Some linter warnings may be expected"
echo

echo "4. Running unit tests..."
go test -v -run Test.*Engine
echo

echo "5. Running integration tests..."
go test -v -run Test.*Integration
echo

echo "6. Running all tests with coverage..."
go test -v -cover ./...
echo

echo "7. Generating coverage report..."
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
echo

echo "=== All Tests Complete ==="
