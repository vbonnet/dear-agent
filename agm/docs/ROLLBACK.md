# AGM Temporal Backend Rollback Procedure

**Last Updated**: 2026-02-15
**Status**: Production-Ready
**Related**: Backend Implementation (BACKEND_IMPLEMENTATION.md), Temporal Integration

---

## Overview

This document provides step-by-step procedures for rolling back from the Temporal backend to the tmux backend when issues arise with Temporal integration.

**Purpose**: Ensure business continuity by providing a safe rollback path when Temporal backend encounters problems.

**When to use**: See [Rollback Scenarios](#rollback-scenarios) section.

---

## Table of Contents

1. [Rollback Scenarios](#rollback-scenarios)
2. [Pre-Rollback Checklist](#pre-rollback-checklist)
3. [Rollback Procedure](#rollback-procedure)
4. [Data Preservation](#data-preservation)
5. [Temporal Workflow Cleanup](#temporal-workflow-cleanup)
6. [Verification Steps](#verification-steps)
7. [Re-Migration Procedure](#re-migration-procedure)
8. [Troubleshooting](#troubleshooting)

---

## Rollback Scenarios

### When to Rollback

Roll back to tmux backend when experiencing:

1. **Temporal Server Outage**
   - Temporal server is unreachable
   - Connection timeouts or network issues
   - Temporal cluster is down for maintenance

2. **Workflow Execution Errors**
   - Session workflows failing consistently
   - Workflow state corruption
   - Activity timeouts or failures

3. **Performance Issues**
   - High latency in session operations
   - Temporal server resource exhaustion
   - Workflow queue backlog

4. **Integration Bugs**
   - Bugs in Temporal backend implementation
   - Data inconsistencies between SQLite and Temporal
   - Missing features in Temporal backend

5. **Debugging Requirements**
   - Need to debug with tmux-native tools
   - Isolating Temporal-related issues
   - Testing tmux-specific functionality

### When NOT to Rollback

Do NOT rollback if:

- Issue is isolated to a single session (use `agm session recover` or `agm session kill --hard`)
- Problem is with tmux itself (rollback won't help)
- Data loss would occur (ensure data preservation first)
- Temporary Temporal server maintenance (wait for recovery instead)

---

## Pre-Rollback Checklist

Before performing rollback, verify the following:

- [ ] **Identify Root Cause**: Confirm issue is with Temporal backend, not tmux or AGM core
- [ ] **Check Active Sessions**: List all active sessions to understand impact
- [ ] **Backup Data**: Ensure recent backup of SQLite database exists
- [ ] **Communication**: Notify users if this is a multi-user environment
- [ ] **Documentation**: Record reason for rollback for future analysis

**Commands to run:**

```bash
# Check active sessions
agm list

# Check database location
ls -lh ~/.agm/sessions.db

# Check current backend
echo $AGM_SESSION_BACKEND

# Backup database
cp ~/.agm/sessions.db ~/.agm/sessions.db.backup-$(date +%Y%m%d-%H%M%S)
```

---

## Rollback Procedure

### Step 1: Switch Environment Variable

Set the backend to tmux (or unset to use default):

```bash
# Option A: Set explicitly to tmux
export AGM_SESSION_BACKEND=tmux

# Option B: Unset to use default (tmux)
unset AGM_SESSION_BACKEND

# Make permanent (add to ~/.bashrc or ~/.zshrc)
echo 'export AGM_SESSION_BACKEND=tmux' >> ~/.bashrc
source ~/.bashrc
```

**Verification:**

```bash
# Confirm environment variable
echo $AGM_SESSION_BACKEND
# Output: tmux (or empty, which defaults to tmux)
```

### Step 2: Restart AGM

If using AGM daemon or persistent processes, restart them:

```bash
# If using daemon (if implemented)
# agm-daemon restart

# No restart needed for CLI-only usage
# AGM reads environment variable on each invocation
```

**Note**: AGM CLI reads `AGM_SESSION_BACKEND` on every command execution, so no restart is typically needed unless using daemon mode.

### Step 3: Verify Tmux Sessions Are Detected

Confirm that AGM can now interact with tmux sessions:

```bash
# List sessions (should use tmux backend)
agm list

# Check that tmux sessions are visible
tmux ls

# Verify AGM can query session info
agm session info <session-name>
```

**Expected Behavior:**
- `agm list` shows sessions managed by tmux
- No errors about Temporal connection
- Session operations work normally

### Step 4: Preserve Data (if needed)

If you have session data in SQLite that needs to be synced back to YAML manifests:

```bash
# Export sessions from SQLite to YAML
agm admin sync-db-to-yaml

# Or manually export specific session
agm session export <session-name> --format yaml
```

**Note**: AGM uses dual-write strategy (YAML + SQLite), so manifest files should already exist. This step is only needed if data diverged.

### Step 5: Clean Up Temporal Workflows (Optional)

See [Temporal Workflow Cleanup](#temporal-workflow-cleanup) section for details.

**Quick decision guide:**
- **Skip cleanup** if you plan to re-migrate to Temporal soon (workflows will timeout naturally)
- **Perform cleanup** if rollback is permanent or long-term (prevents resource usage)

### Step 6: Verify Rollback Successful

Run comprehensive verification:

```bash
# Test session listing
agm list

# Test session creation
agm create test-rollback-session --project test

# Test session attachment
agm resume test-rollback-session

# Test session operations (inside tmux session)
# Ctrl-B + D to detach

# Clean up test session
agm stop test-rollback-session
```

**Success Criteria:**
- All commands complete without Temporal-related errors
- Sessions are created in tmux
- Attachment/detachment works
- No error messages about Temporal connection

---

## Data Preservation

### Understanding Data Storage

AGM uses a dual-storage model:

1. **YAML Manifests**: `~/src/sessions/<session-name>/manifest.yaml`
   - Primary source of truth for tmux backend
   - Human-readable, git-trackable
   - Contains session metadata (name, project, tags, timestamps)

2. **SQLite Database**: `~/.agm/sessions.db`
   - Used by Temporal backend for fast queries
   - Searchable session history
   - Activity logs and metrics

3. **Temporal Workflow State**: (Temporal server)
   - Workflow execution history
   - Activity results
   - Signals and queries

### What Data is Preserved Automatically

When rolling back from Temporal to tmux:

**Automatically Preserved:**
- ✅ Session manifests (YAML files) - already on disk
- ✅ SQLite database - remains intact
- ✅ Session conversation history (HTML files)
- ✅ Tmux session state (if sessions still running)

**Requires Manual Export:**
- ⚠️ Temporal workflow execution history (if needed for debugging)
- ⚠️ Temporal workflow state (if workflows contain unique data)

### Exporting Data from SQLite

If you need to export SQLite data to YAML:

```bash
# Export all sessions to YAML manifests
agm admin export-db --format yaml --output ~/src/sessions/

# Export specific session
agm session export <session-name> --output ~/src/sessions/<session-name>/manifest.yaml

# Dump SQLite database to SQL
sqlite3 ~/.agm/sessions.db .dump > ~/agm-sessions-backup.sql

# Export as JSON for analysis
sqlite3 ~/.agm/sessions.db "SELECT * FROM sessions;" -json > ~/sessions.json
```

### Syncing SQLite to YAML Manifests

If manifest files are out of sync with SQLite:

```bash
# Sync all sessions from DB to YAML
agm admin sync --db-to-yaml

# Force overwrite existing manifests
agm admin sync --db-to-yaml --force

# Preview changes without modifying files
agm admin sync --db-to-yaml --dry-run
```

**Note**: This command may not exist yet. If not, use manual export approach above.

### Session History Preservation

Conversation history is stored separately:

```bash
# Location: ~/src/sessions/<session-name>/*.html
ls ~/src/sessions/<session-name>/

# Backup conversation history
tar -czf ~/session-conversations-backup-$(date +%Y%m%d).tar.gz ~/src/sessions/
```

---

## Temporal Workflow Cleanup

### Do You Need to Clean Up Workflows?

**Decision Matrix:**

| Scenario | Cleanup Needed? | Reason |
|----------|----------------|---------|
| Temporary rollback (< 1 week) | No | Workflows will timeout naturally |
| Permanent rollback | Yes | Prevent resource usage |
| Temporal server issues | No | Cannot connect to clean up |
| Debugging/testing | No | May need workflow history |
| Long-term rollback (> 1 week) | Yes | Good housekeeping |

### Listing Active Workflows

If Temporal server is reachable:

```bash
# Using tctl (Temporal CLI)
tctl workflow list --namespace agm

# Filter AGM session workflows
tctl workflow list --namespace agm --query "WorkflowType='SessionWorkflow'"

# Get specific workflow details
tctl workflow describe --workflow-id <session-id>
```

**Note**: Requires Temporal CLI (`tctl`) installed and configured.

### Terminating Workflows Gracefully

**Option 1: Cancel Workflows (Graceful)**

```bash
# Cancel specific session workflow
tctl workflow cancel --workflow-id <session-id> --reason "Rollback to tmux backend"

# Cancel all session workflows (use with caution)
tctl workflow list --namespace agm --query "WorkflowType='SessionWorkflow'" \
  | jq -r '.executions[].execution.workflowId' \
  | xargs -I {} tctl workflow cancel --workflow-id {}
```

**Option 2: Terminate Workflows (Immediate)**

```bash
# Terminate specific workflow (no cleanup activities run)
tctl workflow terminate --workflow-id <session-id> --reason "Rollback to tmux backend"

# Terminate multiple workflows
tctl workflow list --namespace agm --open \
  | jq -r '.executions[].execution.workflowId' \
  | xargs -I {} tctl workflow terminate --workflow-id {} --reason "Rollback"
```

**Difference:**
- **Cancel**: Sends cancellation signal, allows workflow to clean up gracefully
- **Terminate**: Immediately stops workflow without cleanup

**Recommendation**: Use **cancel** unless workflows are stuck/unresponsive.

### Cleaning Up Workflow History

Workflow history is retained in Temporal server:

```bash
# History retention is configured at namespace level
tctl namespace describe agm

# To manually delete closed workflows (not recommended)
# Contact Temporal admin or use retention policies
```

**Note**: Workflow history is generally harmless and can be retained for debugging. Temporal server's retention policy will eventually clean it up.

### Cleanup is Optional - Tradeoffs

**Pros of Cleaning Up:**
- Reduces resource usage on Temporal server
- Cleaner workflow list
- Prevents confusion (no orphaned workflows)

**Cons of Cleaning Up:**
- Loses workflow execution history (useful for debugging)
- Cannot query workflow state after cleanup
- Requires Temporal server access (may not be available during outage)

**Recommendation**: Skip cleanup if:
- Rollback is temporary (< 1 week)
- Temporal server is unreachable
- You may need workflow history for debugging

---

## Verification Steps

### Post-Rollback Testing

After completing rollback, verify the following:

#### 1. Backend Selection

```bash
# Verify environment variable
echo $AGM_SESSION_BACKEND
# Expected: tmux (or empty)

# Verify AGM uses tmux backend
agm list --debug 2>&1 | grep -i backend
# Should show "using tmux backend" (if debug logging exists)
```

#### 2. Session Listing

```bash
# List sessions via AGM
agm list

# List sessions via tmux directly
tmux ls

# Compare outputs - should match
```

#### 3. Session Creation

```bash
# Create new session
agm create test-rollback --project verification

# Verify session exists in tmux
tmux has-session -t test-rollback && echo "✓ Session exists in tmux"

# Verify manifest created
ls ~/src/sessions/test-rollback/manifest.yaml && echo "✓ Manifest exists"
```

#### 4. Session Operations

```bash
# Attach to session
agm resume test-rollback

# Inside tmux session, verify:
# - Working directory is correct
# - Session name is correct (check tmux status bar)

# Detach (Ctrl-B + D)

# Resume again to verify re-attachment
agm resume test-rollback
```

#### 5. Session Metadata

```bash
# Check session info
agm session info test-rollback

# Verify fields:
# - Name, project, tags
# - Created/updated timestamps
# - Lifecycle state
```

#### 6. Cleanup Test Session

```bash
# Stop test session
agm stop test-rollback

# Verify session stopped
tmux has-session -t test-rollback 2>/dev/null || echo "✓ Session stopped"
```

### Error-Free Verification

No errors should appear related to:
- ❌ Temporal connection timeouts
- ❌ Workflow execution failures
- ❌ "backend 'temporal' not available"
- ❌ SQLite database connection issues

### Performance Verification

Compare performance before/after rollback:

```bash
# Time session listing
time agm list

# Should be fast (< 1 second for typical session counts)
```

---

## Re-Migration Procedure

If rollback was temporary and you want to migrate back to Temporal:

### Prerequisites

- [ ] Temporal server is healthy and reachable
- [ ] Root cause of original issue is resolved
- [ ] Temporal backend code is fixed (if issue was a bug)
- [ ] Testing confirms Temporal backend works correctly

### Step 1: Verify Temporal Availability

```bash
# Ping Temporal server
tctl cluster health

# Check namespace exists
tctl namespace describe agm
```

### Step 2: Use Migration Tool

AGM provides a migration tool (if implemented in Task 5.1):

```bash
# Migrate all tmux sessions to Temporal
agm migrate tmux-to-temporal

# Or migrate specific session
agm migrate tmux-to-temporal --session <session-name>

# Dry-run to preview
agm migrate tmux-to-temporal --dry-run
```

### Step 3: Switch Backend to Temporal

```bash
# Set environment variable
export AGM_SESSION_BACKEND=temporal

# Make permanent
echo 'export AGM_SESSION_BACKEND=temporal' >> ~/.bashrc
source ~/.bashrc
```

### Step 4: Verify Migration

```bash
# List sessions (should use Temporal backend)
agm list

# Verify workflows are running
tctl workflow list --namespace agm

# Test session operations
agm resume <session-name>
```

### Step 5: Monitor for Issues

After re-migration, monitor for:
- Workflow execution errors
- Performance degradation
- Data inconsistencies

**Monitoring commands:**

```bash
# Check workflow status
tctl workflow list --namespace agm --open

# Check for failed workflows
tctl workflow list --namespace agm --query "ExecutionStatus='Failed'"

# AGM logs (if logging is configured)
tail -f ~/.agm/logs/agm.log
```

### Rollback Plan

If re-migration fails:
- Follow this rollback procedure again
- Investigate root cause before attempting re-migration
- Consider gradual migration (migrate a few sessions first)

---

## Troubleshooting

### Issue: "Backend 'tmux' not registered"

**Symptom:**
```
Error: backend "tmux" is not registered (available: [])
```

**Cause**: AGM binary was built without tmux backend support.

**Solution:**
```bash
# Rebuild AGM with all backends
cd main/agm
go build -o ~/bin/agm ./cmd/agm

# Verify backends are registered
agm version --verbose  # Should list available backends
```

### Issue: Sessions Visible in tmux but Not in AGM

**Symptom:** `tmux ls` shows sessions, but `agm list` doesn't.

**Cause**: Manifest files missing or out of sync.

**Solution:**
```bash
# Discover tmux sessions and create manifests
agm admin sync

# Or create manifest manually for specific session
agm admin create-manifest <tmux-session-name>
```

### Issue: Data Loss After Rollback

**Symptom**: Session data is missing after rollback.

**Cause**: Manifest files were not synced before rollback.

**Solution:**
```bash
# Restore from database backup
cp ~/.agm/sessions.db.backup-* ~/.agm/sessions.db

# Export all sessions to YAML
agm admin export-db --format yaml

# Verify manifests exist
ls ~/src/sessions/*/manifest.yaml
```

### Issue: Rollback Successful but Some Sessions Not Working

**Symptom**: Some sessions work, others fail with "session not found".

**Cause**: Tmux sessions were terminated during Temporal backend usage.

**Solution:**
```bash
# Identify missing tmux sessions
agm list --show-missing

# Archive sessions that no longer have tmux sessions
agm archive <session-name>

# Or recreate tmux sessions
agm session recreate <session-name>
```

### Issue: Environment Variable Not Persisting

**Symptom**: `AGM_SESSION_BACKEND=tmux` works in current shell but resets after logout.

**Cause**: Environment variable not added to shell profile.

**Solution:**
```bash
# Add to appropriate shell profile
# For bash:
echo 'export AGM_SESSION_BACKEND=tmux' >> ~/.bashrc

# For zsh:
echo 'export AGM_SESSION_BACKEND=tmux' >> ~/.zshrc

# Reload shell
source ~/.bashrc  # or source ~/.zshrc
```

### Issue: Cannot Connect to Temporal for Cleanup

**Symptom**: Cannot clean up Temporal workflows because server is unreachable.

**Solution:**
- **Acceptable**: Skip cleanup, workflows will timeout naturally
- **Timeouts**: Temporal workflows have timeouts configured (default: 24-72 hours)
- **No impact**: Orphaned workflows in Temporal don't affect tmux backend functionality

### Getting Help

If rollback fails or you encounter issues not covered here:

1. **Check Logs**:
   ```bash
   # AGM logs (if logging is configured)
   tail -f ~/.agm/logs/agm.log

   # Tmux logs
   tmux list-sessions -F "#{session_name}: #{session_created}"
   ```

2. **Enable Debug Mode**:
   ```bash
   export AGM_DEBUG=1
   agm list
   ```

3. **File an Issue**:
   - Repository: https://github.com/vbonnet/ai-tools
   - Include: AGM version, error messages, steps to reproduce
   - Label: `backend`, `rollback`, `temporal`

4. **Emergency Contact**:
   - Check repository README for support channels
   - Discord/Slack/IRC channels (if available)

---

## Appendix

### A. Environment Variable Reference

| Variable | Default | Values | Description |
|----------|---------|--------|-------------|
| `AGM_SESSION_BACKEND` | `tmux` | `tmux`, `temporal` | Selects backend implementation |
| `AGM_DEBUG` | `false` | `true`, `false`, `1`, `0` | Enables debug logging |
| `AGM_TEMPORAL_HOST` | `localhost:7233` | `<host>:<port>` | Temporal server address |
| `AGM_TEMPORAL_NAMESPACE` | `agm` | `<namespace>` | Temporal namespace |

### B. File Locations

| File/Directory | Purpose |
|----------------|---------|
| `~/src/sessions/<session-name>/manifest.yaml` | Session manifest (YAML) |
| `~/src/sessions/<session-name>/*.html` | Conversation history |
| `~/.agm/sessions.db` | SQLite database |
| `~/.agm/logs/` | AGM logs (if configured) |
| `~/.claude/history.jsonl` | Claude session history |

### C. Related Documentation

- [Backend Implementation](../BACKEND_IMPLEMENTATION.md) - Backend architecture
- [Troubleshooting Guide](./TROUBLESHOOTING.md) - General AGM troubleshooting
- [Recovery Commands](./RECOVERY-COMMANDS.md) - Session recovery procedures
- [Migration Guide](./MIGRATION-V2-V3.md) - YAML manifest migration

### D. Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-02-15 | 1.0 | Initial rollback procedure documentation |

---

**Document Status**: ✅ Complete
**Task**: 5.4 - Rollback Procedure Documentation
**Bead**: oss-imm
