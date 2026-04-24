# Archive Dolt Migration - Testing Runbook

## Quick Start

This runbook provides step-by-step instructions to verify the Dolt-based archive fix.

## Prerequisites

### 1. Environment Setup

```bash
# Set workspace
export WORKSPACE=oss  # or your workspace name

# Verify Dolt server is running
dolt version
# Expected: dolt version 1.x.x

# Check if Dolt SQL server is running
ps aux | grep "dolt sql-server"
# Should show running process on port 3307
```

### 2. Start Dolt Server (if not running)

```bash
cd ~/src/ws/${WORKSPACE}/.dolt/dolt-db
dolt sql-server -H 127.0.0.1 -P 3307 &

# Verify connection
dolt sql -q "SHOW TABLES"
# Should show agm_sessions, agm_messages, etc.
```

### 3. Build AGM with Fix

```bash
cd main/agm
go build -o ~/go/bin/agm ./cmd/agm

# Verify build
agm version
```

## Test Scenarios

### Scenario 1: Archive by Session ID

**Purpose**: Verify ResolveIdentifier works with session ID

```bash
# Step 1: List sessions and get a session ID
agm session list --json | jq -r '.[0] | {id: .session_id, name: .name, status: .status}'

# Example output:
# {
#   "id": "session-abc123def456",
#   "name": "my-session",
#   "status": "STOPPED"
# }

# Step 2: Archive by session ID
agm session archive session-abc123def456

# Expected output:
# ✓ Archived session: session-abc123def456
#
# The session is now hidden from 'agm session list'.
# Use 'agm session list --all' to see archived sessions.
#
# To restore: agm session unarchive session-abc123def456

# Step 3: Verify not in default list
agm session list | grep session-abc123def456
# Should return nothing (empty)

# Step 4: Verify in --all list
agm session list --all --json | jq -r '.[] | select(.session_id=="session-abc123def456") | .lifecycle'
# Expected: "archived"
```

**Success Criteria**:
- ✅ Archive command succeeds
- ✅ Session not in default list
- ✅ Session in --all list with lifecycle=archived

---

### Scenario 2: Archive by Tmux Session Name

**Purpose**: Verify ResolveIdentifier works with tmux name

```bash
# Step 1: List sessions and get tmux name
agm session list --json | jq -r '.[0] | {tmux: .tmux.session_name, name: .name, status: .status}'

# Example output:
# {
#   "tmux": "claude-5",
#   "name": "research-session",
#   "status": "STOPPED"
# }

# Step 2: Archive by tmux name
agm session archive claude-5

# Expected output:
# ✓ Archived session: claude-5

# Step 3: Verify in Dolt directly
dolt sql -q "SELECT name, tmux_session_name, status FROM agm_sessions WHERE tmux_session_name='claude-5'"

# Expected output:
# +------------------+-------------------+----------+
# | name             | tmux_session_name | status   |
# +------------------+-------------------+----------+
# | research-session | claude-5          | archived |
# +------------------+-------------------+----------+
```

**Success Criteria**:
- ✅ Archive command succeeds
- ✅ Dolt shows status=archived
- ✅ Session not in default list

---

### Scenario 3: Archive by Manifest Name

**Purpose**: Verify ResolveIdentifier works with manifest name field

```bash
# Step 1: List sessions and get manifest name
agm session list --json | jq -r '.[0] | {name: .name, tmux: .tmux.session_name, status: .status}'

# Example output:
# {
#   "name": "research-gemini",
#   "tmux": "gemini-research-1",
#   "status": "STOPPED"
# }

# Step 2: Archive by manifest name
agm session archive research-gemini

# Expected output:
# ✓ Archived session: research-gemini

# Step 3: Verify
agm session list --all | grep research-gemini
# Should show in archived list
```

**Success Criteria**:
- ✅ Archive command succeeds using manifest name
- ✅ Session appears in --all list as archived

---

### Scenario 4: Error Handling - Non-Existent Session

**Purpose**: Verify clear error messages for invalid sessions

```bash
# Attempt to archive non-existent session
agm session archive this-session-does-not-exist-12345

# Expected output:
# ✗ Session not found: this-session-does-not-exist-12345
#   Storage: Dolt storage
#
# Session 'this-session-does-not-exist-12345' not found.
```

**Success Criteria**:
- ✅ Command fails gracefully
- ✅ Error message is clear and helpful
- ✅ Mentions "Dolt storage" as source

---

### Scenario 5: Cannot Re-Archive

**Purpose**: Verify archived sessions are excluded from ResolveIdentifier

```bash
# Step 1: Archive a session
agm session list --json | jq -r '.[0].name'
# Example: "test-session"

agm session archive test-session
# Expected: ✓ Archived session: test-session

# Step 2: Try to archive again
agm session archive test-session

# Expected output:
# ✗ Session not found: test-session
#   Storage: Dolt storage

# This is CORRECT behavior - ResolveIdentifier excludes archived sessions
```

