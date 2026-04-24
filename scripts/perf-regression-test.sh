#!/bin/bash
#
# Performance Regression Testing for Pre-Commit Hook
#
# Hybrid approach:
#   - Local quick check: 1 run with >2x threshold
#   - CI comprehensive: 10 runs with median, statistical analysis
#
# Usage:
#   ./scripts/perf-regression-test.sh [--mode=local|ci]
#

set -e

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
BASELINE_FILE="$REPO_ROOT/.perf-baselines/pre-commit-hook.json"
HOOK_PATH="$REPO_ROOT/../.bare/hooks/pre-commit"

# Default mode
MODE="${1:-local}"
if [[ "$MODE" == --mode=* ]]; then
    MODE="${MODE#--mode=}"
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Ensure baseline directory exists
mkdir -p "$(dirname "$BASELINE_FILE")"

# Check if baseline exists
if [ ! -f "$BASELINE_FILE" ]; then
    echo "${YELLOW}No baseline found. Creating initial baseline...${NC}"
    echo "Run this script again after baseline is created."

    # Create baseline (will be implemented)
    echo '{"scenarios":[],"created_at":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","git_commit":"'$(git rev-parse HEAD)'"}' > "$BASELINE_FILE"
    exit 0
fi

# Load baseline
BASELINE_MEDIAN=$(jq -r '.scenarios[] | select(.name=="current") | .median_ms' "$BASELINE_FILE" 2>/dev/null || echo "0")

if [ "$BASELINE_MEDIAN" == "0" ] || [ "$BASELINE_MEDIAN" == "null" ]; then
    echo "${RED}Error: Baseline file exists but has no valid data${NC}"
    echo "Delete $BASELINE_FILE and re-run to create new baseline"
    exit 1
fi

# Stage some test files to trigger hook
# (In real usage, hook runs on actual staged files)
echo "# Benchmark test file" > "$REPO_ROOT/bench-test-temp.sh"
git -C "$REPO_ROOT" add bench-test-temp.sh 2>/dev/null || true

case "$MODE" in
    local)
        echo "=== Local Quick Check (1 run, >2x threshold) ==="
        echo "Baseline: ${BASELINE_MEDIAN}ms"
        echo ""

        # Single run with timing
        START=$(date +%s.%N)
        timeout 10 "$HOOK_PATH" >/dev/null 2>&1 || EXIT_CODE=$?
        END=$(date +%s.%N)

        DURATION_MS=$(echo "($END - $START) * 1000" | bc | cut -d. -f1)
        THRESHOLD_MS=$(echo "$BASELINE_MEDIAN * 2" | bc | cut -d. -f1)

        echo "Current: ${DURATION_MS}ms"
        echo "Threshold (2x): ${THRESHOLD_MS}ms"
        echo ""

        if [ "$DURATION_MS" -gt "$THRESHOLD_MS" ]; then
            echo "${RED}❌ FAIL: Hook is >2x slower than baseline${NC}"
            echo "This is a significant regression. Run comprehensive CI test for details."
            exit 1
        else
            PERCENT=$(echo "scale=1; ($DURATION_MS - $BASELINE_MEDIAN) / $BASELINE_MEDIAN * 100" | bc)
            if (( $(echo "$PERCENT > 0" | bc -l) )); then
                echo "${GREEN}✅ PASS: Hook within acceptable range (+${PERCENT}%)${NC}"
            else
                echo "${GREEN}✅ PASS: Hook within acceptable range (${PERCENT}%)${NC}"
            fi
        fi
        ;;

    ci)
        echo "=== CI Comprehensive Test (10 runs, median comparison) ==="
        echo "Baseline: ${BASELINE_MEDIAN}ms"
        echo ""

        # Run hyperfine with 10 iterations
        hyperfine --warmup 1 --runs 10 \
                  --export-json /tmp/perf-current.json \
                  "$HOOK_PATH" >/dev/null 2>&1 || true

        # Extract median from hyperfine output
        CURRENT_MEDIAN=$(jq -r '.results[0].median * 1000' /tmp/perf-current.json | cut -d. -f1)
        CURRENT_MEAN=$(jq -r '.results[0].mean * 1000' /tmp/perf-current.json | cut -d. -f1)
        CURRENT_STDDEV=$(jq -r '.results[0].stddev * 1000' /tmp/perf-current.json | cut -d. -f1)

        # Calculate coefficient of variation (CV%)
        CV=$(echo "scale=1; ($CURRENT_STDDEV / $CURRENT_MEAN) * 100" | bc)

        echo "Current median: ${CURRENT_MEDIAN}ms"
        echo "Current mean: ${CURRENT_MEAN}ms ± ${CURRENT_STDDEV}ms"
        echo "Coefficient of variation: ${CV}%"
        echo ""

        # Check variance
        if (( $(echo "$CV > 20" | bc -l) )); then
            echo "${YELLOW}⚠️  WARNING: High variance (CV=${CV}%)${NC}"
            echo "Results may be unreliable. Consider:"
            echo "  - Running on dedicated CI runner"
            echo "  - Increasing run count"
            echo "  - Checking for system interference"
            echo ""
        fi

        # Calculate regression percentage
        DELTA_MS=$(echo "$CURRENT_MEDIAN - $BASELINE_MEDIAN" | bc)
        PERCENT=$(echo "scale=1; ($DELTA_MS / $BASELINE_MEDIAN) * 100" | bc)

        # Threshold: 15% for cloud CI (per D1 research)
        THRESHOLD_PERCENT=15

        echo "Change: ${DELTA_MS}ms (${PERCENT}%)"
        echo "Threshold: ${THRESHOLD_PERCENT}%"
        echo ""

        if (( $(echo "$PERCENT > $THRESHOLD_PERCENT" | bc -l) )); then
            echo "${RED}❌ FAIL: Regression exceeds ${THRESHOLD_PERCENT}% threshold${NC}"
            echo ""
            echo "Performance has regressed by ${PERCENT}%."
            echo "Please either:"
            echo "  1. Optimize the hook to restore performance, OR"
            echo "  2. Justify the regression in PR description and update baseline"
            echo ""
            exit 1
        elif (( $(echo "$PERCENT > 5" | bc -l) )); then
            echo "${YELLOW}⚠️  WARNING: Minor regression detected (${PERCENT}%)${NC}"
            echo "Still within threshold, but consider optimization."
        elif (( $(echo "$PERCENT < -10" | bc -l) )); then
            echo "${GREEN}✅ PASS: Performance improved by ${PERCENT#-}%!${NC}"
            echo "Consider updating baseline to capture improvement."
        else
            echo "${GREEN}✅ PASS: Performance stable (${PERCENT}%)${NC}"
        fi
        ;;

    *)
        echo "${RED}Error: Unknown mode '$MODE'${NC}"
        echo "Usage: $0 [--mode=local|ci]"
        exit 1
        ;;
esac

# Cleanup
git -C "$REPO_ROOT" reset HEAD bench-test-temp.sh >/dev/null 2>&1 || true
rm -f "$REPO_ROOT/bench-test-temp.sh"
