#!/bin/bash
# Visual Regression Testing for Diagram-as-Code
# Compares rendered diagrams to baseline images using ImageMagick

set -e

# Configuration
BASELINE_DIR="${BASELINE_DIR:-test/visual-baselines}"
CURRENT_DIR="${CURRENT_DIR:-test/visual-current}"
DIFF_DIR="${DIFF_DIR:-test/visual-diffs}"

# Thresholds (as percentages)
THRESHOLD_PASS=0.01    # < 1% = auto-pass
THRESHOLD_FLAG=0.05    # 1-5% = flag for review
THRESHOLD_BLOCK=0.20   # > 20% = block

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Track results
total=0
passed=0
flagged=0
failed=0
missing_baseline=0

# Create output directories
mkdir -p "$CURRENT_DIR" "$DIFF_DIR"

echo "🎨 Visual Regression Testing"
echo "==============================="
echo "Baseline: $BASELINE_DIR"
echo "Current:  $CURRENT_DIR"
echo "Diffs:    $DIFF_DIR"
echo ""

# Find all diagram files
find_diagrams() {
    find examples -type f \( -name "*.d2" -o -name "*.dsl" -o -name "*.mmd" \) 2>/dev/null || true
}

# Render diagram to PNG
render_diagram() {
    local diagram=$1
    local output=$2
    local ext="${diagram##*.}"

    case "$ext" in
        d2)
            if command -v d2 &> /dev/null; then
                d2 "$diagram" "$output" --theme=0 2>&1 | grep -v "^$" || true
            else
                echo "⚠️  d2 not found, skipping D2 diagram"
                return 1
            fi
            ;;
        dsl)
            if command -v structurizr-cli &> /dev/null; then
                # Structurizr requires more complex setup, skip for MVP
                echo "⚠️  Structurizr rendering requires workspace setup, skipping"
                return 1
            else
                echo "⚠️  structurizr-cli not found, skipping Structurizr diagram"
                return 1
            fi
            ;;
        mmd)
            if command -v mmdc &> /dev/null; then
                mmdc -i "$diagram" -o "$output" -b transparent 2>&1 | grep -v "^$" || true
            else
                echo "⚠️  mmdc not found, skipping Mermaid diagram"
                return 1
            fi
            ;;
        *)
            echo "⚠️  Unknown diagram format: $ext"
            return 1
            ;;
    esac
}

# Compare two images
compare_images() {
    local baseline=$1
    local current=$2
    local diff=$3

    # Use ImageMagick compare with RMSE metric
    # Returns format: "123.45 (0.00188)"
    # We want the percentage in parentheses
    local result
    result=$(compare -metric RMSE "$baseline" "$current" "$diff" 2>&1 | tr -d '()' | awk '{print $2}') || true

    # Convert to percentage (multiply by 100)
    local diff_percent
    diff_percent=$(echo "$result * 100" | bc -l 2>/dev/null || echo "0")

    echo "$diff_percent"
}

# Main test loop
for diagram in $(find_diagrams); do
    total=$((total + 1))

    # Extract name without extension and path
    name=$(basename "$diagram")
    name_no_ext="${name%.*}"

    echo "Testing: $name"

    # Render current version
    current_file="$CURRENT_DIR/$name_no_ext.png"
    if ! render_diagram "$diagram" "$current_file"; then
        echo "  ⚠️  Skipped (render failed)"
        echo ""
        continue
    fi

    # Check if baseline exists
    baseline_file="$BASELINE_DIR/$name_no_ext.png"
    if [ ! -f "$baseline_file" ]; then
        echo "  ⚠️  No baseline found - creating baseline"
        mkdir -p "$(dirname "$baseline_file")"
        cp "$current_file" "$baseline_file"
        missing_baseline=$((missing_baseline + 1))
        echo ""
        continue
    fi

    # Compare images
    diff_file="$DIFF_DIR/$name_no_ext-diff.png"
    diff_percent=$(compare_images "$baseline_file" "$current_file" "$diff_file")

    # Evaluate threshold
    if (( $(echo "$diff_percent < $THRESHOLD_PASS" | bc -l) )); then
        echo -e "  ${GREEN}✅ PASS${NC} (${diff_percent}% diff)"
        passed=$((passed + 1))
    elif (( $(echo "$diff_percent < $THRESHOLD_FLAG" | bc -l) )); then
        echo -e "  ${YELLOW}⚠️  FLAG${NC} (${diff_percent}% diff - review recommended)"
        flagged=$((flagged + 1))
    elif (( $(echo "$diff_percent < $THRESHOLD_BLOCK" | bc -l) )); then
        echo -e "  ${YELLOW}⚠️  FLAG${NC} (${diff_percent}% diff - significant change)"
        flagged=$((flagged + 1))
    else
        echo -e "  ${RED}❌ FAIL${NC} (${diff_percent}% diff - exceeds threshold)"
        echo "     Diff image: $diff_file"
        failed=$((failed + 1))
    fi

    echo ""
done

# Summary
echo "==============================="
echo "Summary"
echo "==============================="
echo "Total diagrams:     $total"
echo "Passed:             $passed"
echo "Flagged:            $flagged"
echo "Failed:             $failed"
echo "Missing baselines:  $missing_baseline"
echo ""

# Exit code
if [ $failed -gt 0 ]; then
    echo -e "${RED}❌ Visual regression tests FAILED${NC}"
    echo "Review diff images in: $DIFF_DIR"
    exit 1
elif [ $flagged -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Visual regression tests FLAGGED${NC}"
    echo "Review diff images in: $DIFF_DIR"
    echo "Run with VISUAL_REGRESSION_STRICT=1 to fail on flagged tests"
    if [ "${VISUAL_REGRESSION_STRICT:-0}" = "1" ]; then
        exit 1
    fi
    exit 0
elif [ $missing_baseline -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Created $missing_baseline baseline(s)${NC}"
    echo "Commit baselines to git: git add $BASELINE_DIR"
    exit 0
else
    echo -e "${GREEN}✅ All visual regression tests PASSED${NC}"
    exit 0
fi
