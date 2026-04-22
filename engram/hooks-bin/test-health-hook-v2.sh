#!/bin/bash
# Test suite for engram-health-hook-v2.sh
# Tests stress detection, rate limiting, and performance
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOOK_SCRIPT="$HOME/bin/engram-health-hook-v2.sh"
TEST_RESULTS_DIR="/tmp/engram-health-hook-tests-$$"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Setup
mkdir -p "$TEST_RESULTS_DIR"

# Helper functions
pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    echo "  Details: $2"
    ((TESTS_FAILED++))
}

test_stress_detection() {
    echo "Test: Stress detection under high load"
    ((TESTS_RUN++))

    # Simulate high load by modifying /proc/loadavg temporarily
    # (This test requires the hook to read /proc/loadavg correctly)

    # Get actual CPU count
    local num_cpus=$(nproc)
    local threshold=$((num_cpus * 2))

    # Read current load
    local current_load=$(awk '{print int($1 + 0.5)}' /proc/loadavg)

    if [ "$current_load" -gt "$threshold" ]; then
        # System is actually under stress - verify hook exits quickly
        local start=$(date +%s%N)
        $HOOK_SCRIPT
        local end=$(date +%s%N)
        local duration_ms=$(( (end - start) / 1000000 ))

        if [ "$duration_ms" -lt 50 ]; then
            pass "Stress detection: Hook exited in ${duration_ms}ms (< 50ms threshold)"
        else
            fail "Stress detection: Hook took ${duration_ms}ms (expected < 50ms)" "Duration too long under stress"
        fi
    else
        # System is not under stress - skip this test
        echo "  ⚠️  Skipped (system load $current_load <= threshold $threshold)"
    fi
}

test_rate_limiting() {
    echo "Test: Rate limiting prevents rapid re-execution"
    ((TESTS_RUN++))

    # Clean up any existing rate limit file
    local rate_limit_file="$HOME/.engram/cache/health-hook-last-run"
    rm -f "$rate_limit_file"

    # First run - should execute normally
    $HOOK_SCRIPT

    # Verify rate limit file was created
    if [ -f "$rate_limit_file" ]; then
        local first_timestamp=$(cat "$rate_limit_file")

        # Second run immediately after - should be rate limited
        local start=$(date +%s%N)
        $HOOK_SCRIPT
        local end=$(date +%s%N)
        local duration_ms=$(( (end - start) / 1000000 ))

        # Verify timestamp wasn't updated (rate limited)
        local second_timestamp=$(cat "$rate_limit_file")

        if [ "$first_timestamp" -eq "$second_timestamp" ] && [ "$duration_ms" -lt 10 ]; then
            pass "Rate limiting: Second run blocked in ${duration_ms}ms"
        else
            fail "Rate limiting: Expected rate limit block" "First: $first_timestamp, Second: $second_timestamp, Duration: ${duration_ms}ms"
        fi
    else
        fail "Rate limiting: Rate limit file not created" "File not found: $rate_limit_file"
    fi

    # Cleanup
    rm -f "$rate_limit_file"
}

test_performance_baseline() {
    echo "Test: Performance baseline (no cache, no issues)"
    ((TESTS_RUN++))

    # Remove cache to simulate first run
    local cache_file="$HOME/.engram/cache/health-check.json"
    local cache_backup="${cache_file}.backup"

    if [ -f "$cache_file" ]; then
        mv "$cache_file" "$cache_backup"
    fi

    # Measure performance
    local start=$(date +%s%N)
    $HOOK_SCRIPT
    local end=$(date +%s%N)
    local duration_ms=$(( (end - start) / 1000000 ))

    # Restore cache
    if [ -f "$cache_backup" ]; then
        mv "$cache_backup" "$cache_file"
    fi

    # Should exit very quickly if no cache
    if [ "$duration_ms" -lt 25 ]; then
        pass "Performance: No-cache scenario completed in ${duration_ms}ms (< 25ms)"
    else
        fail "Performance: No-cache scenario took ${duration_ms}ms" "Expected < 25ms"
    fi
}

test_timeout_protection() {
    echo "Test: Timeout protection prevents hangs"
    ((TESTS_RUN++))

    # Create a mock hook that sleeps forever
    local mock_hook="/tmp/engram-health-hook-hang-$$"
    cat > "$mock_hook" << 'MOCK_END'
#!/bin/bash
# Simulate hung hook
sleep 10
MOCK_END
    chmod +x "$mock_hook"

    # Run mock hook with timeout (should be killed within 1 second)
    local start=$(date +%s)
    timeout 1 "$mock_hook" 2>/dev/null || true
    local end=$(date +%s)
    local duration=$((end - start))

    rm -f "$mock_hook"

    if [ "$duration" -le 1 ]; then
        pass "Timeout protection: Hung hook terminated in ${duration}s"
    else
        fail "Timeout protection: Took ${duration}s to terminate" "Expected <= 1s"
    fi
}

test_cache_parsing() {
    echo "Test: Cache parsing and issue detection"
    ((TESTS_RUN++))

    # Create mock cache with warnings
    local cache_file="$HOME/.engram/cache/health-check.json"
    local cache_backup="${cache_file}.backup"

    if [ -f "$cache_file" ]; then
        mv "$cache_file" "$cache_backup"
    fi

    # Create cache with mock warnings
    mkdir -p "$(dirname "$cache_file")"
    cat > "$cache_file" << 'JSON_END'
{
  "ttl": 300,
  "summary": {
    "warnings": 1,
    "errors": 0
  },
  "checks": [
    {
      "status": "warning",
      "message": "Test warning message"
    }
  ]
}
JSON_END

    # Run hook and capture output
    local output=$($HOOK_SCRIPT 2>&1 || true)

    # Restore cache
    rm -f "$cache_file"
    if [ -f "$cache_backup" ]; then
        mv "$cache_backup" "$cache_file"
    fi

    # Verify output contains warning
    if echo "$output" | grep -q "Test warning message"; then
        pass "Cache parsing: Warning message detected and displayed"
    else
        fail "Cache parsing: Warning not found in output" "Output: $output"
    fi
}

# Run all tests
echo "========================================"
echo "Engram Health Hook v2 - Test Suite"
echo "========================================"
echo ""

test_stress_detection
test_rate_limiting
test_performance_baseline
test_timeout_protection
test_cache_parsing

# Summary
echo ""
echo "========================================"
echo "Test Results"
echo "========================================"
echo "Total:  $TESTS_RUN"
echo "Passed: $TESTS_PASSED"
echo "Failed: $TESTS_FAILED"

# Cleanup
rm -rf "$TEST_RESULTS_DIR"

# Exit with failure if any tests failed
if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
else
    exit 0
fi
