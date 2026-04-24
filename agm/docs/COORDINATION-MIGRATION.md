  # AGM Multi-Session Coordination - Migration Guide

**Phase 3 Task 3.1: Installation & Migration**
**Bead**: oss-5wpp
**Date**: 2026-02-20

## Overview

This guide walks you through migrating existing AGM installations to support multi-session coordination with message queue and daemon-based delivery.

## Prerequisites

- AGM v3.0+ installed
- Claude Code CLI installed
- Tmux 2.6+
- SQLite 3.x
- Go 1.21+ (for building daemon)
- Python 3.8+ (for migration scripts)

## Migration Timeline

**Estimated Duration**: 15-30 minutes
**Downtime**: None (zero-downtime migration)
**Rollback Time**: 5 minutes

## Pre-Migration Checklist

### 1. Verify Current Installation

```bash
# Check AGM version
agm --version

# Check running sessions
agm list --all

# Check for orphaned sessions
agm admin find-orphans

# Run health check
agm doctor --validate
```

### 2. Review System Status

```bash
# Check tmux sessions
tmux list-sessions

# Check for stale locks
find ~/.agm -name "*.lock" -mtime +1

# Check disk space (need ~100MB for queue)
df -h ~/.agm
```

### 3. Backup Current State

```bash
# Automatic backup (recommended)
scripts/backup-agm.sh

# Manual backup
mkdir -p ~/agm-backup-$(date +%Y%m%d)
cp -r ~/.agm ~/agm-backup-$(date +%Y%m%d)/
cp -r ~/.claude/hooks ~/agm-backup-$(date +%Y%m%d)/claude-hooks
```

## Migration Steps

### Option A: Automated Migration (Recommended)

```bash
cd ./agm

# Run automated installation
./scripts/install-agm-coordination.sh
```

The script performs:
1. ✅ Dependency checks
2. ✅ Directory creation
3. ✅ Automatic backup
4. ✅ Hook installation
5. ✅ Manifest migration
6. ✅ Queue initialization
7. ✅ Systemd service setup
8. ✅ Verification

### Option B: Manual Migration

#### Step 1: Build AGM Daemon

```bash
cd ./agm

# Build daemon binary
go build -o agm-daemon cmd/agm-daemon/*.go

# Install to user bin
cp agm-daemon ~/bin/
chmod +x ~/bin/agm-daemon
```

#### Step 2: Install Claude Hooks

```bash
# Install hooks for state tracking
agm admin install-hooks

# Verify hooks installed
ls -la ~/.claude/hooks/
# Should show:
#   posttool-agm-state-notify
#   session-start/agm-state-ready
```

#### Step 3: Migrate Session Manifests

```bash
# Add state fields to existing sessions
find ~/.agm/sessions -name "manifest.json" -exec \
  python3 -c "
import json, sys
with open(sys.argv[1], 'r+') as f:
    data = json.load(f)
    if 'state' not in data:
        data['state'] = 'OFFLINE'
        data['state_updated_at'] = '$(date -Iseconds)'
        data['state_updated_by'] = 'migration'
        f.seek(0)
        json.dump(data, f, indent=2)
        f.truncate()
" {} \;
```

#### Step 4: Initialize Message Queue

```bash
# Create queue directory
mkdir -p ~/.agm/queue

# Queue database will be created automatically on first use
```

#### Step 5: Install Systemd Service

```bash
# Create service file
mkdir -p ~/.config/systemd/user

cat > ~/.config/systemd/user/agm-daemon.service <<'EOF'
[Unit]
Description=AGM Daemon - Multi-Session Message Delivery
Documentation=https://github.com/vbonnet/dear-agent/tree/main/agm
After=default.target

[Service]
Type=simple
ExecStart=/home/$USER/bin/agm-daemon
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

# Enable and start service
systemctl --user daemon-reload
systemctl --user enable agm-daemon.service
systemctl --user start agm-daemon.service
```

#### Step 6: Enable User Lingering

```bash
# Allow sessions to persist after logout
loginctl enable-linger $USER

# Verify lingering enabled
loginctl show-user $USER | grep Linger
# Should show: Linger=yes
```

## Post-Migration Verification

### 1. Verify Daemon Status

```bash
# Check daemon running
systemctl --user status agm-daemon

# View daemon logs
journalctl --user -u agm-daemon -f

# Check AGM status
agm daemon status
```

**Expected Output**:
```
AGM Daemon Status
═══════════════════════════════════════════════════════════════

Session Status:
┌────────────────────┬───────────┬──────────┬────────┬──────────┐
│ Session            │ State     │ Queued   │ Failed │ Updated  │
├────────────────────┼───────────┼──────────┼────────┼──────────┤
│ (no sessions)      │           │          │        │          │
└────────────────────┴───────────┴──────────┴────────┴──────────┘

Queue Summary:
  Pending: 0 messages
  Failed:  0 messages
  Total:   0 messages

Daemon: Running (PID 12345)
Poll Interval: 30s
Last Poll: 5s ago
Next Poll: in 25s
```

