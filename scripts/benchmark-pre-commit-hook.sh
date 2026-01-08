#!/bin/bash
#
# Pre-Commit Hook Performance Benchmark
#
# Measures hook execution time with different file counts to ensure
# the hook doesn't slow down developer workflow.
#
# Usage:
#   ./scripts/benchmark-pre-commit-hook.sh
#
# Performance Thresholds:
#   ✅ <100ms: Excellent (typical 1-10 file commits)
#   ⚠️  100-500ms: Acceptable (large commits)
#   ❌ >500ms: Needs optimization
#

set -e

# Determine script location and repository root
SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

GIT_COMMON_DIR=$(git -C "$REPO_ROOT" rev-parse --git-common-dir 2>/dev/null || git -C "$REPO_ROOT" rev-parse --git-dir)
HOOK_PATH="$GIT_COMMON_DIR/hooks/pre-commit"

if [ ! -f "$HOOK_PATH" ]; then
    echo "${RED}Error: Hook not found at $HOOK_PATH${NC}"
    exit 1
fi

echo "=== Pre-Commit Hook Performance Benchmark ==="
echo "Repository: $REPO_ROOT"
echo "Hook: $HOOK_PATH"
echo ""

# Ensure clean state
git -C "$REPO_ROOT" reset HEAD . 2>/dev/null || true

# Helper function to measure hook execution
measure_hook() {
    local description="$1"
    local file_count="$2"

    START=$(date +%s.%N)
    timeout 5 "$HOOK_PATH" >/dev/null 2>&1 || true
    END=$(date +%s.%N)

    # Calculate duration in milliseconds
    DURATION_MS=$(echo "($END - $START) * 1000" | bc | cut -d. -f1)

    # Color code based on threshold
    if [ "$DURATION_MS" -lt 100 ]; then
        COLOR=$GREEN
        STATUS="✅"
    elif [ "$DURATION_MS" -lt 500 ]; then
        COLOR=$YELLOW
        STATUS="⚠️ "
    else
        COLOR=$RED
        STATUS="❌"
    fi

    printf "%s %-30s %3d files: %s%4dms%s\n" \
        "$STATUS" "$description" "$file_count" "$COLOR" "$DURATION_MS" "$NC"
}

# Test 1: No staged files
echo "Test 1: Empty staging area"
measure_hook "No files" 0

# Test 2: Single shell file
echo ""
echo "Test 2: Shell scripts with validation"
cat > "$REPO_ROOT/bench-test-1.sh" << 'EOF'
#!/bin/bash
echo "test"
EOF
git -C "$REPO_ROOT" add bench-test-1.sh
measure_hook "1 shell file" 1
git -C "$REPO_ROOT" reset HEAD bench-test-1.sh >/dev/null
rm -f bench-test-1.sh

# Test 3: Multiple shell files
echo ""
echo "Test 3: Scaling with file count"
for count in 5 10 20 50; do
    # Create test files
    for i in $(seq 1 $count); do
        echo "# Test file $i" > "$REPO_ROOT/bench-test-$i.sh"
    done

    git -C "$REPO_ROOT" add bench-test-*.sh
    measure_hook "Shell files" $count
    git -C "$REPO_ROOT" reset HEAD bench-test-*.sh >/dev/null
    rm -f "$REPO_ROOT"/bench-test-*.sh
done

# Test 4: Mixed file types
echo ""
echo "Test 4: Mixed file types (shell + Python + Markdown)"
cat > "$REPO_ROOT/bench-mixed-1.sh" << 'EOF'
#!/bin/bash
echo "test"
EOF

cat > "$REPO_ROOT/bench-mixed-2.py" << 'EOF'
import os
print("test")
EOF

cat > "$REPO_ROOT/bench-mixed-3.md" << 'EOF'
# Test file
EOF

git -C "$REPO_ROOT" add bench-mixed-*.sh bench-mixed-*.py bench-mixed-*.md
measure_hook "Mixed types" 3
git -C "$REPO_ROOT" reset HEAD bench-mixed-* >/dev/null
rm -f "$REPO_ROOT"/bench-mixed-*

# Test 5: Command file validation
echo ""
echo "Test 5: Slash command documentation validation"
mkdir -p "$REPO_ROOT/bench-commands"
cat > "$REPO_ROOT/bench-commands/test-cmd.md" << 'EOF'
---
description: Test command for benchmarking
allowed-tools: Bash
---
# Test Command
EOF

git -C "$REPO_ROOT" add bench-commands/test-cmd.md
measure_hook "Command file" 1
git -C "$REPO_ROOT" reset HEAD bench-commands/test-cmd.md >/dev/null
rm -rf "$REPO_ROOT"/bench-commands

echo ""
echo "=== Performance Summary ==="
echo "  ✅ <100ms: Excellent (typical commits)"
echo "  ⚠️  100-500ms: Acceptable (large commits)"
echo "  ❌ >500ms: Needs optimization"
echo ""
echo "Note: Measurements include hook startup overhead (~10-15ms)"
