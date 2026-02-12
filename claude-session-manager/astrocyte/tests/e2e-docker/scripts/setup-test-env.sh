#!/bin/bash
# Setup test environment for astrocyte E2E tests
# Creates tmux server, initializes directories, and prepares test fixtures

set -euo pipefail

# Configuration
TEST_HOME="/home/testuser"
SESSIONS_DIR="$TEST_HOME/src/sessions"
ASTROCYTE_DIR="$TEST_HOME/.agm/astrocyte"
TMUX_SOCKET="/tmp/csm.sock"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

# Create directory structure
setup_directories() {
    log_info "Setting up directory structure..."

    mkdir -p "$SESSIONS_DIR"
    mkdir -p "$ASTROCYTE_DIR"/{diagnoses,prompts,logs}
    mkdir -p "$TEST_HOME/test-results"

    log_info "Directories created"
}

# Start tmux server with CSM socket
start_tmux_server() {
    log_info "Starting tmux server..."

    # Kill any existing tmux server
    tmux -S "$TMUX_SOCKET" kill-server 2>/dev/null || true

    # Start new tmux server
    tmux -S "$TMUX_SOCKET" new-session -d -s init-session

    # Verify tmux server is running
    if tmux -S "$TMUX_SOCKET" list-sessions >/dev/null 2>&1; then
        log_info "Tmux server started on socket: $TMUX_SOCKET"
    else
        log_error "Failed to start tmux server"
        return 1
    fi

    # Kill the init session
    tmux -S "$TMUX_SOCKET" kill-session -t init-session 2>/dev/null || true
}

# Create astrocyte configuration
create_config() {
    log_info "Creating astrocyte configuration..."

    cat > "$ASTROCYTE_DIR/config.yaml" <<EOF
# Astrocyte E2E Test Configuration

interval_seconds: 5  # Fast checks for testing

thresholds:
  mustering_timeout: 1        # 1 minute for fast testing
  zero_token_waiting: 1       # 1 minute for fast testing
  cursor_frozen: 2            # 2 minutes for testing
  permission_prompt_duration: 1  # 1 minute for testing

slack:
  enabled: false

email:
  enabled: false

recovery:
  enabled: true
  method: "escape"
  max_attempts: 1

logging:
  incidents_file: "$ASTROCYTE_DIR/incidents.jsonl"
  diagnoses_dir: "$ASTROCYTE_DIR/diagnoses"
  verbose: true

diagnosis:
  enabled: true
  use_csm_prompt_file: true
  fallback_to_tmux: false

remote:
  enabled: false
EOF

    log_info "Configuration created at $ASTROCYTE_DIR/config.yaml"
}

# Main setup
main() {
    log_info "Starting test environment setup..."

    setup_directories
    start_tmux_server
    create_config

    log_info "Test environment setup complete"
}

main "$@"