### 2. Test Message Delivery

```bash
# Create two test sessions
agm new test-sender
agm new test-receiver

# Send test message
agm send test-sender test-receiver "Hello from coordination!"

# Check queue
agm queue list

# Verify delivery (within 30 seconds)
# Check receiver session for message
```

### 3. Verify Hooks Working

```bash
# Start a Claude session
agm resume test-session

# In Claude, run a tool (e.g., Read a file)
# Check state was updated
agm session state get test-session
# Should show: WORKING or DONE
```

### 4. Check Health

```bash
# Run full health check
agm doctor --validate

# Should pass all checks:
# ✓ Claude installation
# ✓ Tmux installed
# ✓ User lingering enabled
# ✓ Hooks installed
# ✓ Daemon running
# ✓ Queue database accessible
```

## Migration for Existing Sessions

### Zero-Downtime Migration

Existing sessions can continue running during migration:

1. **Active Sessions**: Continue working, state tracked after next tool use
2. **Stopped Sessions**: Migrated to v3 manifest format automatically
3. **Archived Sessions**: No migration needed (can be migrated on unarchive)

### Session State Initialization

After migration, session states are initialized as:

- **Running sessions**: `OFFLINE` → transitions to `DONE` on next Claude start
- **Stopped sessions**: `OFFLINE` (correct state)
- **First tool use**: Hook sets state to `WORKING`
- **Tool completion**: Hook sets state to `DONE`

## Database Migration

The message queue uses SQLite with schema versioning:

### Initial Schema (v1)

Created automatically on first queue operation:

```sql
CREATE TABLE message_queue (
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

CREATE INDEX idx_pending ON message_queue(to_session, status, created_at)
    WHERE status = 'pending';
```

### Schema Versioning

Future schema updates use migrations:

```bash
# Check current schema version
sqlite3 ~/.agm/queue.db "PRAGMA user_version;"

# Migrations are applied automatically by daemon
# Manual migration (if needed):
agm admin migrate-queue
```

## Backup and Restore Procedures

### Creating Backups

```bash
# Full backup script
scripts/backup-agm.sh

# Manual backup
BACKUP_DIR=~/agm-backup-$(date +%Y%m%d_%H%M%S)
mkdir -p "$BACKUP_DIR"

# Backup configuration
cp ~/.agm/config.yaml "$BACKUP_DIR/config.yaml"

# Backup sessions
cp -r ~/.agm/sessions "$BACKUP_DIR/sessions"

# Backup queue database
sqlite3 ~/.agm/queue.db ".backup '$BACKUP_DIR/queue.db'"

# Backup hooks
cp -r ~/.claude/hooks "$BACKUP_DIR/claude-hooks"
```

### Restoring from Backup

```bash
# Stop daemon first
systemctl --user stop agm-daemon

# Restore configuration
cp "$BACKUP_DIR/config.yaml" ~/.agm/config.yaml

# Restore sessions
rm -rf ~/.agm/sessions
cp -r "$BACKUP_DIR/sessions" ~/.agm/sessions

# Restore queue
sqlite3 ~/.agm/queue.db ".restore '$BACKUP_DIR/queue.db'"

# Restore hooks
cp -r "$BACKUP_DIR/claude-hooks"/* ~/.claude/hooks/

# Restart daemon
systemctl --user start agm-daemon
```

## Rollback Procedure

If you need to revert to pre-coordination state:

### Automated Rollback

```bash
# Rollback using script
scripts/rollback-agm-coordination.sh ~/.agm/backups/20260220_143000
```

### Manual Rollback

```bash
# 1. Stop daemon
systemctl --user stop agm-daemon
systemctl --user disable agm-daemon

# 2. Remove hooks
rm ~/.claude/hooks/posttool-agm-state-notify
rm ~/.claude/hooks/session-start/agm-state-ready

# 3. Restore session manifests from backup
cp -r ~/agm-backup-20260220/sessions ~/.agm/

# 4. Remove queue database
rm ~/.agm/queue.db

# 5. Verify rollback
agm doctor
```

## Troubleshooting

### Daemon Won't Start

**Symptom**: `systemctl --user status agm-daemon` shows failed

**Solution**:
```bash
# Check logs
journalctl --user -u agm-daemon -n 50

# Common issues:
# - Binary not found: verify ~/bin/agm-daemon exists
# - Permission denied: chmod +x ~/bin/agm-daemon
# - Queue DB locked: rm ~/.agm/daemon.pid
```

### Hooks Not Firing

**Symptom**: Session state always shows `OFFLINE`

