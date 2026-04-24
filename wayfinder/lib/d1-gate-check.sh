#!/bin/bash
set -euo pipefail

# d1-gate-check.sh: D1 auth/security keyword detection
# Escalates XS/S → M when auth/security keywords detected

PROJECT_DIR="${1:-.}"
CHARTER_PATH="$PROJECT_DIR/W0-charter.md"
D1_DELIVERABLE="$PROJECT_DIR/D1-problem-validation.md"

# Exit silently if files don't exist yet
if [[ ! -f "$CHARTER_PATH" ]] && [[ ! -f "$D1_DELIVERABLE" ]]; then
    exit 0
fi

# Auth/security keywords (case-insensitive)
AUTH_KEYWORDS="auth|authentication|authorization|security|crypto|cryptography|secrets|credentials|token|session|permission|access control"

# Check for keywords in charter and D1 deliverable (exclude .md files for context-aware detection)
if grep -Eiq "$AUTH_KEYWORDS" "$CHARTER_PATH" "$D1_DELIVERABLE" 2>/dev/null; then
    echo "🚨 D1 Gate Check: Security/auth keywords detected"

    # Extract matched keywords
    MATCHED_KEYWORDS=$(grep -Eioh "$AUTH_KEYWORDS" "$CHARTER_PATH" "$D1_DELIVERABLE" 2>/dev/null | sort -u | tr '\n' ',' | sed 's/,$//')
    echo "Keywords found: $MATCHED_KEYWORDS"

    # Get current depth
    CURRENT_DEPTH=$(wayfinder -C "$PROJECT_DIR" session status --field depth 2>/dev/null || echo "S")

    # Escalation logic
    case "$CURRENT_DEPTH" in
        XS)
            echo "📈 Escalating XS → M (+22m for security validation)"
            TARGET_DEPTH="M"
            ;;
        S)
            echo "📈 Escalating S → M (+12m for security focus)"
            TARGET_DEPTH="M"
            ;;
        M|L|XL)
            echo "ℹ️  Depth already ${CURRENT_DEPTH} (no escalation needed)"
            exit 0
            ;;
        *)
            echo "⚠️  Unknown depth: $CURRENT_DEPTH (defaulting to M)"
            TARGET_DEPTH="M"
            ;;
    esac

    # Attempt escalation (graceful fallback if command fails)
    if wayfinder -C "$PROJECT_DIR" session escalate \
        --to "$TARGET_DEPTH" \
        --trigger "D1: auth/security keywords" \
        --reason "Security-critical changes require deeper validation" 2>/dev/null; then
        echo "✅ Escalation successful: ${CURRENT_DEPTH} → ${TARGET_DEPTH}"
    else
        echo "⚠️  Escalation command failed (wayfinder session escalate not available)"
        echo "ℹ️  Recommendation: Manually set depth to $TARGET_DEPTH"
    fi
else
    echo "✅ D1 Gate Check: No auth/security keywords detected"
fi
