#!/bin/bash
set -e

echo "Running w0 package tests..."
go test -v .

echo ""
echo "Running golangci-lint..."
golangci-lint run ./...

echo ""
echo "All checks passed!"
