#!/usr/bin/env bash
#
# Test Engram MCP Server - Basic Tools (Task 3.2)
#
# Tests the 3 required tools:
# 1. engram_retrieve
# 2. engram_plugins_list
# 3. wayfinder_phase_status

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_SCRIPT="$SCRIPT_DIR/engram_mcp_server.py"

echo "=== Engram MCP Server - Basic Tools Test ==="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Test counter
TESTS_RUN=0
TESTS_PASSED=0

run_test() {
    local test_name="$1"
    local request="$2"
    local expected_pattern="$3"

    TESTS_RUN=$((TESTS_RUN + 1))
    echo "Test $TESTS_RUN: $test_name"

    # Send request to server
    response=$(echo "$request" | python3 "$SERVER_SCRIPT" 2>/dev/null | head -1)

    # Check for error
    if echo "$response" | grep -q '"error"'; then
        echo -e "${RED}✗ FAILED${NC}"
        echo "  Error: $response"
        return 1
    fi

    # Check expected pattern
    if echo "$response" | grep -q "$expected_pattern"; then
        echo -e "${GREEN}✓ PASSED${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}✗ FAILED${NC}"
        echo "  Expected pattern: $expected_pattern"
        echo "  Got: $response"
        return 1
    fi
}

echo "--- Tool 1: engram_retrieve ---"
run_test "engram.retrieve - basic query" \
    '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"engram_retrieve","arguments":{"query":"error handling","top_k":3}}}' \
    '"content"'

echo ""

echo "--- Tool 2: engram_plugins_list ---"
run_test "engram.plugins.list - list plugins" \
    '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"engram_plugins_list","arguments":{}}}' \
    '"content"'

echo ""

echo "--- Tool 3: wayfinder_phase_status ---"
# Use workflow-improvements-mcp project as test case
TEST_PROJECT="$HOME/src/ws/oss/wf/workflow-improvements-mcp"

if [ -d "$TEST_PROJECT" ]; then
    run_test "wayfinder.phase.status - get status" \
        '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"wayfinder_phase_status","arguments":{"project_path":"'"$TEST_PROJECT"'"}}}' \
        '"content"'
else
    echo "Skipping (project not found: $TEST_PROJECT)"
fi

echo ""
echo "=== Test Summary ==="
echo "Tests run: $TESTS_RUN"
echo "Tests passed: $TESTS_PASSED"

if [ "$TESTS_PASSED" -eq "$TESTS_RUN" ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
