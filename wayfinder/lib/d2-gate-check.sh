#!/bin/bash
set -euo pipefail

# d2-gate-check.sh: D2 CVE scanning for dependency changes
# Escalates any depth → L when CVEs found

PROJECT_DIR="${1:-.}"
D2_DELIVERABLE="$PROJECT_DIR/D2-existing-solutions.md"

# Exit silently if D2 deliverable doesn't exist yet
if [[ ! -f "$D2_DELIVERABLE" ]]; then
    exit 0
fi

# Auto-detect package manager
PACKAGE_MANAGER=""
CVE_SCANNER=""

if [[ -f "$PROJECT_DIR/package.json" ]]; then
    PACKAGE_MANAGER="npm"
    CVE_SCANNER="npm audit"
elif [[ -f "$PROJECT_DIR/go.mod" ]]; then
    PACKAGE_MANAGER="go"
    CVE_SCANNER="govulncheck"
elif [[ -f "$PROJECT_DIR/requirements.txt" ]]; then
    PACKAGE_MANAGER="python"
    CVE_SCANNER="safety"
elif [[ -f "$PROJECT_DIR/Cargo.toml" ]]; then
    PACKAGE_MANAGER="rust"
    CVE_SCANNER="cargo audit"
else
    echo "ℹ️  D2 Gate Check: No package manager detected (skipping CVE scan)"
    exit 0
fi

echo "🔍 D2 Gate Check: Running CVE scan ($CVE_SCANNER)..."

# Run CVE scanner
CVE_OUTPUT=""
CVE_COUNT=0

case "$PACKAGE_MANAGER" in
    npm)
        if command -v npm &> /dev/null; then
            CVE_OUTPUT=$(npm audit --json 2>/dev/null || echo "{}")
            CVE_COUNT=$(echo "$CVE_OUTPUT" | jq -r '.metadata.vulnerabilities.total // 0' 2>/dev/null || echo "0")
        else
            echo "⚠️  npm not installed (skipping scan)"
            exit 0
        fi
        ;;
    go)
        if command -v govulncheck &> /dev/null; then
            CVE_OUTPUT=$(govulncheck ./... 2>&1 || true)
            CVE_COUNT=$(echo "$CVE_OUTPUT" | grep -c "Vulnerability" || echo "0")
        else
            echo "⚠️  govulncheck not installed (skipping scan)"
            exit 0
        fi
        ;;
    python)
        if command -v safety &> /dev/null; then
            CVE_OUTPUT=$(safety check 2>&1 || true)
            CVE_COUNT=$(echo "$CVE_OUTPUT" | grep -c "vulnerability" || echo "0")
        else
            echo "⚠️  safety not installed (skipping scan)"
            exit 0
        fi
        ;;
    rust)
        if command -v cargo &> /dev/null; then
            CVE_OUTPUT=$(cargo audit 2>&1 || true)
            CVE_COUNT=$(echo "$CVE_OUTPUT" | grep -c "vulnerability" || echo "0")
        else
            echo "⚠️  cargo not installed (skipping scan)"
            exit 0
        fi
        ;;
esac

# Escalate if CVEs found
if [[ "$CVE_COUNT" -gt 0 ]]; then
    echo "🚨 D2 Gate Check: $CVE_COUNT CVE(s) found"
    echo ""
    echo "Output excerpt:"
    echo "$CVE_OUTPUT" | head -15
    echo ""

    # Get current depth
    CURRENT_DEPTH=$(wayfinder -C "$PROJECT_DIR" session status --field depth 2>/dev/null || echo "S")

    # Escalate to L (52m)
    TARGET_DEPTH="L"

    if [[ "$CURRENT_DEPTH" == "XL" ]]; then
        echo "ℹ️  Depth already XL (no escalation needed)"
        exit 0
    elif [[ "$CURRENT_DEPTH" == "L" ]]; then
        echo "ℹ️  Depth already L (no escalation needed)"
        exit 0
    fi

    echo "📈 Escalating ${CURRENT_DEPTH} → ${TARGET_DEPTH} (+CVE remediation time)"

    # Attempt escalation (graceful fallback if command fails)
    if wayfinder -C "$PROJECT_DIR" session escalate \
        --to "$TARGET_DEPTH" \
        --trigger "D2: $CVE_COUNT CVE(s) detected" \
        --reason "CVE remediation requires additional security review and patching" 2>/dev/null; then
        echo "✅ Escalation successful: ${CURRENT_DEPTH} → ${TARGET_DEPTH}"
    else
        echo "⚠️  Escalation command failed (wayfinder session escalate not available)"
        echo "ℹ️  Recommendation: Manually set depth to $TARGET_DEPTH"
    fi
else
    echo "✅ D2 Gate Check: No CVEs detected"
fi
