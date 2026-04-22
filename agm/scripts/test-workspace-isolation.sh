#!/bin/bash
# AGM Multi-Workspace Isolation Test Runner
# Task 3.4: AGM Multi-Workspace Testing (bead: oss-6xkh)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
OSS_WORKSPACE_DIR="${OSS_WORKSPACE_DIR:-$HOME/src/ws/oss}"
SECONDARY_WORKSPACE_DIR="${SECONDARY_WORKSPACE_DIR:-$HOME/src/ws/secondary}"
OSS_PORT="${OSS_PORT:-3307}"
SECONDARY_PORT="${SECONDARY_PORT:-3308}"
DOLT_BIN="${DOLT_BIN:-dolt}"

# PIDs for cleanup
OSS_DOLT_PID=""
SECONDARY_DOLT_PID=""

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up Dolt servers...${NC}"

    if [ -n "$OSS_DOLT_PID" ]; then
        echo "Stopping OSS Dolt server (PID: $OSS_DOLT_PID)"
        kill $OSS_DOLT_PID 2>/dev/null || true
    fi

    if [ -n "$SECONDARY_DOLT_PID" ]; then
        echo "Stopping Secondary Dolt server (PID: $SECONDARY_DOLT_PID)"
        kill $SECONDARY_DOLT_PID 2>/dev/null || true
    fi

    # Wait for processes to die
    sleep 2

    # Force kill if still running
    if [ -n "$OSS_DOLT_PID" ]; then
        kill -9 $OSS_DOLT_PID 2>/dev/null || true
    fi
    if [ -n "$SECONDARY_DOLT_PID" ]; then
        kill -9 $SECONDARY_DOLT_PID 2>/dev/null || true
    fi

    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set up trap for cleanup
trap cleanup EXIT INT TERM

