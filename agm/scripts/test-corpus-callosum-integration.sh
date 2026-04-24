#!/usr/bin/env bash
# Test AGM Corpus Callosum Integration
# Verifies that AGM schema is registered and queryable

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CC_BIN="${CC_BIN:-cc}"  # Allow override via environment variable

echo "Testing AGM Corpus Callosum Integration"
echo "========================================"
echo

# Check if Corpus Callosum CLI is available
if ! command -v "${CC_BIN}" &> /dev/null; then
    echo "ERROR: Corpus Callosum CLI (${CC_BIN}) not found in PATH"
    echo "Please build and install corpus-callosum first:"
    echo "  cd ../corpus-callosum"
    echo "  make build && make install"
    exit 1
fi

# Test 1: Check version
echo "Test 1: Check Corpus Callosum version"
if ! "${CC_BIN}" version &> /dev/null; then
    echo "ERROR: Failed to get Corpus Callosum version"
    exit 1
fi
echo "✓ Corpus Callosum CLI is working"
echo

# Test 2: Discover AGM component
echo "Test 2: Discover AGM component"
if ! "${CC_BIN}" discover --workspace oss 2>/dev/null | grep -q '"component": "agm"'; then
    echo "ERROR: AGM component not found in registry"
    echo "Run: ./scripts/register-corpus-callosum.sh"
    exit 1
fi
echo "✓ AGM component is registered"
echo

# Test 3: Get AGM schema
echo "Test 3: Get AGM schema"
SCHEMA_OUTPUT=$("${CC_BIN}" schema --component agm --workspace oss 2>/dev/null || true)
if [ -z "$SCHEMA_OUTPUT" ]; then
    echo "ERROR: Failed to retrieve AGM schema"
    exit 1
fi

# Verify schema contains expected definitions
if ! echo "$SCHEMA_OUTPUT" | grep -q '"session"' || ! echo "$SCHEMA_OUTPUT" | grep -q '"message"'; then
    echo "ERROR: AGM schema missing expected definitions (session, message)"
    exit 1
fi
echo "✓ AGM schema retrieved successfully"
echo "  - Found 'session' schema"
echo "  - Found 'message' schema"
echo

# Test 4: Validate session data
echo "Test 4: Validate sample session data"
TEST_DATA="/tmp/agm-test-session-$$.json"
cat > "$TEST_DATA" << 'EOF'
{
  "id": "test-550e8400-e29b-41d4-a716-446655440000",
  "name": "test-corpus-callosum-integration",
  "timestamp": 1708300800000,
  "agent_type": "claude",
  "model": "claude-sonnet-4.5",
  "mode": "implementer",
  "status": "active",
  "workspace": "oss"
}
EOF

if ! "${CC_BIN}" validate --component agm --schema session --data "$TEST_DATA" --workspace oss 2>/dev/null | grep -q '"status": "valid"'; then
    echo "ERROR: Session data validation failed"
    rm -f "$TEST_DATA"
    exit 1
fi
rm -f "$TEST_DATA"
echo "✓ Session data validation passed"
echo

# Test 5: Validate message data
echo "Test 5: Validate sample message data"
TEST_DATA="/tmp/agm-test-message-$$.json"
cat > "$TEST_DATA" << 'EOF'
{
  "id": "msg-660f8511-f39c-51e5-b827-557766551111",
  "session_id": "test-550e8400-e29b-41d4-a716-446655440000",
  "role": "user",
  "content": "Test message for Corpus Callosum integration",
  "timestamp": 1708300800000,
  "tokens": {
    "input": 100,
    "output": 0,
    "cache_write": 50,
    "cache_read": 0
  }
}
EOF

if ! "${CC_BIN}" validate --component agm --schema message --data "$TEST_DATA" --workspace oss 2>/dev/null | grep -q '"status": "valid"'; then
    echo "ERROR: Message data validation failed"
    rm -f "$TEST_DATA"
    exit 1
fi
rm -f "$TEST_DATA"
echo "✓ Message data validation passed"
echo

# Test 6: Schema compatibility check
echo "Test 6: Verify schema metadata"
SCHEMA_JSON=$("${CC_BIN}" schema --component agm --workspace oss 2>/dev/null)
VERSION=$(echo "$SCHEMA_JSON" | grep -o '"version": "[^"]*"' | head -1 | cut -d'"' -f4)
COMPATIBILITY=$(echo "$SCHEMA_JSON" | grep -o '"compatibility": "[^"]*"' | head -1 | cut -d'"' -f4)

if [ "$VERSION" != "1.0.0" ]; then
    echo "WARNING: Unexpected version: $VERSION (expected 1.0.0)"
fi

if [ "$COMPATIBILITY" != "backward" ]; then
    echo "WARNING: Unexpected compatibility mode: $COMPATIBILITY (expected backward)"
fi

echo "✓ Schema metadata verified"
echo "  - Version: $VERSION"
echo "  - Compatibility: $COMPATIBILITY"
echo

echo "========================================"
echo "✅ All Corpus Callosum integration tests passed!"
echo "========================================"
echo
echo "AGM is successfully registered with Corpus Callosum"
echo "You can now query AGM sessions via:"
echo "  cc discover --workspace oss"
echo "  cc schema --component agm --workspace oss"
echo "  cc validate --component agm --schema session --data <file>"
