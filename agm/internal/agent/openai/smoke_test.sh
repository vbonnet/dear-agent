#!/bin/bash
# Smoke test for OpenAI client implementation
# Run this script to verify the implementation works correctly

set -e

cd "$(dirname "$0")"
MODULE_ROOT="../../../"

echo "========================================="
echo "OpenAI Client - Smoke Test"
echo "========================================="
echo ""

echo "1. Running go mod tidy..."
go mod tidy -C "$MODULE_ROOT"
echo "✅ go.mod is clean"
echo ""

echo "2. Building package..."
go build -C "$MODULE_ROOT" ./internal/agent/openai
echo "✅ Package builds successfully"
echo ""

echo "3. Running go vet..."
go vet -C "$MODULE_ROOT" ./internal/agent/openai/...
echo "✅ go vet passed"
echo ""

echo "4. Running unit tests..."
go test -C "$MODULE_ROOT" ./internal/agent/openai -v -short
echo "✅ All unit tests passed"
echo ""

echo "5. Checking test coverage..."
go test -C "$MODULE_ROOT" ./internal/agent/openai -cover -short
echo ""

echo "6. Verifying dependency is in go.mod..."
if grep -q "github.com/sashabaranov/go-openai" "$MODULE_ROOT/go.mod"; then
    echo "✅ go-openai dependency found in go.mod"
else
    echo "❌ go-openai dependency NOT found in go.mod"
    exit 1
fi
echo ""

echo "========================================="
echo "All smoke tests passed! ✅"
echo "========================================="
echo ""
echo "Optional: Run integration tests (requires OPENAI_API_KEY):"
echo "  export OPENAI_API_KEY=sk-..."
echo "  go test -C $MODULE_ROOT ./internal/agent/openai -v -run Integration"
echo ""
