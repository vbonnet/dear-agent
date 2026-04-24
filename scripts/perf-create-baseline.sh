#!/bin/bash
#
# Create Performance Baseline for Pre-Commit Hook
#
# Runs comprehensive benchmark and saves results as baseline.
# Should be run when:
#   - First setting up performance testing
#   - After justified performance regression
#   - After major performance improvements
#
# Usage:
#   ./scripts/perf-create-baseline.sh [--force]
#

set -e

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
BASELINE_FILE="$REPO_ROOT/.perf-baselines/pre-commit-hook.json"
HOOK_PATH="$REPO_ROOT/../.bare/hooks/pre-commit"

FORCE=false
if [ "$1" == "--force" ]; then
    FORCE=true
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Check if baseline already exists
if [ -f "$BASELINE_FILE" ] && [ "$FORCE" != "true" ]; then
    echo "${YELLOW}Baseline already exists at:${NC}"
    echo "  $BASELINE_FILE"
    echo ""
    echo "Current baseline:"
    jq '.' "$BASELINE_FILE"
    echo ""
    echo "${RED}Use --force to overwrite existing baseline${NC}"
    exit 1
fi

mkdir -p "$(dirname "$BASELINE_FILE")"

echo "=== Creating Performance Baseline ==="
echo "Hook: $HOOK_PATH"
echo "Baseline: $BASELINE_FILE"
echo ""
echo "Running 20 iterations (1 warmup + 19 measured)..."
echo "This will take approximately 1 minute..."
echo ""

# Stage a test file to trigger hook validation
echo "# Baseline test file" > "$REPO_ROOT/baseline-test.sh"
git -C "$REPO_ROOT" add baseline-test.sh 2>/dev/null || true

# Run hyperfine with 20 runs
hyperfine --warmup 1 --runs 20 \
          --export-json /tmp/baseline-raw.json \
          "$HOOK_PATH"

# Extract statistics
MEDIAN=$(jq -r '.results[0].median * 1000' /tmp/baseline-raw.json)
MEAN=$(jq -r '.results[0].mean * 1000' /tmp/baseline-raw.json)
STDDEV=$(jq -r '.results[0].stddev * 1000' /tmp/baseline-raw.json)
MIN=$(jq -r '.results[0].min * 1000' /tmp/baseline-raw.json)
MAX=$(jq -r '.results[0].max * 1000' /tmp/baseline-raw.json)

# Calculate CV%
CV=$(echo "scale=2; ($STDDEV / $MEAN) * 100" | bc)

echo ""
echo "Baseline Statistics:"
echo "  Median: ${MEDIAN}ms"
echo "  Mean: ${MEAN}ms"
echo "  StdDev: ${STDDEV}ms"
echo "  Min: ${MIN}ms"
echo "  Max: ${MAX}ms"
echo "  CV%: ${CV}%"
echo ""

# Check if variance is acceptable
if (( $(echo "$CV > 20" | bc -l) )); then
    echo "${YELLOW}⚠️  WARNING: High variance (CV=${CV}%)${NC}"
    echo "This baseline may be unreliable. Consider:"
    echo "  - Running on dedicated hardware"
    echo "  - Closing background applications"
    echo "  - Re-running baseline creation"
    echo ""
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Baseline creation cancelled."
        exit 1
    fi
fi

# Get git context
GIT_COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD)
GIT_BRANCH=$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)

# Create baseline JSON
cat > "$BASELINE_FILE" <<EOF
{
  "schema_version": "1.0",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "git_commit": "$GIT_COMMIT",
  "git_branch": "$GIT_BRANCH",
  "hyperfine_version": "$(hyperfine --version | head -1)",
  "scenarios": [
    {
      "name": "current",
      "description": "Pre-commit hook execution time",
      "median_ms": ${MEDIAN},
      "mean_ms": ${MEAN},
      "stddev_ms": ${STDDEV},
      "min_ms": ${MIN},
      "max_ms": ${MAX},
      "cv_percent": ${CV},
      "runs": 20,
      "warmup_runs": 1
    }
  ]
}
EOF

echo "${GREEN}✅ Baseline created successfully${NC}"
echo ""
echo "Baseline saved to:"
echo "  $BASELINE_FILE"
echo ""
echo "Commit this file to version control:"
echo "  git add .perf-baselines/pre-commit-hook.json"
echo "  git commit -m 'perf: establish performance baseline (median: ${MEDIAN}ms)'"
echo ""

# Cleanup
git -C "$REPO_ROOT" reset HEAD baseline-test.sh >/dev/null 2>&1 || true
rm -f "$REPO_ROOT/baseline-test.sh"
rm -f /tmp/baseline-raw.json
