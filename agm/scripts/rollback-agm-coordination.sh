#!/usr/bin/env bash
# AGM Multi-Session Coordination - Rollback Script
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
AGM_HOME="${HOME}/.agm"
CLAUDE_HOME="${HOME}/.claude"
BACKUP_DIR="${1:-}"

usage() {
    cat <<EOF
Usage: $0 <backup_directory>

Rollback AGM coordination features to pre-installation state.

Arguments:
  backup_directory  Path to backup created during installation
                    (e.g., ~/.agm/backups/20260220_143000)

Example:
  $0 ~/.agm/backups/20260220_143000

EOF
    exit 1
}

stop_daemon() {
    log_info "Stopping AGM daemon..."

    if systemctl --user is-active agm-daemon.service &>/dev/null; then
        systemctl --user stop agm-daemon.service
        log_success "Daemon stopped"
    else
        log_info "Daemon not running"
    fi
}

disable_systemd_service() {
    log_info "Disabling systemd service..."

    if systemctl --user is-enabled agm-daemon.service &>/dev/null; then
        systemctl --user disable agm-daemon.service
        log_success "Service disabled"
    fi

    local service_file="${HOME}/.config/systemd/user/agm-daemon.service"
    if [ -f "$service_file" ]; then
        rm "$service_file"
        systemctl --user daemon-reload
        log_success "Service file removed"
    fi
}

remove_hooks() {
    log_info "Removing Claude hooks..."

    local hooks=(
        "${CLAUDE_HOME}/hooks/posttool-agm-state-notify"
        "${CLAUDE_HOME}/hooks/session-start/agm-state-ready"
    )

    for hook in "${hooks[@]}"; do
        if [ -f "$hook" ]; then
            rm "$hook"
            log_success "Removed: $hook"
        fi
    done
}

restore_from_backup() {
    log_info "Restoring from backup: $BACKUP_DIR"

    if [ ! -d "$BACKUP_DIR" ]; then
        log_error "Backup directory not found: $BACKUP_DIR"
        return 1
    fi

    # Restore config
    if [ -f "${BACKUP_DIR}/config.yaml.bak" ]; then
        cp "${BACKUP_DIR}/config.yaml.bak" "${AGM_HOME}/config.yaml"
        log_success "Restored config.yaml"
    fi

    # Restore sessions
    if [ -d "${BACKUP_DIR}/sessions.bak" ]; then
        rm -rf "${AGM_HOME}/sessions"
        cp -r "${BACKUP_DIR}/sessions.bak" "${AGM_HOME}/sessions"
        log_success "Restored sessions directory"
    fi

    # Restore queue database
    if [ -f "${BACKUP_DIR}/queue.db.bak" ]; then
        sqlite3 "${AGM_HOME}/queue.db" ".restore '${BACKUP_DIR}/queue.db.bak'"
        log_success "Restored queue database"
    fi
}

cleanup_queue() {
    log_warn "Removing message queue database..."

    read -p "This will delete all queued messages. Continue? (y/N) " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        if [ -f "${AGM_HOME}/queue.db" ]; then
            rm "${AGM_HOME}/queue.db"
            rm -f "${AGM_HOME}/queue.db-shm" "${AGM_HOME}/queue.db-wal"
            log_success "Queue database removed"
        fi
    else
        log_info "Keeping queue database"
    fi
}

revert_manifest_migrations() {
    log_info "Reverting session manifest migrations..."

    local reverted=0

    # Find all session manifests and remove state fields
    while IFS= read -r manifest; do
        if python3 -c "
import json
import sys

try:
    with open('$manifest', 'r') as f:
        data = json.load(f)

    # Remove coordination state fields
    changed = False
    for field in ['state', 'state_updated_at', 'state_updated_by']:
        if field in data:
            del data[field]
            changed = True

    if changed:
        with open('$manifest', 'w') as f:
            json.dump(data, f, indent=2)
        sys.exit(0)
    else:
        sys.exit(2)  # No changes needed
except Exception as e:
    print(f'Error: {e}', file=sys.stderr)
    sys.exit(1)
"; then
            ((reverted++))
            log_success "Reverted: $manifest"
        fi
    done < <(find "${AGM_HOME}/sessions" -name "manifest.json" 2>/dev/null || true)

    if [ $reverted -gt 0 ]; then
        log_success "Reverted $reverted session manifests"
    else
        log_info "No manifests required reversion"
    fi
}

verify_rollback() {
    log_info "Verifying rollback..."

    local issues=0

    # Check hooks removed
    if [ ! -f "${CLAUDE_HOME}/hooks/posttool-agm-state-notify" ] && \
       [ ! -f "${CLAUDE_HOME}/hooks/session-start/agm-state-ready" ]; then
        log_success "✓ Hooks removed"
    else
        log_warn "⚠ Some hooks still present"
        ((issues++))
    fi

    # Check daemon stopped
    if ! systemctl --user is-active agm-daemon.service &>/dev/null; then
        log_success "✓ Daemon stopped"
    else
        log_warn "⚠ Daemon still running"
        ((issues++))
    fi

    # Check service disabled
    if ! systemctl --user is-enabled agm-daemon.service &>/dev/null; then
        log_success "✓ Service disabled"
    else
        log_warn "⚠ Service still enabled"
        ((issues++))
    fi

    if [ $issues -eq 0 ]; then
        log_success "Rollback verified successfully!"
        return 0
    else
        log_warn "Rollback completed with $issues warnings"
        return 0
    fi
}

main() {
    if [ -z "$BACKUP_DIR" ]; then
        usage
    fi

    echo -e "${BLUE}"
    cat <<'EOF'
╔═══════════════════════════════════════════════════════════════════════╗
║                                                                       ║
║   AGM Multi-Session Coordination - Rollback Script                   ║
║   Phase 3 Task 3.1: Installation & Migration                         ║
║                                                                       ║
╚═══════════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"

    log_warn "This will rollback AGM coordination features"
    read -p "Continue? (y/N) " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Rollback cancelled"
        exit 0
    fi

    # Execute rollback steps
    stop_daemon
    disable_systemd_service
    remove_hooks
    restore_from_backup
    cleanup_queue
    revert_manifest_migrations
    verify_rollback

    cat <<EOF

${GREEN}=============================================================================
Rollback Complete!
=============================================================================${NC}

${BLUE}What was rolled back:${NC}
  - AGM daemon stopped and disabled
  - Claude hooks removed
  - Session manifests reverted to v2 format
  - Configuration restored from backup
  - Queue database removed (optional)

${BLUE}What was preserved:${NC}
  - AGM binary (still available in PATH)
  - Session data and history
  - Backup directory: ${BACKUP_DIR}

${BLUE}Manual cleanup (optional):${NC}
  To completely remove AGM coordination:
    rm -rf ~/.agm/logs/daemon
    rm -rf ~/.agm/queue

EOF

    log_success "Rollback completed successfully!"
}

# Run main if executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
