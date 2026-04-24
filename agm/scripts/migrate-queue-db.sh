#!/usr/bin/env bash
# AGM Queue Database Migration Script
# Handles schema versioning and upgrades

set -euo pipefail

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Configuration
AGM_HOME="${HOME}/.agm"
QUEUE_DB="${AGM_HOME}/queue.db"
BACKUP_DB="${QUEUE_DB}.backup-$(date +%Y%m%d_%H%M%S)"

# Migration versions
CURRENT_VERSION=1  # Latest schema version

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

get_schema_version() {
    if [ ! -f "$QUEUE_DB" ]; then
        echo "0"  # Database doesn't exist
        return
    fi

    local version=$(sqlite3 "$QUEUE_DB" "PRAGMA user_version;" 2>/dev/null || echo "0")
    echo "$version"
}

set_schema_version() {
    local version=$1
    sqlite3 "$QUEUE_DB" "PRAGMA user_version = $version;"
}

create_initial_schema() {
    log_info "Creating initial schema (v1)..."

    sqlite3 "$QUEUE_DB" <<'EOF'
-- Enable WAL mode for concurrent access
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;

-- Main message queue table
CREATE TABLE IF NOT EXISTS message_queue (
    message_id TEXT PRIMARY KEY,
    from_session TEXT NOT NULL,
    to_session TEXT NOT NULL,
    message TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    delivered_at TIMESTAMP,
    ack_required INTEGER NOT NULL DEFAULT 1,
    ack_received INTEGER NOT NULL DEFAULT 0,
    ack_timeout TIMESTAMP
);

-- Index for fast pending message lookups
CREATE INDEX IF NOT EXISTS idx_pending
ON message_queue(to_session, status, created_at)
WHERE status = 'pending';

-- Index for failed message queries
CREATE INDEX IF NOT EXISTS idx_failed
ON message_queue(status, created_at)
WHERE status = 'failed';

-- Index for acknowledgment timeouts
CREATE INDEX IF NOT EXISTS idx_ack_timeout
ON message_queue(ack_timeout)
WHERE ack_timeout IS NOT NULL AND ack_received = 0;

-- Set schema version
PRAGMA user_version = 1;
EOF

    log_success "Schema v1 created"
}

migrate_v1_to_v2() {
    # Placeholder for future migration
    log_info "Migrating from v1 to v2..."

    # Example future migration:
    # ALTER TABLE message_queue ADD COLUMN retry_after TIMESTAMP;

    set_schema_version 2
    log_success "Migrated to v2"
}

backup_database() {
    log_info "Creating backup: $BACKUP_DB"
    sqlite3 "$QUEUE_DB" ".backup '$BACKUP_DB'"
    log_success "Backup created"
}

verify_database() {
    log_info "Verifying database integrity..."

    # Check for corruption
    local integrity=$(sqlite3 "$QUEUE_DB" "PRAGMA integrity_check;" 2>/dev/null || echo "error")
    if [ "$integrity" != "ok" ]; then
        log_error "Database integrity check failed: $integrity"
        return 1
    fi

    # Verify tables exist
    local tables=$(sqlite3 "$QUEUE_DB" "SELECT name FROM sqlite_master WHERE type='table';" 2>/dev/null)
    if [[ ! "$tables" =~ "message_queue" ]]; then
        log_error "message_queue table not found"
        return 1
    fi

    # Verify indexes exist
    local indexes=$(sqlite3 "$QUEUE_DB" "SELECT name FROM sqlite_master WHERE type='index';" 2>/dev/null)
    if [[ ! "$indexes" =~ "idx_pending" ]]; then
        log_warn "idx_pending index not found (may impact performance)"
    fi

    log_success "Database verified"
    return 0
}

optimize_database() {
    log_info "Optimizing database..."

    sqlite3 "$QUEUE_DB" <<'EOF'
-- Analyze query optimizer statistics
ANALYZE;

-- Vacuum to reclaim space
VACUUM;

-- Optimize indexes
PRAGMA optimize;
EOF

    log_success "Database optimized"
}

print_stats() {
    log_info "Database statistics:"

    local db_size=$(du -h "$QUEUE_DB" | cut -f1)
    local message_count=$(sqlite3 "$QUEUE_DB" "SELECT COUNT(*) FROM message_queue;" 2>/dev/null || echo "0")
    local pending=$(sqlite3 "$QUEUE_DB" "SELECT COUNT(*) FROM message_queue WHERE status='pending';" 2>/dev/null || echo "0")
    local delivered=$(sqlite3 "$QUEUE_DB" "SELECT COUNT(*) FROM message_queue WHERE status='delivered';" 2>/dev/null || echo "0")
    local failed=$(sqlite3 "$QUEUE_DB" "SELECT COUNT(*) FROM message_queue WHERE status='failed';" 2>/dev/null || echo "0")

    cat <<EOF

  Database File: $QUEUE_DB
  Size: $db_size

  Messages:
    Total:     $message_count
    Pending:   $pending
    Delivered: $delivered
    Failed:    $failed

EOF
}

main() {
    echo -e "${BLUE}"
    cat <<'EOF'
╔═══════════════════════════════════════════════════════════════════════╗
║                                                                       ║
║   AGM Queue Database Migration                                       ║
║                                                                       ║
╚═══════════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"

    # Get current version
    local current=$(get_schema_version)
    log_info "Current schema version: v$current"
    log_info "Target schema version: v$CURRENT_VERSION"

    # Check if migration needed
    if [ "$current" -eq "$CURRENT_VERSION" ]; then
        log_success "Database already at latest version"
        verify_database || exit 1
        print_stats
        exit 0
    fi

    # Create directory if needed
    mkdir -p "$AGM_HOME"

    # Backup before migration
    if [ -f "$QUEUE_DB" ] && [ "$current" -gt 0 ]; then
        backup_database
    fi

    # Run migrations
    if [ "$current" -eq 0 ]; then
        create_initial_schema
        current=1
    fi

    if [ "$current" -eq 1 ] && [ "$CURRENT_VERSION" -ge 2 ]; then
        migrate_v1_to_v2
        current=2
    fi

    # Add future migrations here
    # if [ "$current" -eq 2 ] && [ "$CURRENT_VERSION" -ge 3 ]; then
    #     migrate_v2_to_v3
    #     current=3
    # fi

    # Verify final state
    verify_database || exit 1

    # Optimize
    optimize_database

    # Print stats
    print_stats

    cat <<EOF
${GREEN}=============================================================================
Migration Complete!
=============================================================================${NC}

${BLUE}Schema Version:${NC} v$current
${BLUE}Backup:${NC} $BACKUP_DB

${BLUE}Next Steps:${NC}
  - Start daemon: systemctl --user start agm-daemon
  - Check status: agm daemon status
  - View messages: agm queue list

EOF

    log_success "Migration completed successfully!"
}

# Run main if executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
