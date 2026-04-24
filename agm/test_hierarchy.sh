#!/bin/bash
# Test script for hierarchy functionality
# Run this from the repository root: ./test_hierarchy.sh

set -e

echo "==================================="
echo "Testing Hierarchy Functionality"
echo "==================================="
echo ""

echo "1. Testing database hierarchy functions..."
go test ./internal/db -run TestGetAllSessionsHierarchy -v -cover
go test ./internal/db -run TestGetChildren -v -cover
go test ./internal/db -run TestGetParent -v -cover
go test ./internal/db -run TestGetSessionTree -v -cover
echo ""

echo "2. Testing UI hierarchy rendering..."
go test ./internal/ui -run TestFormatTableWithHierarchy -v -cover
go test ./internal/ui -run TestRenderHierarchy -v -cover
echo ""

echo "3. Running all hierarchy tests with coverage..."
go test ./internal/db -run Hierarchy -coverprofile=coverage-db.out
go test ./internal/ui -run Hierarchy -coverprofile=coverage-ui.out
echo ""

echo "4. Coverage report for database layer:"
go tool cover -func=coverage-db.out | grep -E "(GetAllSessionsHierarchy|GetChildren|GetParent|GetSessionTree)"
echo ""

echo "5. Coverage report for UI layer:"
go tool cover -func=coverage-ui.out | grep -E "(FormatTableWithHierarchy|renderHierarchy)"
echo ""

echo "==================================="
echo "All hierarchy tests completed!"
echo "==================================="
