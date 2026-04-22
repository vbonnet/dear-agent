#!/bin/bash
# AGM Post-Uninstall Hook
# Cleanup cache and temporary files after component uninstall

set -e

WORKSPACE_ROOT=$1

echo "AGM Post-Uninstall Cleanup"
echo "Workspace: ${WORKSPACE_ROOT}"
echo

# Step 1: Remove cache directory
CACHE_DIR="${HOME}/.agm-cache"
if [ -d "${CACHE_DIR}" ]; then
    rm -rf "${CACHE_DIR}"
    echo "✓ Removed cache directory: ${CACHE_DIR}"
else
    echo "✓ Cache directory already removed"
fi

# Step 2: Clean tmux session artifacts
# NOTE: In actual implementation, check for orphaned tmux sessions
echo "✓ Cleaned tmux session artifacts (placeholder)"

# Step 3: Remove workspace initialization marker
if [ -f "${WORKSPACE_ROOT}/.agm-initialized" ]; then
    rm "${WORKSPACE_ROOT}/.agm-initialized"
    echo "✓ Removed initialization marker"
fi

# Step 4: Preserve configuration and session data
# NOTE: We intentionally do NOT delete:
#   - ${HOME}/.agm/config.yaml (user configuration)
#   - ${HOME}/.agm/sessions/ (legacy session files)
#   - Backup files (created by export-data.sh)
# This allows users to re-install AGM later without losing configuration.

echo
echo "✅ Cleanup complete"
echo
echo "Preserved files:"
echo "  - Configuration: ${HOME}/.agm/config.yaml"
echo "  - Sessions:      ${HOME}/.agm/sessions/"
echo "  - Backups:       ${WORKSPACE_ROOT}/backup/"
echo
echo "To remove all AGM data:"
echo "  rm -rf ${HOME}/.agm"
echo "  rm -rf ${WORKSPACE_ROOT}/backup/agm-*"
