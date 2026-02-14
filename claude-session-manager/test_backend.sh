#!/bin/bash
# Test script for backend package

set -e

cd "$(dirname "$0")"

echo "Testing backend package..."
go test -v ./internal/backend/...

echo ""
echo "Testing integration tests..."
go test -v ./test/integration/backend_switching_test.go

echo ""
echo "All backend tests passed!"
