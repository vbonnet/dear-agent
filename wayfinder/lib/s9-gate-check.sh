#!/bin/bash
set -euo pipefail

# s9-gate-check.sh: S9 test failure threshold detection
# Escalates based on test failure count: >3 → M, >10 → L

PROJECT_DIR="${1:-.}"
S9_DELIVERABLE="$PROJECT_DIR/S9-validation.md"

# Exit silently if S9 deliverable doesn't exist yet
if [[ ! -f "$S9_DELIVERABLE" ]]; then
    exit 0
fi

# Auto-detect test framework
TEST_FRAMEWORK=""
TEST_COMMAND=""

if [[ -f "$PROJECT_DIR/go.mod" ]]; then
    TEST_FRAMEWORK="go"
    TEST_COMMAND="go test ./..."
elif [[ -f "$PROJECT_DIR/package.json" ]]; then
    TEST_FRAMEWORK="npm"
    TEST_COMMAND="npm test"
elif [[ -f "$PROJECT_DIR/pytest.ini" ]] || [[ -f "$PROJECT_DIR/setup.py" ]]; then
    TEST_FRAMEWORK="pytest"
    TEST_COMMAND="pytest"
elif [[ -f "$PROJECT_DIR/Cargo.toml" ]]; then
    TEST_FRAMEWORK="cargo"
    TEST_COMMAND="cargo test"
else
    echo "ℹ️  S9 Gate Check: No test framework detected (skipping)"
    exit 0
fi

echo "🧪 S9 Gate Check: Running tests ($TEST_FRAMEWORK)..."

# Run tests and capture output
TEST_OUTPUT=""
TEST_EXIT_CODE=0

if command -v $(echo "$TEST_COMMAND" | awk '{print $1}') &> /dev/null; then
    TEST_OUTPUT=$(eval "$TEST_COMMAND" 2>&1 || true)
    TEST_EXIT_CODE=$?
else
    echo "⚠️  Test command not available: $TEST_COMMAND (skipping)"
    exit 0
fi

# Parse test failures
FAILURE_COUNT=0

case "$TEST_FRAMEWORK" in
    go)
        FAILURE_COUNT=$(echo "$TEST_OUTPUT" | grep -c "FAIL" || echo "0")
        ;;
    npm)
        FAILURE_COUNT=$(echo "$TEST_OUTPUT" | grep -c "failing" || echo "0")
        ;;
    pytest)
        FAILURE_COUNT=$(echo "$TEST_OUTPUT" | grep -oP '\d+ failed' | awk '{print $1}' || echo "0")
        ;;
    cargo)
        FAILURE_COUNT=$(echo "$TEST_OUTPUT" | grep -oP '\d+ failed' | awk '{print $1}' || echo "0")
        ;;
esac

# Escalation logic based on failure count
if [[ "$FAILURE_COUNT" -gt 10 ]]; then
    echo "🚨 S9 Gate Check: $FAILURE_COUNT test failures detected (critical)"
    echo ""
    echo "Test output excerpt (last 30 lines):"
    echo "$TEST_OUTPUT" | tail -30
    echo ""

    # Get current depth
    CURRENT_DEPTH=$(wayfinder -C "$PROJECT_DIR" session status --field depth 2>/dev/null || echo "S")

    # Escalate to L (52m)
    TARGET_DEPTH="L"

    if [[ "$CURRENT_DEPTH" == "XL" ]] || [[ "$CURRENT_DEPTH" == "L" ]]; then
        echo "ℹ️  Depth already ${CURRENT_DEPTH} (no escalation needed)"
        exit 0
    fi

    echo "📈 Escalating ${CURRENT_DEPTH} → ${TARGET_DEPTH} (+extensive debugging time)"

    # Attempt escalation
    if wayfinder -C "$PROJECT_DIR" session escalate \
        --to "$TARGET_DEPTH" \
        --trigger "S9: $FAILURE_COUNT test failures" \
        --reason "Extensive test failures require deeper investigation and fixes" 2>/dev/null; then
        echo "✅ Escalation successful: ${CURRENT_DEPTH} → ${TARGET_DEPTH}"
    else
        echo "⚠️  Escalation command failed (wayfinder session escalate not available)"
        echo "ℹ️  Recommendation: Manually set depth to $TARGET_DEPTH"
    fi

elif [[ "$FAILURE_COUNT" -gt 3 ]]; then
    echo "⚠️  S9 Gate Check: $FAILURE_COUNT test failures detected (moderate)"
    echo ""
    echo "Test output excerpt (last 15 lines):"
    echo "$TEST_OUTPUT" | tail -15
    echo ""

    # Get current depth
    CURRENT_DEPTH=$(wayfinder -C "$PROJECT_DIR" session status --field depth 2>/dev/null || echo "S")

    # Escalate to M (31m)
    TARGET_DEPTH="M"

    if [[ "$CURRENT_DEPTH" == "XL" ]] || [[ "$CURRENT_DEPTH" == "L" ]] || [[ "$CURRENT_DEPTH" == "M" ]]; then
        echo "ℹ️  Depth already ${CURRENT_DEPTH} (no escalation needed)"
        exit 0
    fi

    echo "📈 Escalating ${CURRENT_DEPTH} → ${TARGET_DEPTH} (+debugging time)"

    # Attempt escalation
    if wayfinder -C "$PROJECT_DIR" session escalate \
        --to "$TARGET_DEPTH" \
        --trigger "S9: $FAILURE_COUNT test failures" \
        --reason "Multiple test failures require additional debugging effort" 2>/dev/null; then
        echo "✅ Escalation successful: ${CURRENT_DEPTH} → ${TARGET_DEPTH}"
    else
        echo "⚠️  Escalation command failed (wayfinder session escalate not available)"
        echo "ℹ️  Recommendation: Manually set depth to $TARGET_DEPTH"
    fi

elif [[ "$FAILURE_COUNT" -gt 0 ]]; then
    echo "ℹ️  S9 Gate Check: $FAILURE_COUNT test failure(s) detected (minor)"
    echo "No escalation (threshold is >3 failures)"

else
    echo "✅ S9 Gate Check: All tests passed"
fi
