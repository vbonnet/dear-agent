#!/usr/bin/env bash
# AGM Corpus Callosum Schema Registration
# Registers AGM schemas with Corpus Callosum protocol (if available)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_FILE="${SCRIPT_DIR}/../schemas/corpus-callosum-schema.json"

# Check if Corpus Callosum CLI is available
if ! command -v cc &> /dev/null; then
    echo "INFO: Corpus Callosum CLI not found - skipping schema registration (graceful degradation)"
    exit 0
fi

# Check if schema file exists
if [ ! -f "$SCHEMA_FILE" ]; then
    echo "ERROR: Schema file not found: $SCHEMA_FILE"
    exit 1
fi

# Register schema with Corpus Callosum
echo "Registering AGM schemas with Corpus Callosum..."
if cc register --schema="$SCHEMA_FILE" --component=agm --version=1.0.0; then
    echo "✓ AGM schemas registered successfully"
else
    echo "WARNING: Failed to register AGM schemas (non-fatal - component will function without Corpus Callosum)"
    exit 0  # Non-fatal: AGM works without Corpus Callosum
fi
