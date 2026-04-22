#!/usr/bin/env bash
# AGM Corpus Callosum Schema Unregistration
# Removes AGM schemas from Corpus Callosum protocol (if available)

set -euo pipefail

# Check if Corpus Callosum CLI is available
if ! command -v cc &> /dev/null; then
    echo "INFO: Corpus Callosum CLI not found - nothing to unregister"
    exit 0
fi

# Unregister schema from Corpus Callosum
echo "Unregistering AGM schemas from Corpus Callosum..."
if cc unregister --component=agm --version=1.0.0; then
    echo "✓ AGM schemas unregistered successfully"
else
    echo "WARNING: Failed to unregister AGM schemas (non-fatal)"
    exit 0  # Non-fatal: uninstall should proceed
fi
