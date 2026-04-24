# AGM Operations Runbook

**Version**: 1.0
**Last Updated**: 2026-02-20
**Target Audience**: DevOps, SREs, System Administrators

---

## Table of Contents

- [Overview](#overview)
- [Installation & Deployment](#installation--deployment)
- [Daemon Operations](#daemon-operations)
- [Monitoring & Health Checks](#monitoring--health-checks)
- [Common Operational Tasks](#common-operational-tasks)
- [Incident Response](#incident-response)
- [Backup & Recovery](#backup--recovery)
- [Performance Tuning](#performance-tuning)
- [Troubleshooting Guide](#troubleshooting-guide)
- [Maintenance Procedures](#maintenance-procedures)
- [Security Operations](#security-operations)

---

## Overview

### System Components

AGM (AI/Agent Gateway Manager) consists of:

1. **AGM CLI** (`agm`): Command-line interface for session management
2. **AGM Daemon** (`agm-daemon`): Background message delivery service
3. **Message Queue**: SQLite database at `~/.agm/queue.db`
4. **Session Manifests**: JSON files in `~/.agm/sessions/*/manifest.json`
5. **Claude Hooks**: State management scripts in `~/.claude/hooks/`

### Operational Requirements

| Component | Requirement | Notes |
|-----------|-------------|-------|
| OS | Linux, macOS | Windows via WSL2 |
| Go | 1.24+ | For building from source |
| Tmux | 3.0+ | Required for session management |
| Claude CLI | Latest | Required for Claude agent |
| Disk Space | 1GB+ | For logs, queue, manifests |
| Memory | 512MB+ | Daemon uses ~10MB baseline |
| CPU | 1 core | Daemon uses <1% CPU |

### Service Dependencies

```
AGM CLI ──depends on──> Tmux
                        └> Session Manifests

AGM Daemon ──depends on──> Message Queue (SQLite)
                           └> Tmux
                           └> Session Manifests
                           └> Claude Hooks

Claude Hooks ──depends on──> Claude CLI
                             └> Session Manifests
```

---

## Installation & Deployment

### Fresh Installation

#### Step 1: Install Prerequisites

```bash
# Install tmux
# Ubuntu/Debian
sudo apt-get update && sudo apt-get install -y tmux

# macOS
brew install tmux

# Verify tmux version
tmux -V  # Should be 3.0+
```

#### Step 2: Install AGM

**Option A: Pre-built Binary (Recommended)**

```bash
# Download latest release
curl -L https://github.com/vbonnet/dear-agent/releases/latest/download/agm-linux-amd64 \
  -o /tmp/agm
chmod +x /tmp/agm
sudo mv /tmp/agm /usr/local/bin/

# Verify installation
agm version
```

**Option B: Build from Source**

```bash
# Clone repository
git clone https://github.com/vbonnet/dear-agent.git
cd ai-tools/agm

# Build AGM CLI
cd cmd/agm
go build -o agm
sudo mv agm /usr/local/bin/

# Build AGM Daemon
cd ../agm-daemon
go build -o agm-daemon
sudo mv agm-daemon /usr/local/bin/

# Verify
agm version
agm-daemon --version
```

#### Step 3: Initialize Directories

```bash
# Create AGM directories
mkdir -p ~/.agm/{sessions,logs/daemon,hooks/claude}

# Verify directory structure
tree ~/.agm
# Expected output:
# ~/.agm
# ├── sessions/
# ├── logs/
# │   └── daemon/
# └── hooks/
#     └── claude/
```

#### Step 4: Install Claude Hooks

```bash
# Copy hooks from AGM installation
cp -r /usr/share/agm/hooks/claude/* ~/.claude/hooks/

# Or install manually
agm admin install-hooks --global

# Verify hooks installed
ls -la ~/.claude/hooks/session-start/
ls -la ~/.claude/hooks/message-complete/
ls -la ~/.claude/hooks/compact-start/
ls -la ~/.claude/hooks/compact-complete/
```

#### Step 5: Start Daemon

```bash
# Start daemon (foreground for testing)
agm daemon start --foreground

# Or start daemon (background)
agm daemon start

# Verify daemon running
agm daemon status
# Output: Daemon is running (PID: 12345)
```

### Production Deployment

#### Systemd Service (Linux)

Create `/etc/systemd/system/agm-daemon.service`:

```ini
[Unit]
Description=AGM Message Delivery Daemon
After=network.target

[Service]
Type=simple
User=%i
Environment="HOME=/home/%i"
ExecStart=/usr/local/bin/agm-daemon
Restart=on-failure
RestartSec=10s
StandardOutput=append:/home/%i/.agm/logs/daemon/daemon.log
StandardError=append:/home/%i/.agm/logs/daemon/daemon.log

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
# Enable service for user
sudo systemctl enable agm-daemon@$USER

# Start service
sudo systemctl start agm-daemon@$USER

# Check status
sudo systemctl status agm-daemon@$USER

# View logs
sudo journalctl -u agm-daemon@$USER -f
```

#### Launchd Service (macOS)

Create `~/Library/LaunchAgents/com.agm.daemon.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.agm.daemon</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/agm-daemon</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/Users/USERNAME/.agm/logs/daemon/daemon.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/USERNAME/.agm/logs/daemon/daemon.log</string>
</dict>
</plist>
```

Load service:

```bash
# Load service
launchctl load ~/Library/LaunchAgents/com.agm.daemon.plist

# Verify running
launchctl list | grep agm

# View logs
tail -f ~/.agm/logs/daemon/daemon.log
```

---

## Daemon Operations

### Starting the Daemon

```bash
# Start daemon (background)
agm daemon start

# Start with custom config
agm daemon start --config ~/.agm/daemon-config.yaml

# Start with custom poll interval
agm daemon start --poll-interval 10s

# Start in foreground (for debugging)
agm daemon start --foreground

# Start with verbose logging
agm daemon start --log-level debug
```

### Stopping the Daemon

```bash
# Graceful stop (waits for current deliveries)
agm daemon stop

# Force stop (immediate shutdown)
agm daemon stop --force

# Stop via PID file
kill $(cat ~/.agm/daemon.pid)

# Stop systemd service
sudo systemctl stop agm-daemon@$USER
```

### Restarting the Daemon

```bash
# Restart (stops and starts)
agm daemon restart

# Reload config (SIGHUP, future feature)
agm daemon reload

# Systemd restart
sudo systemctl restart agm-daemon@$USER
```

### Checking Daemon Status

```bash
# Basic status
agm daemon status
# Output: Daemon is running (PID: 12345)

# Detailed status with queue info
agm daemon status --queue
# Output:
#   Daemon is running (PID: 12345)
#   Queue: 5 pending, 123 delivered, 2 failed

# JSON format (for automation)
agm daemon status --format json
# Output: {"running":true,"pid":12345,"queue":{"pending":5,"delivered":123,"failed":2}}

# Check process health
ps aux | grep agm-daemon

# Check daemon uptime
ps -o etime= -p $(cat ~/.agm/daemon.pid)
```

---

## Monitoring & Health Checks

### Health Check Endpoints

AGM provides health check capabilities through CLI commands:

```bash
# System-wide health check
agm doctor

# Check daemon health
agm daemon health

# Check specific session health
agm session health <session-name>

# Check queue health
agm daemon queue-health
```

### Key Metrics to Monitor

| Metric | Command | Threshold | Alert Level |
|--------|---------|-----------|-------------|
| Daemon uptime | `agm daemon status` | >99% | CRITICAL |
| Pending messages | `agm daemon status --queue` | <100 | WARNING |
| Failed messages | `agm daemon status --queue` | <10 | WARNING |
| Queue size | `du -h ~/.agm/queue.db` | <100MB | WARNING |
| Log size | `du -h ~/.agm/logs/` | <1GB | WARNING |
| Active sessions | `agm session list --filter active` | - | INFO |

### Log Monitoring

**Daemon Logs:**

```bash
# Tail daemon logs
tail -f ~/.agm/logs/daemon/daemon.log

# Search for errors
grep ERROR ~/.agm/logs/daemon/daemon.log

# Count errors per hour
grep ERROR ~/.agm/logs/daemon/daemon.log | awk '{print $1" "$2}' | \
  cut -d: -f1 | uniq -c

# Find slow deliveries (>1s)
grep "delivery took" ~/.agm/logs/daemon/daemon.log | \
  awk '$NF > 1000'  # milliseconds
```

**Session Logs:**

```bash
# Check session state transitions
grep "state transition" ~/.agm/sessions/*/manifest.json

# Find sessions stuck in WORKING
agm session list --filter "state:WORKING" --format json | \
  jq -r '.[] | select(.state_updated_at < (now - 3600))'
```

### Alerting Configuration

**Example: Monitor with Prometheus**

Create `/etc/prometheus/agm-exporter.sh`:

```bash
#!/bin/bash
# AGM Metrics Exporter for Prometheus

# Daemon status
DAEMON_RUNNING=$(agm daemon status >/dev/null 2>&1 && echo 1 || echo 0)
echo "agm_daemon_running $DAEMON_RUNNING"

# Queue metrics
QUEUE_JSON=$(agm daemon status --queue --format json)
PENDING=$(echo "$QUEUE_JSON" | jq -r '.queue.pending')
DELIVERED=$(echo "$QUEUE_JSON" | jq -r '.queue.delivered')
FAILED=$(echo "$QUEUE_JSON" | jq -r '.queue.failed')

echo "agm_queue_pending $PENDING"
echo "agm_queue_delivered $DELIVERED"
echo "agm_queue_failed $FAILED"

# Active sessions
ACTIVE=$(agm session list --filter active --format json | jq -r 'length')
echo "agm_sessions_active $ACTIVE"
```

Run with cron:

```cron
*/5 * * * * /etc/prometheus/agm-exporter.sh > /var/lib/prometheus/node-exporter/agm.prom
```

---

## Common Operational Tasks

### Task 1: Create New Session

```bash
# Create session for user
agm new my-session --harness claude-code --detached

# Verify creation
agm session list | grep my-session

# Check session health
agm session health my-session
```

### Task 2: Send Message to Session

```bash
# Send message
agm send target-session "Process data at ~/data/input.csv"

# Verify message queued
agm daemon status --queue

# Monitor delivery
tail -f ~/.agm/logs/daemon/daemon.log | grep target-session
```

### Task 3: Archive Old Sessions

```bash
# List sessions older than 30 days
agm session list --filter "age:>30d" --format simple

# Archive old sessions
for session in $(agm session list --filter "age:>30d" --format simple); do
  agm session archive "$session"
done

# Verify archived count
agm session list --filter archived --format json | jq -r 'length'
```

### Task 4: Clean Message Queue

```bash
# Show queue statistics
agm daemon status --queue --verbose

# Clean delivered messages older than 7 days
agm daemon clean --older-than 7d --status delivered

# Purge failed messages
agm daemon clean --status failed --force

# Vacuum database to reclaim space
agm daemon vacuum
```

### Task 5: Update Session State

```bash
# Get current state
agm session get-state my-session

# Force state to DONE (admin only)
agm admin set-state my-session DONE

# Verify state update
agm session history my-session --limit 5
```

### Task 6: Rotate Logs

```bash
# Check log sizes
du -h ~/.agm/logs/

# Archive old logs
tar -czf ~/.agm/logs/archive-$(date +%Y%m%d).tar.gz \
  ~/.agm/logs/daemon/*.log.*

# Truncate current log (daemon must be stopped)
agm daemon stop
> ~/.agm/logs/daemon/daemon.log
agm daemon start

# Or use logrotate (recommended)
# See "Log Rotation" section below
```

---

## Incident Response

### Incident 1: Daemon Crashed

**Symptoms:**
- `agm daemon status` reports "not running"
- Messages not being delivered

**Response:**

```bash
# 1. Check for stale PID file
if [ -f ~/.agm/daemon.pid ]; then
  PID=$(cat ~/.agm/daemon.pid)
  if ! ps -p $PID > /dev/null; then
    echo "Stale PID file detected"
    rm ~/.agm/daemon.pid
  fi
fi

# 2. Check logs for crash reason
tail -100 ~/.agm/logs/daemon/daemon.log

# 3. Backup queue database (precaution)
cp ~/.agm/queue.db ~/.agm/queue.db.backup-$(date +%Y%m%d-%H%M%S)

# 4. Restart daemon
agm daemon start

# 5. Verify recovery
agm daemon status
agm daemon status --queue
```

**Post-Incident:**
- Review logs for root cause
- File bug report if reproducible
- Update monitoring to detect earlier

### Incident 2: Message Queue Corruption

**Symptoms:**
- Daemon fails to start with "database is locked" or "database corrupt"
- Queue operations fail

**Response:**

```bash
# 1. Stop daemon
agm daemon stop

# 2. Backup corrupted database
mv ~/.agm/queue.db ~/.agm/queue.db.corrupt
mv ~/.agm/queue.db-shm ~/.agm/queue.db-shm.corrupt 2>/dev/null
mv ~/.agm/queue.db-wal ~/.agm/queue.db-wal.corrupt 2>/dev/null

# 3. Attempt recovery with SQLite
sqlite3 ~/.agm/queue.db.corrupt ".recover" | sqlite3 ~/.agm/queue.db

# 4. If recovery fails, restore from backup
if [ $? -ne 0 ]; then
  cp ~/.agm/backups/queue.db.latest ~/.agm/queue.db
fi

# 5. Restart daemon
agm daemon start

# 6. Verify queue integrity
agm daemon status --queue --verbose
```

**Post-Incident:**
- Enable WAL mode for better corruption resistance
- Implement automated queue backups
- Review disk health (may indicate hardware issue)

### Incident 3: Sessions Stuck in WORKING State

**Symptoms:**
- Multiple sessions showing WORKING for >1 hour
- Messages not being delivered despite sessions being idle

**Response:**

```bash
# 1. Identify stuck sessions
agm session list --filter "state:WORKING" --format json | \
  jq -r '.[] | select(.state_updated_at < (now - 3600)) | .name'

# 2. Validate session is actually idle
for session in $(agm session list --filter "state:WORKING" --format simple); do
  # Check tmux session activity
  tmux capture-pane -t "$session" -p | tail -5
done

# 3. Force state to DONE if confirmed idle
for session in <stuck-sessions>; do
  agm admin set-state "$session" DONE
  echo "Reset state for $session"
done

# 4. Verify message delivery resumes
tail -f ~/.agm/logs/daemon/daemon.log | grep "delivered"
```

**Post-Incident:**
- Review hook execution logs
- Check for hook script failures
- Implement state watchdog (auto-reset after timeout)

### Incident 4: High Memory Usage

**Symptoms:**
- Daemon process using >500MB RAM
- System experiencing memory pressure

**Response:**

```bash
# 1. Check daemon memory usage
ps aux | grep agm-daemon

# 2. Check queue size
sqlite3 ~/.agm/queue.db "SELECT COUNT(*) FROM message_queue;"
du -h ~/.agm/queue.db

# 3. Check for message leaks (messages stuck in pending)
agm daemon status --queue --verbose

# 4. Clean old messages
agm daemon clean --older-than 1d --status delivered

# 5. Restart daemon to reclaim memory
agm daemon restart

# 6. Monitor memory after restart
watch -n 5 'ps aux | grep agm-daemon'
```

**Post-Incident:**
- Implement automatic queue cleaning (daily cron)
- Set queue size limits
- Add memory usage alerts

---

## Backup & Recovery

### Backup Strategy

**What to Backup:**

1. **Message Queue**: `~/.agm/queue.db`
2. **Session Manifests**: `~/.agm/sessions/*/manifest.json`
3. **Configuration**: `~/.config/agm/config.yaml`
4. **Logs**: `~/.agm/logs/` (optional, for forensics)

**Backup Schedule:**

- **Queue**: Hourly (critical data)
- **Manifests**: Daily (low churn)
- **Config**: On change (manual trigger)
- **Logs**: Weekly (retention only)

### Automated Backup Script

Create `~/bin/agm-backup.sh`:

```bash
#!/bin/bash
set -euo pipefail

BACKUP_DIR=~/.agm/backups/$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP_DIR"

# Backup queue database
if [ -f ~/.agm/queue.db ]; then
  sqlite3 ~/.agm/queue.db ".backup '$BACKUP_DIR/queue.db'"
  echo "Backed up queue.db"
fi

# Backup session manifests
if [ -d ~/.agm/sessions ]; then
  tar -czf "$BACKUP_DIR/sessions.tar.gz" ~/.agm/sessions/
  echo "Backed up sessions"
fi

# Backup config
if [ -f ~/.config/agm/config.yaml ]; then
  cp ~/.config/agm/config.yaml "$BACKUP_DIR/"
  echo "Backed up config"
fi

# Cleanup old backups (keep last 7 days)
find ~/.agm/backups/ -type d -mtime +7 -exec rm -rf {} +

echo "Backup complete: $BACKUP_DIR"
```

Add to cron:

```cron
# Hourly queue backup
0 * * * * ~/bin/agm-backup.sh

# Daily manifest backup
0 2 * * * ~/bin/agm-backup.sh
```

### Recovery Procedures

**Scenario 1: Restore Queue Database**

```bash
# 1. Stop daemon
agm daemon stop

# 2. Find latest backup
LATEST=$(ls -t ~/.agm/backups/*/queue.db | head -1)
echo "Restoring from: $LATEST"

# 3. Backup current (corrupted) database
mv ~/.agm/queue.db ~/.agm/queue.db.broken

# 4. Restore from backup
cp "$LATEST" ~/.agm/queue.db

# 5. Restart daemon
agm daemon start

# 6. Verify queue contents
agm daemon status --queue --verbose
```

**Scenario 2: Restore Session Manifests**

```bash
# 1. Find latest backup
LATEST=$(ls -t ~/.agm/backups/*/sessions.tar.gz | head -1)

# 2. Backup current manifests
mv ~/.agm/sessions ~/.agm/sessions.backup-$(date +%Y%m%d-%H%M%S)

# 3. Extract backup
mkdir -p ~/.agm/sessions
tar -xzf "$LATEST" -C ~/.agm/

# 4. Verify manifests
agm session list --format table
```

**Scenario 3: Disaster Recovery (Complete Reinstall)**

```bash
# 1. Stop all AGM processes
agm daemon stop
pkill -f agm

# 2. Restore from backup
BACKUP_DIR=~/.agm/backups/<latest-backup>
cp "$BACKUP_DIR/queue.db" ~/.agm/
tar -xzf "$BACKUP_DIR/sessions.tar.gz" -C ~/.agm/
cp "$BACKUP_DIR/config.yaml" ~/.config/agm/

# 3. Reinstall AGM binaries
curl -L <latest-release-url> -o /tmp/agm
sudo mv /tmp/agm /usr/local/bin/
chmod +x /usr/local/bin/agm

# 4. Reinstall hooks
agm admin install-hooks --global

# 5. Restart daemon
agm daemon start

# 6. Verify system health
agm doctor
```

---

## Performance Tuning

### Daemon Performance

**Optimize Poll Interval:**

```bash
# Default: 30s (good for most use cases)
agm daemon start

# Low latency: 5s poll (trades CPU for latency)
agm daemon start --poll-interval 5s

# Batch processing: 2m poll (lower CPU usage)
agm daemon start --poll-interval 2m

# Calculate optimal interval:
# Target latency = desired message delivery time
# Poll interval = target latency / 2 (for 50% average case)
```

**Queue Optimization:**

```bash
# Enable WAL mode (better concurrent access)
sqlite3 ~/.agm/queue.db "PRAGMA journal_mode=WAL;"

# Set cache size (default 2MB, increase for large queues)
sqlite3 ~/.agm/queue.db "PRAGMA cache_size=-10000;"  # 10MB

# Analyze database for query optimization
sqlite3 ~/.agm/queue.db "ANALYZE;"

# Vacuum database monthly
agm daemon vacuum
```

**Index Optimization:**

```sql
-- Create composite index for common queries
CREATE INDEX IF NOT EXISTS idx_queue_status_priority
ON message_queue(status, priority DESC, created_at);

-- Create index for cleanup queries
CREATE INDEX IF NOT EXISTS idx_queue_cleanup
ON message_queue(status, delivered_at)
WHERE status IN ('delivered', 'failed');
```

### Session Performance

**Reduce Manifest I/O:**

```bash
# Cache session list (update every 5 minutes)
*/5 * * * * agm session list --format json > /tmp/agm-sessions-cache.json
```

**Optimize Hook Execution:**

```bash
# Profile hook execution time
time ~/.claude/hooks/session-start/agm-state-ready

# Disable slow hooks (edit ~/.claude/hooks-config.yaml)
disabled_hooks:
  - slow-analytics-hook
  - heavy-logging-hook
```

### Tmux Performance

**Optimize Tmux Configuration:**

Add to `~/.tmux.conf`:

```conf
# Increase history limit (reduce scrollback searches)
set-option -g history-limit 50000

# Faster command sequences
set -s escape-time 0

# Increase message display time
set -g display-time 4000

# Aggressive resize
setw -g aggressive-resize on
```

---

## Troubleshooting Guide

### Daemon Won't Start

**Check 1: PID File Conflict**

```bash
# Remove stale PID file
rm ~/.agm/daemon.pid

# Retry start
agm daemon start
```

**Check 2: Port/Socket Conflict**

```bash
# Check for zombie processes
ps aux | grep agm-daemon
kill -9 <zombie-pid>

# Check Unix socket permissions
ls -la /tmp/agm-daemon.sock
rm /tmp/agm-daemon.sock  # If stale
```

**Check 3: Database Lock**

```bash
# Check database lock
lsof ~/.agm/queue.db

# Force unlock (risky)
fuser -k ~/.agm/queue.db

# Or restore from backup
mv ~/.agm/queue.db ~/.agm/queue.db.locked
cp ~/.agm/backups/latest/queue.db ~/.agm/
```

### Messages Not Delivered

**Check 1: Session State**

```bash
# Verify session is DONE
agm session get-state target-session

# Check state history
agm session history target-session --limit 10

# Force DONE if stuck
agm admin set-state target-session DONE
```

**Check 2: Daemon Running**

```bash
# Verify daemon is running
agm daemon status

# Check daemon logs for errors
tail -50 ~/.agm/logs/daemon/daemon.log | grep ERROR
```

**Check 3: Queue Status**

```bash
# Check message is in queue
agm daemon status --queue --verbose

# Query specific message
agm daemon message-info <message-id>
```

### High Resource Usage

**CPU Usage:**

```bash
# Check daemon CPU usage
top -p $(cat ~/.agm/daemon.pid)

# Profile daemon (if built with profiling)
agm daemon profile --duration 30s

# Reduce poll frequency
agm daemon restart --poll-interval 1m
```

**Disk Usage:**

```bash
# Check queue size
du -h ~/.agm/queue.db

# Check log size
du -h ~/.agm/logs/

# Clean old data
agm daemon clean --older-than 7d
```

**Memory Usage:**

```bash
# Check daemon memory
ps aux | grep agm-daemon | awk '{print $6}'

# Check for memory leaks
valgrind --leak-check=full agm-daemon
```

---

## Maintenance Procedures

### Daily Maintenance

```bash
#!/bin/bash
# Daily maintenance script

# Check daemon health
agm daemon health || agm daemon restart

# Clean delivered messages
agm daemon clean --older-than 1d --status delivered

# Archive old sessions
agm session list --filter "age:>30d" --format simple | \
  xargs -I {} agm session archive {}

# Check queue size
QUEUE_SIZE=$(sqlite3 ~/.agm/queue.db "SELECT COUNT(*) FROM message_queue;")
if [ "$QUEUE_SIZE" -gt 1000 ]; then
  echo "WARNING: Queue size exceeds 1000 messages"
fi
```

### Weekly Maintenance

```bash
#!/bin/bash
# Weekly maintenance script

# Backup queue and manifests
~/bin/agm-backup.sh

# Vacuum database
agm daemon vacuum

# Rotate logs
logrotate ~/.agm/logrotate.conf

# Generate health report
agm doctor --report > ~/.agm/reports/health-$(date +%Y%m%d).txt
```

### Monthly Maintenance

```bash
#!/bin/bash
# Monthly maintenance script

# Analyze database performance
sqlite3 ~/.agm/queue.db "ANALYZE;"

# Check for database corruption
sqlite3 ~/.agm/queue.db "PRAGMA integrity_check;"

# Archive old logs
tar -czf ~/.agm/logs/archive-$(date +%Y%m).tar.gz ~/.agm/logs/daemon/*.log.*
rm ~/.agm/logs/daemon/*.log.*

# Review failed messages
agm daemon status --queue --filter failed --verbose
```

### Log Rotation Configuration

Create `~/.agm/logrotate.conf`:

```conf
/home/USERNAME/.agm/logs/daemon/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0644 USERNAME USERNAME
    postrotate
        # Signal daemon to reopen log file
        kill -HUP $(cat /home/USERNAME/.agm/daemon.pid) 2>/dev/null || true
    endscript
}
```

---

## Security Operations

### Security Checklist

- [ ] API keys stored in environment variables (not config files)
- [ ] Queue database permissions set to 0600
- [ ] Daemon PID file permissions set to 0644
- [ ] Session manifest permissions set to 0644
- [ ] Hook scripts owned by user (not world-writable)
- [ ] Logs rotated and archived securely
- [ ] Regular security updates applied

### Access Control

```bash
# Set restrictive permissions
chmod 600 ~/.agm/queue.db
chmod 644 ~/.agm/daemon.pid
chmod 644 ~/.agm/sessions/*/manifest.json
chmod 700 ~/.agm/hooks/

# Verify permissions
find ~/.agm -type f -perm /go+w  # Should be empty
```

### Audit Logging

```bash
# Enable audit logging (future feature)
agm daemon start --audit-log ~/.agm/logs/audit.log

# Review audit log
grep "admin set-state" ~/.agm/logs/audit.log
grep "queue clean" ~/.agm/logs/audit.log
```

### Secure Message Handling

```bash
# Never send secrets in messages
# BAD: agm send session "API_KEY=sk-abc123"
# GOOD: agm send session "Use API key from ~/.secrets/api-key"

# Sanitize logs
sed -i 's/API_KEY=[^ ]*/API_KEY=***REDACTED***/g' ~/.agm/logs/daemon/daemon.log
```

---

## Appendix

### Useful Scripts

**Check All Sessions Health:**

```bash
#!/bin/bash
for session in $(agm session list --format simple); do
  echo "Checking $session..."
  agm session health "$session" || echo "  UNHEALTHY: $session"
done
```

**Monitor Queue Growth:**

```bash
#!/bin/bash
while true; do
  PENDING=$(agm daemon status --queue --format json | jq -r '.queue.pending')
  echo "$(date): $PENDING pending messages"
  sleep 60
done
```

**Emergency Stop All Sessions:**

```bash
#!/bin/bash
# Stop daemon
agm daemon stop

# Kill all tmux sessions managed by AGM
for session in $(agm session list --format simple); do
  tmux kill-session -t "$session" 2>/dev/null
done
```

### Configuration Reference

See [AGM Command Reference](AGM-COMMAND-REFERENCE.md) for full configuration options.

---

**Maintained by**: AGM Operations Team
**On-Call Escalation**: ops-agm@example.com
**Documentation Updates**: Submit PR to docs/OPERATIONS_RUNBOOK.md
