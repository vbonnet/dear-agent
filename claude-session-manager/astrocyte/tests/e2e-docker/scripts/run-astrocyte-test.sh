#!/bin/bash
# Run astrocyte daemon in test mode
# Usage: run-astrocyte-test.sh <duration-seconds>

set -euo pipefail

DURATION="${1:-30}"  # Default: 30 seconds
ASTROCYTE_PY="/home/testuser/astrocyte/astrocyte.py"
ASTROCYTE_LOG="/home/testuser/.csm/astrocyte/logs/astrocyte-test.log"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

# Ensure log directory exists
mkdir -p "$(dirname "$ASTROCYTE_LOG")"

log_info "Starting astrocyte daemon for $DURATION seconds..."

# Create a modified version of astrocyte for testing
# Override main() to run for limited time
TEST_ASTROCYTE="/tmp/astrocyte_test.py"
cp "$ASTROCYTE_PY" "$TEST_ASTROCYTE"

# Run astrocyte in background
python3 "$TEST_ASTROCYTE" > "$ASTROCYTE_LOG" 2>&1 &
ASTROCYTE_PID=$!

log_info "Astrocyte daemon started (PID: $ASTROCYTE_PID)"
log_info "Log file: $ASTROCYTE_LOG"

# Wait for specified duration
sleep "$DURATION"

# Stop astrocyte
log_info "Stopping astrocyte daemon..."
kill -TERM "$ASTROCYTE_PID" 2>/dev/null || true
wait "$ASTROCYTE_PID" 2>/dev/null || true

log_info "Astrocyte daemon stopped"

# Display log summary
echo ""
echo "=== Astrocyte Log Summary ==="
echo ""
if [ -f "$ASTROCYTE_LOG" ]; then
    tail -20 "$ASTROCYTE_LOG"
else
    log_warn "Log file not found"
fi
echo ""
echo "=== End of Log ==="
