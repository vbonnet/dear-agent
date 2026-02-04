#!/bin/bash
# Test: Permission prompt rejection
# Verifies astrocyte detects permission prompts with violations and rejects them

set -euo pipefail

TEST_NAME="permission_rejection"
SESSION_NAME="test-permission"
FIXTURE="stuck-permission.txt"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[TEST]${NC} $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
}

# Header
echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║  Test: Permission Prompt Rejection                        ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Setup
log_info "Setting up test environment..."
/tests/scripts/setup-test-env.sh

# Create violation prompt file for csm reject
VIOLATION_PROMPTS="/home/testuser/src/ws/oss/tool-usage-analysis/prompts/VIOLATION-PROMPTS.md"
mkdir -p "$(dirname "$VIOLATION_PROMPTS")"
cat > "$VIOLATION_PROMPTS" <<'EOF'
# Tool Usage Violation Prompts

You violated tool usage guidelines by requesting a bash command with prohibited patterns.

Please revise your approach without using:
- Command chaining (&&, ||, ;)
- Pipes (|)
- Text processing commands (grep, cat, find)
- Directory changes (cd)

Use dedicated tools instead.
EOF

# Create stuck session
log_info "Creating session with permission prompt..."
/tests/scripts/create-stuck-session.sh "$SESSION_NAME" "$FIXTURE"

# Run astrocyte for 2 check cycles
# Permission prompts should be detected immediately (fresh start detection)
log_info "Running astrocyte daemon (10 seconds)..."
/tests/scripts/run-astrocyte-test.sh 10

# Verify recovery
log_info "Verifying recovery..."
if /tests/scripts/verify-recovery.sh "$SESSION_NAME" "permission_prompt"; then
    log_pass "Permission prompt detection test PASSED"

    # Additional check: Verify csm reject was called
    CSM_LOG="/home/testuser/.csm/astrocyte/logs/csm-mock.log"
    if [ -f "$CSM_LOG" ] && grep -q "csm reject.*$SESSION_NAME" "$CSM_LOG"; then
        log_pass "Permission prompt was rejected via csm reject"
    else
        log_fail "csm reject was NOT called"
        exit 1
    fi

    exit 0
else
    log_fail "Permission prompt detection test FAILED"
    exit 1
fi