**Solution**:
```bash
# Verify hooks installed
ls -la ~/.claude/hooks/

# Check hook permissions
chmod +x ~/.claude/hooks/posttool-agm-state-notify
chmod +x ~/.claude/hooks/session-start/agm-state-ready

# Test hook manually
~/.claude/hooks/posttool-agm-state-notify test-session
```

### Messages Not Delivered

**Symptom**: `agm queue list` shows pending messages stuck

**Solution**:
```bash
# Check daemon is polling
journalctl --user -u agm-daemon -f

# Check session state
agm session state get target-session
# If stuck in WORKING, manually set to DONE:
agm session state set target-session DONE

# Check tmux session exists
tmux list-sessions | grep target-session
```

### Manifest Migration Failed

**Symptom**: `agm doctor` shows manifest errors

**Solution**:
```bash
# Validate manifest JSON
jq . ~/.agm/sessions/session-name/manifest.json

# Restore from backup
cp ~/agm-backup-20260220/sessions/session-name/manifest.json \
   ~/.agm/sessions/session-name/manifest.json

# Re-run migration
scripts/install-agm-coordination.sh
```

## Performance Tuning

### Queue Database Optimization

```bash
# Enable WAL mode (default)
sqlite3 ~/.agm/queue.db "PRAGMA journal_mode=WAL;"

# Optimize database
sqlite3 ~/.agm/queue.db "VACUUM;"

# Check database size
du -h ~/.agm/queue.db
```

### Daemon Polling Interval

Default: 30 seconds (configurable in code)

```go
// internal/daemon/daemon.go
const PollInterval = 30 * time.Second  // Adjust as needed
```

Rebuild daemon after changes:
```bash
go build -o ~/bin/agm-daemon cmd/agm-daemon/*.go
systemctl --user restart agm-daemon
```

### Log Rotation

```bash
# Configure journald log rotation
mkdir -p ~/.config/systemd/user/agm-daemon.service.d
cat > ~/.config/systemd/user/agm-daemon.service.d/logging.conf <<'EOF'
[Service]
LogLevelMax=info
EOF

systemctl --user daemon-reload
systemctl --user restart agm-daemon
```

## Security Considerations

### File Permissions

```bash
# Verify secure permissions
chmod 700 ~/.agm
chmod 600 ~/.agm/queue.db
chmod 644 ~/.agm/config.yaml
chmod 755 ~/.claude/hooks/*
```

### Queue Database Security

- **Location**: `~/.agm/queue.db` (user-only access)
- **Encryption**: Use filesystem-level encryption (LUKS, FileVault)
- **Sensitive data**: Messages may contain sensitive prompts

### Daemon Security

- **Runs as user**: No elevated privileges
- **Systemd hardening**: `NoNewPrivileges`, `PrivateTmp`
- **Resource limits**: Memory (256MB), CPU (50%)

## Migration Checklist

- [ ] Backup created and verified
- [ ] Dependencies installed (tmux, sqlite3, go)
- [ ] AGM daemon built and installed
- [ ] Hooks installed in `~/.claude/hooks/`
- [ ] Session manifests migrated to v3 format
- [ ] Queue database initialized
- [ ] Systemd service enabled and started
- [ ] User lingering enabled
- [ ] Daemon status verified (`agm daemon status`)
- [ ] Test message sent and delivered
- [ ] Hooks verified (state transitions working)
- [ ] Health check passed (`agm doctor --validate`)
- [ ] Rollback procedure tested (optional)
- [ ] Documentation updated

## Support and Resources

### Documentation

- **Architecture**: `cmd/agm-daemon/ARCHITECTURE.md`
- **Daemon Spec**: `cmd/agm-daemon/SPEC.md`
- **ADR-006**: Message Queue Architecture
- **ADR-007**: Hook-Based State Detection
- **ADR-008**: Status Aggregation

### Getting Help

1. **Check logs**: `journalctl --user -u agm-daemon -f`
2. **Run diagnostics**: `agm doctor --validate --json`
3. **Review queue**: `agm queue list --verbose`
4. **Inspect database**: `sqlite3 ~/.agm/queue.db ".schema"`

### Reporting Issues

When reporting migration issues, include:

```bash
# Collect diagnostic information
cat > ~/agm-migration-report.txt <<'EOF'
=== AGM Migration Diagnostics ===
Date: $(date)
AGM Version: $(agm --version)
Tmux Version: $(tmux -V)
SQLite Version: $(sqlite3 --version)

=== Daemon Status ===
$(systemctl --user status agm-daemon)

=== Queue Status ===
$(agm daemon status 2>&1)

=== Hooks ===
$(ls -la ~/.claude/hooks/)

=== Recent Logs ===
$(journalctl --user -u agm-daemon -n 100)
EOF
```

---

**Version**: 1.0
**Last Updated**: 2026-02-20
**Phase**: 3 - Deployment & Polish
**Task**: 3.1 - Installation & Migration
**Bead**: oss-5wpp