**Success Criteria**:
- ✅ First archive succeeds
- ✅ Second archive fails with "session not found"
- ✅ Session remains archived (check with --all)

---

### Scenario 6: Bulk Archive

**Purpose**: Verify archiving multiple STOPPED sessions

```bash
# Step 1: List all STOPPED sessions
agm session list --json | jq -r '.[] | select(.status=="STOPPED") | .name'

# Example output:
# hook-enforcement
# agm-send-interrupt
# agm-opencode
# research-cont

# Step 2: Bulk archive
for session in hook-enforcement agm-send-interrupt agm-opencode research-cont; do
    echo "Archiving: $session"
    agm session archive "$session" || echo "  Failed: $session"
done

# Step 3: Verify no STOPPED sessions remain
agm session list --json | jq -r '.[] | select(.status=="STOPPED") | .name'
# Expected: empty (no output)

# Step 4: Verify archived count
agm session list --all --json | jq -r '.[] | select(.lifecycle=="archived") | .name' | wc -l
# Should show count of archived sessions
```

**Success Criteria**:
- ✅ All specified sessions archived successfully
- ✅ No STOPPED sessions in default list
- ✅ Archived sessions appear in --all list

---

## Regression Testing

### Regression: Original Bug Fix

**Bug**: STOPPED sessions existed in Dolt but couldn't be archived

```bash
# Step 1: Verify session exists in Dolt
dolt sql -q "SELECT id, name, status FROM agm_sessions WHERE status='active' LIMIT 1"

# Step 2: Get session ID from output
# Example: session-xyz789

# Step 3: Ensure tmux session is stopped (killed)
# (AGM requires sessions to be stopped before archiving)

# Step 4: Archive using session ID from Dolt
agm session archive session-xyz789

# Expected: SUCCESS (previously failed with "session not found")
```

**Success Criteria**:
- ✅ Session in Dolt can be archived
- ✅ No "session not found" error for valid Dolt sessions

---

## Verification Queries

### Direct Dolt Queries for Debugging

```bash
# 1. List all sessions in Dolt
dolt sql -q "SELECT id, name, tmux_session_name, status FROM agm_sessions WHERE workspace='${WORKSPACE}'"

# 2. Count active vs archived
dolt sql -q "SELECT status, COUNT(*) as count FROM agm_sessions WHERE workspace='${WORKSPACE}' GROUP BY status"

# 3. Find specific session by name
dolt sql -q "SELECT * FROM agm_sessions WHERE name='research-gemini' AND workspace='${WORKSPACE}'"

# 4. Find by tmux name
dolt sql -q "SELECT * FROM agm_sessions WHERE tmux_session_name='claude-5' AND workspace='${WORKSPACE}'"

# 5. Recently archived sessions
dolt sql -q "SELECT id, name, status, updated_at FROM agm_sessions WHERE status='archived' ORDER BY updated_at DESC LIMIT 10"
```

---

## Rollback Plan

If the fix causes issues:

```bash
# 1. Stop using new binary
mv ~/go/bin/agm ~/go/bin/agm.new

# 2. Restore old binary (if backup exists)
mv ~/go/bin/agm.old ~/go/bin/agm

# 3. Or rebuild from previous commit
cd main/agm
git checkout HEAD~1 cmd/agm/archive.go internal/dolt/sessions.go
go build -o ~/go/bin/agm ./cmd/agm
```

---

## Sign-Off Checklist

Before declaring the fix complete:

- [ ] All 6 scenarios pass
- [ ] Regression test passes (original bug fixed)
- [ ] Unit tests pass: `go test ./internal/dolt/... -v`
- [ ] Integration tests pass: `DOLT_TEST_INTEGRATION=1 go test ./test/integration/lifecycle/... -tags=integration`
- [ ] No compilation errors
- [ ] No lint warnings: `golangci-lint run ./...`
- [ ] Documentation updated
- [ ] Manual verification by QA or peer

---

## Quick Reference

### Environment Variables
```bash
export WORKSPACE=oss           # Required
export DOLT_PORT=3307          # Default for OSS
export DOLT_TEST_INTEGRATION=1 # Enable integration tests
```

### Common Commands
```bash
# List active sessions
agm session list

# List all sessions (including archived)
agm session list --all

# Archive by any identifier (ID, tmux name, or manifest name)
agm session archive <identifier>

# JSON output for scripting
agm session list --json | jq .

# Check Dolt directly
dolt sql -q "SELECT * FROM agm_sessions LIMIT 5"
```

### Troubleshooting
```bash
# Check Dolt connection
dolt sql -q "SELECT 1"

# Verify workspace
echo $WORKSPACE

# Check AGM version
agm version

# View recent logs (if using systemd)
journalctl -u dolt-sql-server -f
```

---

## Contact

For issues or questions:
- File issue in AGM repository
- Check documentation: `docs/ARCHIVE-DOLT-MIGRATION.md`
- Consult team Slack channel
