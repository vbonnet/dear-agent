#!/bin/bash
# Deploy Go hooks to ~/.claude/hooks/
# Note: Uses cp for binaries >256KB (Read tool limit)

set -euo pipefail

HOOKS_DIR="$HOME/.claude/hooks"
BIN_DIR="$(cd "$(dirname "$0")" && pwd)/bin"

echo "Deploying Go hooks to $HOOKS_DIR..."

# Create hooks directory if it doesn't exist
mkdir -p "$HOOKS_DIR"

# Detect platform suffix for binary names
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
esac

SUFFIX=""
if [ "$OS" != "linux" ]; then
    SUFFIX="-${OS}-${ARCH}"
fi

HOOKS=(
    posttool-error-collector
    prepush-act-validator
    sessionstart-bead-coverage
    sessionend-bead-coverage
)

DEPLOYED=0
for hook in "${HOOKS[@]}"; do
    SRC="$BIN_DIR/${hook}${SUFFIX}"
    if [ ! -f "$SRC" ]; then
        # Fall back to unsuffixed binary (Linux default)
        SRC="$BIN_DIR/$hook"
    fi
    if [ -f "$SRC" ]; then
        cp "$SRC" "$HOOKS_DIR/engram-$hook"
        chmod +x "$HOOKS_DIR/engram-$hook"
        DEPLOYED=$((DEPLOYED + 1))
    else
        echo "  WARNING: $hook binary not found (tried ${hook}${SUFFIX} and ${hook})"
    fi
done

echo "Deployed $DEPLOYED Go hooks:"
for hook in "${HOOKS[@]}"; do
    if [ -f "$HOOKS_DIR/engram-$hook" ]; then
        SIZE=$(ls -lh "$HOOKS_DIR/engram-$hook" | awk '{print $5}')
        echo "  $HOOKS_DIR/engram-$hook ($SIZE)"
    fi
done

echo ""
echo "Next step: Update ~/.claude/settings.json to use Go hooks instead of Python"
