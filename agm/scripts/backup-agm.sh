#!/usr/bin/env bash
# AGM Backup Script
# Creates comprehensive backup of AGM state for safe migration/rollback

set -euo pipefail

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration
AGM_HOME="${HOME}/.agm"
CLAUDE_HOME="${HOME}/.claude"
BACKUP_ROOT="${AGM_HOME}/backups"
BACKUP_DIR="${BACKUP_ROOT}/$(date +%Y%m%d_%H%M%S)"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

main() {
    log_info "Creating AGM backup..."
    log_info "Backup location: $BACKUP_DIR"

    # Create backup directory
    mkdir -p "$BACKUP_DIR"

    # Backup configuration
    if [ -f "${AGM_HOME}/config.yaml" ]; then
        cp "${AGM_HOME}/config.yaml" "${BACKUP_DIR}/config.yaml"
        log_success "Backed up config.yaml"
    fi

    # Backup all sessions
    if [ -d "${AGM_HOME}/sessions" ]; then
        cp -r "${AGM_HOME}/sessions" "${BACKUP_DIR}/sessions"
        local session_count=$(find "${AGM_HOME}/sessions" -name "manifest.json" | wc -l)
        log_success "Backed up $session_count sessions"
    fi

    # Backup queue database
    if [ -f "${AGM_HOME}/queue.db" ]; then
        sqlite3 "${AGM_HOME}/queue.db" ".backup '${BACKUP_DIR}/queue.db'"
        local message_count=$(sqlite3 "${AGM_HOME}/queue.db" "SELECT COUNT(*) FROM message_queue;" 2>/dev/null || echo 0)
        log_success "Backed up queue database ($message_count messages)"
    fi

    # Backup hooks
    if [ -d "${CLAUDE_HOME}/hooks" ]; then
        cp -r "${CLAUDE_HOME}/hooks" "${BACKUP_DIR}/claude-hooks"
        log_success "Backed up Claude hooks"
    fi

    # Backup daemon logs (last 7 days)
    if [ -d "${AGM_HOME}/logs/daemon" ]; then
        mkdir -p "${BACKUP_DIR}/logs"
        find "${AGM_HOME}/logs/daemon" -name "*.log" -mtime -7 -exec cp {} "${BACKUP_DIR}/logs/" \;
        log_success "Backed up daemon logs"
    fi

    # Create backup manifest
    cat > "${BACKUP_DIR}/MANIFEST.txt" <<EOF
AGM Backup Manifest
===================
Created: $(date -Iseconds)
Hostname: $(hostname)
User: $(whoami)
AGM Version: $(agm --version 2>/dev/null || echo "unknown")

Contents:
$(tree -L 2 "$BACKUP_DIR" 2>/dev/null || find "$BACKUP_DIR" -type f)

Backup Size: $(du -sh "$BACKUP_DIR" | cut -f1)

Restore Command:
  scripts/rollback-agm-coordination.sh "$BACKUP_DIR"
EOF

    # Compress backup (optional)
    if command -v tar &>/dev/null; then
        log_info "Compressing backup..."
        tar -czf "${BACKUP_DIR}.tar.gz" -C "${BACKUP_ROOT}" "$(basename "$BACKUP_DIR")"
        log_success "Compressed backup: ${BACKUP_DIR}.tar.gz"
    fi

    # Print summary
    cat <<EOF

${GREEN}=============================================================================
Backup Complete!
=============================================================================${NC}

${BLUE}Backup Location:${NC}
  Directory: $BACKUP_DIR
  Archive:   ${BACKUP_DIR}.tar.gz

${BLUE}Backup Contents:${NC}
$(cat "${BACKUP_DIR}/MANIFEST.txt")

${BLUE}Restore:${NC}
  scripts/rollback-agm-coordination.sh "$BACKUP_DIR"

EOF

    log_success "Backup completed successfully!"
}

main "$@"
