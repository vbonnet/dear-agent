#!/bin/bash
# Run AGM session lifecycle integration tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "====================================="
echo "AGM Session Lifecycle Test Suite"
echo "====================================="
echo ""

# Check prerequisites
echo "Checking prerequisites..."

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}ERROR: Go is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓${NC} Go installed: $(go version)"

# Check tmux
if ! command -v tmux &> /dev/null; then
    echo -e "${YELLOW}WARNING: tmux not installed - tmux-dependent tests will be skipped${NC}"
else
    echo -e "${GREEN}✓${NC} tmux installed: $(tmux -V)"
fi

# Check AGM binary
if ! command -v agm &> /dev/null && [ ! -f "$PROJECT_ROOT/agm" ]; then
    echo -e "${YELLOW}WARNING: agm binary not found - building...${NC}"
    cd "$PROJECT_ROOT"
    make build || {
        echo -e "${RED}ERROR: Failed to build AGM${NC}"
        exit 1
    }
fi
echo -e "${GREEN}✓${NC} AGM binary available"

echo ""
echo "====================================="
echo "Running Tests"
echo "====================================="
echo ""

cd "$PROJECT_ROOT"

# Parse command line arguments
MODE="${1:-all}"
VERBOSE="${2:-}"

case "$MODE" in
    all)
        echo "Running all lifecycle tests..."
        go test ./test/integration/lifecycle/... -v $VERBOSE
        ;;
    short)
        echo "Running short lifecycle tests..."
        go test ./test/integration/lifecycle/... -v -short $VERBOSE
        ;;
    coverage)
        echo "Running tests with coverage..."
        go test ./test/integration/lifecycle/... -v -coverprofile=lifecycle-coverage.out $VERBOSE
        echo ""
        echo "Generating coverage report..."
        go tool cover -html=lifecycle-coverage.out -o lifecycle-coverage.html
        echo -e "${GREEN}✓${NC} Coverage report: lifecycle-coverage.html"
        ;;
    race)
        echo "Running tests with race detector..."
        go test ./test/integration/lifecycle/... -v -race $VERBOSE
        ;;
    specific)
        TEST_NAME="${2:-}"
        if [ -z "$TEST_NAME" ]; then
            echo -e "${RED}ERROR: No test name specified${NC}"
            echo "Usage: $0 specific TestName"
            exit 1
        fi
        echo "Running test: $TEST_NAME"
        go test ./test/integration/lifecycle/... -v -run "$TEST_NAME"
        ;;
    help)
        echo "AGM Lifecycle Test Runner"
        echo ""
        echo "Usage: $0 [mode] [options]"
        echo ""
        echo "Modes:"
        echo "  all       - Run all lifecycle tests (default)"
        echo "  short     - Run only short/fast tests"
        echo "  coverage  - Run tests with coverage report"
        echo "  race      - Run tests with race detector"
        echo "  specific  - Run specific test by name"
        echo "  help      - Show this help message"
        echo ""
        echo "Examples:"
        echo "  $0                                    # Run all tests"
        echo "  $0 short                              # Run short tests"
        echo "  $0 coverage                           # Generate coverage report"
        echo "  $0 specific TestSessionCreation_FullLifecycle"
        echo ""
        exit 0
        ;;
    *)
        echo -e "${RED}ERROR: Unknown mode: $MODE${NC}"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac

TEST_EXIT_CODE=$?

echo ""
echo "====================================="
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}Tests PASSED${NC}"
else
    echo -e "${RED}Tests FAILED${NC}"
fi
echo "====================================="

exit $TEST_EXIT_CODE
