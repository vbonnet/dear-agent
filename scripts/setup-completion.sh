#!/usr/bin/env bash
# Setup bash completion for csm (Claude Session Manager)
#
# This script installs the csm bash completion that prevents file fallback
# and properly strips Cobra command descriptions.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPLETION_SOURCE="${SCRIPT_DIR}/csm-completion.bash"
COMPLETION_DEST="${HOME}/.csm-completion.bash"
BASHRC="${HOME}/.bashrc"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Setting up bash completion for csm..."
echo ""

# Check if csm is installed
if ! command -v csm &> /dev/null; then
    echo -e "${YELLOW}Warning: csm command not found in PATH${NC}"
    echo "Make sure to install csm first (go install ./cmd/csm)"
    echo "Or add it to your PATH before using completion"
    echo ""
fi

# Copy completion script
echo "Installing completion script to ${COMPLETION_DEST}..."
cp "${COMPLETION_SOURCE}" "${COMPLETION_DEST}"
echo -e "${GREEN}✓${NC} Completion script installed"
echo ""

# Check if already sourced in bashrc
if grep -q "\.csm-completion\.bash" "${BASHRC}" 2>/dev/null; then
    echo -e "${YELLOW}Note:${NC} Completion already configured in ${BASHRC}"
    echo "If you're experiencing issues, try removing old entries and re-running this script"
else
    # Add to bashrc
    echo "Adding completion to ${BASHRC}..."
    cat >> "${BASHRC}" << 'EOF'

# AGM (Claude Session Manager) completion
if [ -f ~/.csm-completion.bash ]; then
    source ~/.csm-completion.bash
fi
EOF
    echo -e "${GREEN}✓${NC} Added completion to ${BASHRC}"
fi

echo ""
echo -e "${GREEN}Setup complete!${NC}"
echo ""
echo "To use completion in your current shell:"
echo "  source ~/.csm-completion.bash"
echo ""
echo "Or start a new terminal session."
echo ""
echo "Test with:"
echo "  csm k<TAB>        # Should complete to 'kill'"
echo "  csm kill <TAB>    # Should list session names"
