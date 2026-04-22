#!/bin/bash
# Script to run validate command integration tests
# Location: ./worktrees/engram/phase4-final/core/cmd/engram/cmd/

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "================================================"
echo "Engram Validate - Integration Test Suite"
echo "================================================"
echo ""

# Change to core directory
cd "$(dirname "$0")/../../../"

echo "Working directory: $(pwd)"
echo ""

# Function to run tests
run_test() {
    local name="$1"
    local cmd="$2"

    echo -e "${YELLOW}Running: ${name}${NC}"
    echo "Command: ${cmd}"
    echo ""

    if eval "${cmd}"; then
        echo -e "${GREEN}✓ ${name} PASSED${NC}"
    else
        echo -e "${RED}✗ ${name} FAILED${NC}"
        return 1
    fi
    echo ""
}

# Menu
if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
    echo "Usage: $0 [option]"
    echo ""
    echo "Options:"
    echo "  --all              Run all tests (unit + integration)"
    echo "  --integration      Run only integration tests"
    echo "  --unit             Run only unit tests"
    echo "  --short            Run quick tests (skip integration)"
    echo "  --corpus           Run corpus validation test"
    echo "  --performance      Run performance benchmark"
    echo "  --coverage         Generate coverage report"
    echo "  --list             List all test functions"
    echo "  --help, -h         Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --all                    # Run all tests"
    echo "  $0 --integration            # Run integration tests only"
    echo "  $0 --short                  # Quick tests for pre-commit"
    echo ""
    exit 0
fi

case "$1" in
    --all)
        echo "Running all tests (unit + integration)..."
        echo ""
        run_test "All Tests" "go test -v ./cmd/engram/cmd -timeout 5m"
        ;;

    --integration)
        echo "Running integration tests only..."
        echo ""
        run_test "Integration Tests" "go test -v ./cmd/engram/cmd -run TestIntegration_ -timeout 5m"
        ;;

    --unit)
        echo "Running unit tests only..."
        echo ""
        run_test "Unit Tests" "go test -v ./cmd/engram/cmd -run '^Test[^I]' -timeout 2m"
        ;;

    --short)
        echo "Running quick tests (skip integration)..."
        echo ""
        run_test "Short Tests" "go test -short ./cmd/engram/cmd"
        ;;

    --corpus)
        echo "Running corpus validation test (500+ files)..."
        echo ""
        run_test "Corpus Validation" "go test -v ./cmd/engram/cmd -run TestIntegration_ValidateRealCorpus -timeout 2m"
        ;;

    --performance)
        echo "Running performance benchmark (<10s for 500+ files)..."
        echo ""
        run_test "Performance Benchmark" "go test -v ./cmd/engram/cmd -run TestIntegration_PerformanceBenchmark -timeout 30s"
        ;;

    --coverage)
        echo "Generating coverage report..."
        echo ""
        go test -coverprofile=coverage.out ./cmd/engram/cmd
        go tool cover -html=coverage.out -o coverage.html
        echo ""
        echo -e "${GREEN}✓ Coverage report generated: coverage.html${NC}"
        echo ""
        go tool cover -func=coverage.out | tail -1
        ;;

    --list)
        echo "Test Functions:"
        echo ""
        echo "Unit Tests (validate_test.go):"
        grep "^func Test" cmd/engram/cmd/validate_test.go | sed 's/func /  - /' | sed 's/(t \*testing.T).*//'
        echo ""
        echo "Integration Tests (validate_integration_test.go):"
        grep "^func TestIntegration_" cmd/engram/cmd/validate_integration_test.go | sed 's/func /  - /' | sed 's/(t \*testing.T).*//'
        echo ""
        echo "Total: $(grep -c '^func Test' cmd/engram/cmd/validate_test.go cmd/engram/cmd/validate_integration_test.go) test functions"
        ;;

    *)
        echo "Running all tests by default (use --help for options)..."
        echo ""
        run_test "All Tests" "go test -v ./cmd/engram/cmd -timeout 5m"
        ;;
esac

echo ""
echo "================================================"
echo "Test run complete!"
echo "================================================"
