#!/bin/bash
#
# Test suite for PreTool validators (WS5)
#
# Tests:
# 1. pretool-bash-validator - pattern database integration
# 2. pretool-beads-validator - beads protection
# 3. pretool-git-validator - git worktree enforcement
#

# Note: Do NOT use 'set -e' here - we intentionally test non-zero exit codes

HOOKS_DIR="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================="
echo "PreTool Validators Test Suite (WS5)"
echo "========================================="
echo ""

# Helper function to run test
run_test() {
    local test_name="$1"
    local validator="$2"
    local input_json="$3"
    local expected_exit="$4"

    echo -n "Testing: $test_name ... "

    # Run validator (capture exit code explicitly)
    set +e  # Temporarily allow non-zero exits
    output=$(echo "$input_json" | python3 "$HOOKS_DIR/$validator" 2>&1)
    exit_code=$?
    set -e  # Re-enable exit on error

    if [ "$exit_code" -eq "$expected_exit" ]; then
        echo -e "${GREEN}PASS${NC}"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}FAIL${NC} (expected exit $expected_exit, got $exit_code)"
        echo "  Output: $output"
        FAIL=$((FAIL + 1))
    fi
}

echo "========================================="
echo "1. BASH VALIDATOR TESTS"
echo "========================================="
echo ""

# Test 1.1: Allow safe command
run_test "1.1 Allow safe git command" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git status"}}' \
    0

# Test 1.2: Block cd chaining (tier2_validation: true)
run_test "1.2 Block cd chaining" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"cd /repo && git push"}}' \
    2

# Test 1.3: Block cat file read (tier2_validation: true)
run_test "1.3 Block cat file read" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"cat file.txt"}}' \
    2

# Test 1.4: Block grep usage (tier2_validation: true)
run_test "1.4 Block grep usage" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"grep TODO file.txt"}}' \
    2

# Test 1.5: Block find usage (tier2_validation: true)
run_test "1.5 Block find usage" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"find . -name *.py"}}' \
    2

# Test 1.6: Block for loop (tier2_validation: true)
run_test "1.6 Block for loop" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"for file in *.txt; do cat $file; done"}}' \
    2

# Test 1.7: Block command chaining with && (tier2_validation: true)
run_test "1.7 Block && chaining" \
    "pretool-bash-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git add . && git commit -m msg"}}' \
    2

# Test 1.8: Allow non-bash tool
run_test "1.8 Ignore non-Bash tool" \
    "pretool-bash-validator" \
    '{"tool_name":"Read","tool_input":{"file_path":"test.txt"}}' \
    0

echo ""
echo "========================================="
echo "2. BEADS VALIDATOR TESTS"
echo "========================================="
echo ""

# Test 2.1: Block Read access to .beads/
run_test "2.1 Block Read .beads/" \
    "pretool-beads-validator" \
    '{"tool_name":"Read","tool_input":{"file_path":"/path/to/.beads/db.sqlite3"}}' \
    1

# Test 2.2: Block Write access to .beads/
run_test "2.2 Block Write .beads/" \
    "pretool-beads-validator" \
    '{"tool_name":"Write","tool_input":{"file_path":"/path/to/.beads/db.sqlite3","content":"test"}}' \
    1

# Test 2.3: Block Edit access to .beads/
run_test "2.3 Block Edit .beads/" \
    "pretool-beads-validator" \
    '{"tool_name":"Edit","tool_input":{"file_path":"/path/to/.beads/db.sqlite3","old_string":"foo","new_string":"bar"}}' \
    1

# Test 2.4: Block sqlite3 direct access
run_test "2.4 Block sqlite3 .beads/" \
    "pretool-beads-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"sqlite3 .beads/db.sqlite3 SELECT * FROM beads"}}' \
    1

# Test 2.5: Block rm .beads/
run_test "2.5 Block rm .beads/" \
    "pretool-beads-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"rm -rf .beads/"}}' \
    1

# Test 2.6: Allow normal file operations
run_test "2.6 Allow normal Read" \
    "pretool-beads-validator" \
    '{"tool_name":"Read","tool_input":{"file_path":"/path/to/file.txt"}}' \
    0

# Test 2.7: Allow non-beads bash commands
run_test "2.7 Allow normal Bash" \
    "pretool-beads-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}' \
    0

echo ""
echo "========================================="
echo "3. GIT VALIDATOR TESTS"
echo "========================================="
echo ""

