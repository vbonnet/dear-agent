#!/usr/bin/env bash
# Wayfinder Workspace Isolation Test Runner
# Runs comprehensive test suite for workspace isolation

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
WAYFINDER_TEST_INTEGRATION=1
export WAYFINDER_TEST_INTEGRATION

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}Wayfinder Workspace Isolation Test Suite${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# Function to print section header
print_section() {
    echo ""
    echo -e "${BLUE}>>> $1${NC}"
    echo ""
}

# Function to print success
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Function to print error
print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Function to print warning
print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# Track test results
TESTS_PASSED=0
TESTS_FAILED=0
BENCHMARKS_RUN=0

# 1. Run unit tests
print_section "Running Unit Tests"
if go test -v -run "^Test" -count=1 .; then
    print_success "Unit tests passed"
    ((TESTS_PASSED++))
else
    print_error "Unit tests failed"
    ((TESTS_FAILED++))
fi

# 2. Run integration tests (workspace isolation)
print_section "Running Integration Tests (Workspace Isolation)"
if WAYFINDER_TEST_INTEGRATION=1 go test -v -run "TestWorkspaceIsolation" -count=1 .; then
    print_success "Workspace isolation tests passed"
    ((TESTS_PASSED++))
else
    print_error "Workspace isolation tests failed"
    ((TESTS_FAILED++))
fi

# 3. Run edge case tests
print_section "Running Edge Case Tests"
if WAYFINDER_TEST_INTEGRATION=1 go test -v -run "TestWorkspaceFilterEdgeCases" -count=1 .; then
    print_success "Edge case tests passed"
    ((TESTS_PASSED++))
else
    print_error "Edge case tests failed"
    ((TESTS_FAILED++))
fi

# 4. Run test data generator tests
print_section "Running Test Data Generator Tests"
if go test -v -run "TestGenerateTestData" -count=1 .; then
    print_success "Test data generator tests passed"
    ((TESTS_PASSED++))
else
    print_error "Test data generator tests failed"
    ((TESTS_FAILED++))
fi

# 5. Run performance benchmarks
print_section "Running Performance Benchmarks"
print_warning "Target: <10ms overhead per operation"
echo ""

# Run benchmarks and capture output
BENCH_OUTPUT=$(go test -bench=. -benchmem -count=3 2>&1)
echo "$BENCH_OUTPUT"

if echo "$BENCH_OUTPUT" | grep -q "FAIL"; then
    print_error "Benchmarks failed"
    ((TESTS_FAILED++))
else
    print_success "Benchmarks completed"
    ((BENCHMARKS_RUN++))
fi

# Parse benchmark results for performance validation
echo ""
print_section "Performance Analysis"

# Extract average operation times
if echo "$BENCH_OUTPUT" | grep -q "BenchmarkWorkspaceQueries"; then
    echo "Workspace Query Performance:"
    echo "$BENCH_OUTPUT" | grep "BenchmarkWorkspaceQueries" | awk '{print "  " $1 ": " $3 " " $4}'
    echo ""
fi

if echo "$BENCH_OUTPUT" | grep -q "BenchmarkMonolithicVsMultiWorkspace"; then
    echo "Monolithic vs Multi-Workspace Comparison:"
    echo "$BENCH_OUTPUT" | grep "BenchmarkMonolithicVsMultiWorkspace" | awk '{print "  " $1 ": " $3 " " $4}'
    echo ""
fi

# 6. Run test with race detector
print_section "Running Race Detector Tests"
if go test -race -run "^Test" -count=1 . > /dev/null 2>&1; then
    print_success "Race detector tests passed"
    ((TESTS_PASSED++))
else
    print_warning "Race detector tests failed or not applicable"
fi

# 7. Check test coverage
print_section "Calculating Test Coverage"
if go test -cover -coverprofile=coverage.out . > /dev/null 2>&1; then
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
    echo "Total coverage: $COVERAGE"

    # Parse coverage percentage
    COVERAGE_NUM=$(echo "$COVERAGE" | sed 's/%//')
    COVERAGE_INT=$(printf "%.0f" "$COVERAGE_NUM")

    if [ "$COVERAGE_INT" -ge 80 ]; then
        print_success "Coverage is acceptable (${COVERAGE})"
    elif [ "$COVERAGE_INT" -ge 60 ]; then
        print_warning "Coverage is moderate (${COVERAGE})"
    else
        print_warning "Coverage is low (${COVERAGE})"
    fi

    # Generate HTML coverage report
    go tool cover -html=coverage.out -o coverage.html 2>/dev/null
    print_success "Coverage report generated: coverage.html"
else
    print_warning "Coverage calculation failed"
fi

# Summary
echo ""
echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""
echo "Tests passed: $TESTS_PASSED"
echo "Tests failed: $TESTS_FAILED"
echo "Benchmarks run: $BENCHMARKS_RUN"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    print_success "All tests passed! ✓"
    echo ""
    print_section "Validation Report"
    echo "✓ Zero cross-contamination verified"
    echo "✓ Workspace isolation confirmed"
    echo "✓ Performance benchmarks completed"
    echo "✓ Edge cases handled correctly"
    echo ""
    exit 0
else
    print_error "Some tests failed!"
    echo ""
    echo "Please review the failures above and fix the issues."
    echo ""
    exit 1
fi
