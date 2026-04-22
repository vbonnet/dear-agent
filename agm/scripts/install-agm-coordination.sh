#!/usr/bin/env bash
# AGM Multi-Session Coordination - Complete Installation Script
# Phase 3 Task 3.1: Installation & Migration
# Bead: oss-5wpp

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
AGM_HOME="${HOME}/.agm"
CLAUDE_HOME="${HOME}/.claude"
BACKUP_DIR="${AGM_HOME}/backups/$(date +%Y%m%d_%H%M%S)"

# Functions
check_dependencies() {
    log_info "Checking dependencies..."

    local missing_deps=()

    # Check for required binaries
    for cmd in go tmux sqlite3; do
        if ! command -v "$cmd" &>/dev/null; then
            missing_deps+=("$cmd")
        fi
    done

    if [ ${#missing_deps[@]} -gt 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Install them with: apt install ${missing_deps[*]} (Ubuntu/Debian)"
        return 1
    fi

    log_success "All dependencies installed"
    return 0
}

create_directories() {
    log_info "Creating directory structure..."

    mkdir -p "${AGM_HOME}"/{sessions,logs/{daemon,hooks},backups,queue}
    mkdir -p "${CLAUDE_HOME}/hooks/session-start"

    log_success "Directories created"
}

backup_existing_config() {
    log_info "Backing up existing configuration..."

    mkdir -p "${BACKUP_DIR}"

    # Backup existing AGM files
    if [ -f "${AGM_HOME}/config.yaml" ]; then
        cp "${AGM_HOME}/config.yaml" "${BACKUP_DIR}/config.yaml.bak"
        log_success "Backed up config.yaml"
    fi

    # Backup existing sessions
    if [ -d "${AGM_HOME}/sessions" ]; then
        cp -r "${AGM_HOME}/sessions" "${BACKUP_DIR}/sessions.bak"
        log_success "Backed up sessions directory"
    fi

    # Backup queue database if it exists
    if [ -f "${AGM_HOME}/queue.db" ]; then
        sqlite3 "${AGM_HOME}/queue.db" ".backup '${BACKUP_DIR}/queue.db.bak'"
        log_success "Backed up queue database"
    fi

    log_success "Backup completed: ${BACKUP_DIR}"
}

install_hooks() {
    log_info "Installing Claude hooks..."

    # Install hooks using AGM command
    if command -v agm &>/dev/null; then
        agm admin install-hooks
        log_success "Hooks installed via agm admin install-hooks"
    else
        log_warn "agm binary not found, installing hooks manually"

        # Copy hooks manually
        if [ -d "${PROJECT_ROOT}/cmd/agm/hooks" ]; then
            cp "${PROJECT_ROOT}/cmd/agm/hooks/posttool-agm-state-notify" \
               "${CLAUDE_HOME}/hooks/posttool-agm-state-notify"
            cp "${PROJECT_ROOT}/cmd/agm/hooks/session-start-agm-state-ready" \
               "${CLAUDE_HOME}/hooks/session-start/agm-state-ready"

            chmod +x "${CLAUDE_HOME}/hooks/posttool-agm-state-notify"
            chmod +x "${CLAUDE_HOME}/hooks/session-start/agm-state-ready"

            log_success "Hooks installed manually"
        else
            log_error "Hook files not found in ${PROJECT_ROOT}/cmd/agm/hooks"
            return 1
        fi
    fi
}

migrate_session_manifests() {
    log_info "Migrating existing sessions to v3 format..."

    local migrated=0
    local failed=0

    # Find all session manifests
    while IFS= read -r manifest; do
        # Check if manifest needs migration (check for 'state' field)
        if ! grep -q '"state"' "$manifest" 2>/dev/null; then
            log_info "Migrating: $manifest"

            # Add state field to manifest (OFFLINE by default)
            # This will be updated to READY when session starts
            if python3 -c "
import json
import sys

try:
    with open('$manifest', 'r') as f:
        data = json.load(f)

    # Add state fields if missing
    if 'state' not in data:
        data['state'] = 'OFFLINE'
        data['state_updated_at'] = '$(date -Iseconds)'
        data['state_updated_by'] = 'migration'

    with open('$manifest', 'w') as f:
        json.dump(data, f, indent=2)

    sys.exit(0)
except Exception as e:
    print(f'Error: {e}', file=sys.stderr)
    sys.exit(1)
"; then
                ((migrated++))
            else
                ((failed++))
                log_warn "Failed to migrate: $manifest"
            fi
        fi
    done < <(find "${AGM_HOME}/sessions" -name "manifest.json" 2>/dev/null || true)

    log_success "Migrated $migrated sessions ($failed failed)"
}

initialize_queue_database() {
    log_info "Initializing message queue database..."

    # The queue database will be created automatically when agm-daemon starts
    # or when the first message is queued. Just verify the directory exists.

    if [ ! -d "${AGM_HOME}/queue" ]; then
        mkdir -p "${AGM_HOME}/queue"
    fi

    log_success "Queue directory ready"
}

install_systemd_service() {
    log_info "Installing systemd service..."

    local service_file="${HOME}/.config/systemd/user/agm-daemon.service"
    local agm_daemon_bin

    # Find agm-daemon binary
    if command -v agm-daemon &>/dev/null; then
        agm_daemon_bin=$(command -v agm-daemon)
    elif [ -f "${PROJECT_ROOT}/agm-daemon" ]; then
        agm_daemon_bin="${PROJECT_ROOT}/agm-daemon"
    elif [ -f "${HOME}/bin/agm-daemon" ]; then
        agm_daemon_bin="${HOME}/bin/agm-daemon"
    else
        log_warn "agm-daemon binary not found. Build it first with: cd ${PROJECT_ROOT} && go build -o agm-daemon cmd/agm-daemon/*.go"
        return 1
    fi

    mkdir -p "${HOME}/.config/systemd/user"

    cat > "$service_file" <<EOF
[Unit]
Description=AGM Daemon - Multi-Session Message Delivery
Documentation=https://github.com/vbonnet/ai-tools/tree/main/agm
After=default.target

[Service]
Type=simple
ExecStart=${agm_daemon_bin}
Restart=always
RestartSec=10

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=agm-daemon

# Resource limits
MemoryMax=256M
CPUQuota=50%

# Security hardening
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=default.target
EOF

    # Reload systemd and enable service
    systemctl --user daemon-reload
    systemctl --user enable agm-daemon.service

    log_success "Systemd service installed: $service_file"
    log_info "Start with: systemctl --user start agm-daemon"
    log_info "Status with: systemctl --user status agm-daemon"
}

verify_installation() {
    log_info "Verifying installation..."

    local issues=0

    # Check hooks
    if [ -f "${CLAUDE_HOME}/hooks/posttool-agm-state-notify" ] && \
       [ -f "${CLAUDE_HOME}/hooks/session-start/agm-state-ready" ]; then
        log_success "✓ Hooks installed"
    else
        log_error "✗ Hooks missing"
        ((issues++))
    fi

    # Check directories
    for dir in "${AGM_HOME}/sessions" "${AGM_HOME}/logs/daemon" "${AGM_HOME}/backups"; do
        if [ -d "$dir" ]; then
            log_success "✓ Directory exists: $dir"
        else
            log_error "✗ Directory missing: $dir"
            ((issues++))
        fi
    done

    # Check systemd service
    if systemctl --user is-enabled agm-daemon.service &>/dev/null; then
        log_success "✓ Systemd service enabled"
    else
        log_warn "⚠ Systemd service not enabled (optional)"
    fi

    if [ $issues -eq 0 ]; then
        log_success "Installation verified successfully!"
        return 0
    else
        log_error "Installation verification failed with $issues issues"
        return 1
    fi
}

print_next_steps() {
    cat <<EOF

${GREEN}=============================================================================
Installation Complete!
=============================================================================${NC}

${BLUE}Next Steps:${NC}

1. ${YELLOW}Start the AGM daemon:${NC}
   systemctl --user start agm-daemon
   systemctl --user status agm-daemon

2. ${YELLOW}Verify daemon is running:${NC}
   agm daemon status

3. ${YELLOW}Test message delivery:${NC}
   # Create two test sessions
   agm new test-session-1
   agm new test-session-2

   # Send message from session-1 to session-2
   agm send test-session-1 test-session-2 "Hello from session 1!"

4. ${YELLOW}Enable user lingering (sessions persist after logout):${NC}
   loginctl enable-linger \$USER

5. ${YELLOW}View daemon logs:${NC}
   journalctl --user -u agm-daemon -f

${BLUE}Documentation:${NC}
  - Migration guide: docs/COORDINATION-MIGRATION.md
  - Daemon spec: cmd/agm-daemon/SPEC.md
  - Troubleshooting: docs/TROUBLESHOOTING.md

${BLUE}Backup Location:${NC}
  ${BACKUP_DIR}

${BLUE}Rollback:${NC}
  If you need to rollback, run: scripts/rollback-agm-coordination.sh

EOF
}

# Main execution
main() {
    echo -e "${BLUE}"
    cat <<'EOF'
╔═══════════════════════════════════════════════════════════════════════╗
║                                                                       ║
║   AGM Multi-Session Coordination - Installation Script               ║
║   Phase 3 Task 3.1: Installation & Migration                         ║
║                                                                       ║
╚═══════════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"

    log_info "Starting installation..."

    # Run installation steps
    check_dependencies || exit 1
    create_directories
    backup_existing_config
    install_hooks || exit 1
    migrate_session_manifests
    initialize_queue_database
    install_systemd_service || log_warn "Systemd service installation skipped"
    verify_installation || exit 1

    print_next_steps

    log_success "Installation completed successfully!"
}

# Run main if executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
