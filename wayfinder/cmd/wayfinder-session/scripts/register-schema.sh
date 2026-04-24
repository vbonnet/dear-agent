#!/bin/bash
# register-schema.sh - Register Wayfinder schema with Corpus Callosum
# Called during Wayfinder installation/component setup

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_FILE="${SCRIPT_DIR}/../schema/wayfinder-v1.schema.json"

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
    echo "⚠️  Corpus Callosum not installed - skipping schema registration"
    echo "   Wayfinder will work normally, but cross-component discovery will be unavailable"
    exit 0
fi

echo "Registering Wayfinder schema with Corpus Callosum..."

# Register schema
if "$CC_BIN" register --component wayfinder --schema "$SCHEMA_FILE" 2>&1; then
    echo "✅ Wayfinder schema registered successfully"

    # Verify registration
    if "$CC_BIN" discover --component wayfinder &> /dev/null; then
        echo "✅ Schema verification passed"
    else
        echo "⚠️  Schema registered but verification failed"
    fi
else
    echo "⚠️  Schema registration failed - Wayfinder will work without Corpus Callosum integration"
    exit 0
fi