# Print header
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}AGM Multi-Workspace Isolation Test Suite${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check if Dolt is installed
echo -e "${YELLOW}Checking Dolt installation...${NC}"
if ! command -v $DOLT_BIN &> /dev/null; then
    echo -e "${RED}ERROR: Dolt not found in PATH${NC}"
    echo "Please install Dolt from https://github.com/dolthub/dolt/releases"
    exit 1
fi
DOLT_VERSION=$($DOLT_BIN version | head -n1)
echo -e "${GREEN}✓ Dolt installed: $DOLT_VERSION${NC}"
echo ""

# Check workspace directories
echo -e "${YELLOW}Checking workspace directories...${NC}"

if [ ! -d "$OSS_WORKSPACE_DIR" ]; then
    echo -e "${RED}ERROR: OSS workspace directory not found: $OSS_WORKSPACE_DIR${NC}"
    exit 1
fi
echo -e "${GREEN}✓ OSS workspace: $OSS_WORKSPACE_DIR${NC}"

SECONDARY_EXISTS=false
if [ -d "$SECONDARY_WORKSPACE_DIR" ]; then
    SECONDARY_EXISTS=true
    echo -e "${GREEN}✓ Secondary workspace: $SECONDARY_WORKSPACE_DIR${NC}"
else
    echo -e "${YELLOW}⚠ Secondary workspace not found: $SECONDARY_WORKSPACE_DIR${NC}"
    echo -e "${YELLOW}  Tests will run with OSS workspace only${NC}"
fi
echo ""

# Initialize Dolt databases if needed
echo -e "${YELLOW}Initializing Dolt databases...${NC}"

if [ ! -d "$OSS_WORKSPACE_DIR/.dolt" ]; then
    echo "Initializing OSS workspace Dolt database..."
    cd "$OSS_WORKSPACE_DIR"
    $DOLT_BIN init
fi
echo -e "${GREEN}✓ OSS Dolt database ready${NC}"

if [ "$SECONDARY_EXISTS" = true ] && [ ! -d "$SECONDARY_WORKSPACE_DIR/.dolt" ]; then
    echo "Initializing Secondary workspace Dolt database..."
    cd "$SECONDARY_WORKSPACE_DIR"
    $DOLT_BIN init
fi
if [ "$SECONDARY_EXISTS" = true ]; then
    echo -e "${GREEN}✓ Secondary Dolt database ready${NC}"
fi
echo ""

# Check for port conflicts
echo -e "${YELLOW}Checking for port conflicts...${NC}"

if lsof -Pi :$OSS_PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo -e "${RED}ERROR: Port $OSS_PORT is already in use${NC}"
    echo "Please stop the process using this port or choose a different port"
    lsof -Pi :$OSS_PORT -sTCP:LISTEN
    exit 1
fi
echo -e "${GREEN}✓ Port $OSS_PORT is available${NC}"

if [ "$SECONDARY_EXISTS" = true ]; then
    if lsof -Pi :$SECONDARY_PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${RED}ERROR: Port $SECONDARY_PORT is already in use${NC}"
        echo "Please stop the process using this port or choose a different port"
        lsof -Pi :$SECONDARY_PORT -sTCP:LISTEN
        exit 1
    fi
    echo -e "${GREEN}✓ Port $SECONDARY_PORT is available${NC}"
fi
echo ""

# Start Dolt SQL servers
echo -e "${YELLOW}Starting Dolt SQL servers...${NC}"

echo "Starting OSS Dolt server on port $OSS_PORT..."
cd "$OSS_WORKSPACE_DIR"
$DOLT_BIN sql-server --port $OSS_PORT --host 127.0.0.1 --user root --password "" --loglevel info > /tmp/dolt-oss.log 2>&1 &
OSS_DOLT_PID=$!
echo -e "${GREEN}✓ OSS Dolt server started (PID: $OSS_DOLT_PID)${NC}"

if [ "$SECONDARY_EXISTS" = true ]; then
    echo "Starting Secondary Dolt server on port $SECONDARY_PORT..."
    cd "$SECONDARY_WORKSPACE_DIR"
    $DOLT_BIN sql-server --port $SECONDARY_PORT --host 127.0.0.1 --user root --password "" --loglevel info > /tmp/dolt-secondary.log 2>&1 &
    SECONDARY_DOLT_PID=$!
    echo -e "${GREEN}✓ Secondary Dolt server started (PID: $SECONDARY_DOLT_PID)${NC}"
fi
echo ""

# Wait for servers to be ready
echo -e "${YELLOW}Waiting for Dolt servers to be ready...${NC}"
sleep 3

# Test OSS connection
echo "Testing OSS Dolt connection..."
if ! mysql -h 127.0.0.1 -P $OSS_PORT -u root -e "SELECT 1" >/dev/null 2>&1; then
    echo -e "${RED}ERROR: Cannot connect to OSS Dolt server${NC}"
    echo "Check logs: /tmp/dolt-oss.log"
    exit 1
fi
echo -e "${GREEN}✓ OSS Dolt server is ready${NC}"

if [ "$SECONDARY_EXISTS" = true ]; then
    echo "Testing Secondary Dolt connection..."
    if ! mysql -h 127.0.0.1 -P $SECONDARY_PORT -u root -e "SELECT 1" >/dev/null 2>&1; then
        echo -e "${RED}ERROR: Cannot connect to Secondary Dolt server${NC}"
        echo "Check logs: /tmp/dolt-secondary.log"
        exit 1
    fi
    echo -e "${GREEN}✓ Secondary Dolt server is ready${NC}"
fi
echo ""

# Run Go tests
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Running Workspace Isolation Tests${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

cd "$PROJECT_ROOT"

# Set test environment
export DOLT_TEST_INTEGRATION=1
export DOLT_HOST=127.0.0.1

# Test 1: Unit tests (no Dolt server required)
echo -e "${YELLOW}Running unit tests...${NC}"
if go test -v ./internal/dolt -run "TestNew|TestDefaultConfig|TestBuildDSN"; then
    echo -e "${GREEN}✓ Unit tests passed${NC}"
else
    echo -e "${RED}✗ Unit tests failed${NC}"
    exit 1
fi
echo ""

# Test 2: Integration tests
if [ "$SECONDARY_EXISTS" = true ]; then
    echo -e "${YELLOW}Running full workspace isolation tests (OSS + Secondary)...${NC}"
    export WORKSPACE=oss
    export DOLT_PORT=$OSS_PORT

    if go test -v ./internal/dolt -run TestWorkspaceIsolation; then
        echo -e "${GREEN}✓ Workspace isolation tests passed${NC}"
    else
        echo -e "${RED}✗ Workspace isolation tests failed${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}Running single workspace tests (OSS only)...${NC}"
    export WORKSPACE=oss
    export DOLT_PORT=$OSS_PORT

    if go test -v ./internal/dolt -run TestSessionCRUD; then
        echo -e "${GREEN}✓ Session CRUD tests passed${NC}"
    else
        echo -e "${RED}✗ Session CRUD tests failed${NC}"
        exit 1
    fi
fi
echo ""

# Test 3: Edge case tests
echo -e "${YELLOW}Running edge case tests...${NC}"
export WORKSPACE=oss
export DOLT_PORT=$OSS_PORT

if go test -v ./internal/dolt -run TestWorkspaceFilterEdgeCases; then
    echo -e "${GREEN}✓ Edge case tests passed${NC}"
else
    echo -e "${RED}✗ Edge case tests failed${NC}"
    exit 1
fi
echo ""

# Test 4: Performance benchmarks (optional)
if [ "${RUN_BENCHMARKS:-false}" = "true" ]; then
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Running Performance Benchmarks${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    export WORKSPACE=oss
    export DOLT_PORT=$OSS_PORT

    go test -v ./internal/dolt -bench=BenchmarkWorkspaceQueries -benchtime=5s | tee /tmp/benchmark-results.txt
    echo ""
    echo -e "${GREEN}✓ Benchmark results saved to /tmp/benchmark-results.txt${NC}"
fi

# Summary
echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${GREEN}✓ All workspace isolation tests passed${NC}"
echo ""
echo "Test Configuration:"
echo "  - OSS Workspace: $OSS_WORKSPACE_DIR (port $OSS_PORT)"
if [ "$SECONDARY_EXISTS" = true ]; then
    echo "  - Secondary Workspace: $SECONDARY_WORKSPACE_DIR (port $SECONDARY_PORT)"
else
    echo "  - Secondary Workspace: Not configured"
fi
echo ""
echo "Logs:"
echo "  - OSS Dolt: /tmp/dolt-oss.log"
if [ "$SECONDARY_EXISTS" = true ]; then
    echo "  - Secondary Dolt: /tmp/dolt-secondary.log"
fi
if [ "${RUN_BENCHMARKS:-false}" = "true" ]; then
    echo "  - Benchmarks: /tmp/benchmark-results.txt"
fi
echo ""

# Cleanup will happen via trap
echo -e "${YELLOW}Press Ctrl+C to stop Dolt servers and exit${NC}"
echo -e "${YELLOW}Or wait 5 seconds for automatic cleanup...${NC}"
sleep 5
