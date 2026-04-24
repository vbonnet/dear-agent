#!/bin/bash
# unregister-schema.sh - Unregister Wayfinder schema from Corpus Callosum
# Called during Wayfinder uninstallation/component removal

set -e

# Find Corpus Callosum CLI (not the C compiler)
CC_BIN=""
if command -v corpus-callosum &> /dev/null; then
    CC_BIN="corpus-callosum"
elif [ -f "${HOME}/src/ws/oss/repos/ai-tools/main/corpus-callosum/cc" ]; then
    CC_BIN="${HOME}/src/ws/oss/repos/ai-tools/main/corpus-callosum/cc"
elif [ -f "${GOPATH}/bin/cc" ]; then
    # Check if it's the Corpus Callosum version
    if "${GOPATH}/bin/cc" version --format json 2>&1 | grep -q "protocol"; then
        CC_BIN="${GOPATH}/bin/cc"
    fi
fi

if [ -z "$CC_BIN" ]; then
    echo "Corpus Callosum not installed - nothing to unregister"
    exit 0
fi

echo "Unregistering Wayfinder schema from Corpus Callosum..."

# Unregister schema
if "$CC_BIN" unregister --component wayfinder --confirm 2>&1; then
    echo "✅ Wayfinder schema unregistered successfully"
else
    echo "⚠️  Schema unregistration failed (may not have been registered)"
    exit 0
fi