# Test 3.1: Block git branch creation
run_test "3.1 Block git branch creation" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch feature-xyz"}}' \
    1

# Test 3.2: Block git checkout main
run_test "3.2 Block git checkout main" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git checkout main"}}' \
    1

# Test 3.3: Block git push --force main
run_test "3.3 Block git push --force main" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git push --force origin main"}}' \
    1

# Test 3.4: Allow git worktree commands
run_test "3.4 Allow git worktree add" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git worktree add ../feature-xyz feature-branch"}}' \
    0

# Test 3.5: Allow git worktree list
run_test "3.5 Allow git worktree list" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git worktree list"}}' \
    0

# Test 3.6: Allow git status (read-only)
run_test "3.6 Allow git status" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git status"}}' \
    0

# Test 3.7: Allow non-git commands
run_test "3.7 Ignore non-git command" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"npm install"}}' \
    0

# Test 3.8: Ignore non-Bash tools
run_test "3.8 Ignore non-Bash tool" \
    "pretool-git-validator" \
    '{"tool_name":"Read","tool_input":{"file_path":"test.txt"}}' \
    0

# --- Read-only git operation tests ---

# Test 3.9: Allow git log (read-only)
run_test "3.9 Allow git log" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git log --oneline -10"}}' \
    0

# Test 3.10: Allow git diff (read-only)
run_test "3.10 Allow git diff" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git diff HEAD~1"}}' \
    0

# Test 3.11: Allow git show (read-only)
run_test "3.11 Allow git show" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git show HEAD"}}' \
    0

# Test 3.12: Allow git branch listing
run_test "3.12 Allow git branch (listing)" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch"}}' \
    0

# Test 3.13: Allow git branch -l
run_test "3.13 Allow git branch -l" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -l"}}' \
    0

# Test 3.14: Allow git branch --list
run_test "3.14 Allow git branch --list" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch --list"}}' \
    0

# Test 3.15: Allow git branch -a (list all)
run_test "3.15 Allow git branch -a" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -a"}}' \
    0

# Test 3.16: Allow git branch -r (list remote)
run_test "3.16 Allow git branch -r" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -r"}}' \
    0

# Test 3.17: Allow git remote -v (read-only)
run_test "3.17 Allow git remote -v" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git remote -v"}}' \
    0

# Test 3.18: Allow git rev-parse (read-only)
run_test "3.18 Allow git rev-parse" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git rev-parse HEAD"}}' \
    0

# Test 3.19: Allow git ls-files (read-only)
run_test "3.19 Allow git ls-files" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git ls-files"}}' \
    0

# Test 3.20: Allow git ls-tree (read-only)
run_test "3.20 Allow git ls-tree" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git ls-tree HEAD"}}' \
    0

# Test 3.21: Allow git describe (read-only)
run_test "3.21 Allow git describe" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git describe --tags"}}' \
    0

# Test 3.22: Allow git -C <path> status (read-only with -C)
run_test "3.22 Allow git -C status" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git -C ./engram status"}}' \
    0

# Test 3.23: Allow git -C <path> log (read-only with -C)
run_test "3.23 Allow git -C log" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git -C ./engram log --oneline -5"}}' \
    0

# Test 3.24: Allow git -C <path> diff (read-only with -C)
run_test "3.24 Allow git -C diff" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git -C ./engram diff"}}' \
    0

# Test 3.25: Block git branch -d (deletion)
run_test "3.25 Block git branch -d" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -d old-branch"}}' \
    1

# Test 3.26: Block git branch -D (force delete)
run_test "3.26 Block git branch -D" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -D old-branch"}}' \
    1

# Test 3.27: Block git branch -m (rename)
run_test "3.27 Block git branch -m" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -m old-name new-name"}}' \
    1

# Test 3.28: Allow git branch -v (verbose listing)
run_test "3.28 Allow git branch -v" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch -v"}}' \
    0

# Test 3.29: Allow git branch --merged
run_test "3.29 Allow git branch --merged" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git branch --merged"}}' \
    0

# Test 3.30: Block git switch main
run_test "3.30 Block git switch main" \
    "pretool-git-validator" \
    '{"tool_name":"Bash","tool_input":{"command":"git switch main"}}' \
    1

echo ""
echo "========================================="
echo "TEST RESULTS"
echo "========================================="
echo ""
echo -e "${GREEN}PASSED: $PASS${NC}"
echo -e "${RED}FAILED: $FAIL${NC}"
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
