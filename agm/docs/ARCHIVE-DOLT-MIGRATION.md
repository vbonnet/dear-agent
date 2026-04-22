# Archive Command Dolt Migration

## Overview

This document describes the migration of the `agm session archive` command from filesystem-based storage to Dolt database storage, fixing the bug where STOPPED sessions could not be archived.

## Problem Statement

**Bug**: `agm session list` displayed 9 STOPPED sessions, but `agm session archive <name>` failed with "session not found" error.

**Root Cause**: AGM was partially migrated to Dolt storage:
- `agm session list` (default) uses **Dolt database** via `getStorage()` adapter
- `agm session archive` used **filesystem resolution** via `session.ResolveIdentifier()`
- The two storage backends were out of sync

Sessions existed in Dolt but the archive command only searched the filesystem.

## Solution

**Approach**: Complete migration to Dolt as single source of truth. No fallbacks, no dual-write, no backward compatibility.

### Changes

#### 1. Added `ResolveIdentifier()` to Dolt Adapter

**File**: `internal/dolt/sessions.go`

```go
// ResolveIdentifier finds a session by session ID, tmux name, or manifest name
func (a *Adapter) ResolveIdentifier(identifier string) (*manifest.Manifest, error) {
    query := `
        SELECT id, created_at, updated_at, status, workspace, model, name, agent,
            context_project, context_purpose, context_tags, context_notes,
            claude_uuid, tmux_session_name, metadata
        FROM agm_sessions
        WHERE workspace = ?
          AND (id = ? OR tmux_session_name = ? OR name = ?)
          AND status != 'archived'
        LIMIT 1
    `
    // ... implementation
}
```

**Key Features**:
- Single SQL query matches against `id`, `tmux_session_name`, OR `name` fields
- Excludes archived sessions (`status != 'archived'`)
- Returns error if not found

#### 2. Updated Archive Command

**File**: `cmd/agm/archive.go`

**Before**:
```go
// Filesystem resolution
m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir)
if err != nil {
    ui.PrintSessionNotFoundError(sessionName, sessionsDir)
    return err
}

// Update lifecycle field
m.Lifecycle = manifest.LifecycleArchived

// Write manifest (automatic backup + UpdatedAt)
if err := manifest.Write(manifestPath, m); err != nil {
    ui.PrintManifestWriteError(err)
    return err
}
```

**After**:
```go
// Get Dolt adapter (single source of truth)
adapter, err := getStorage()
if err != nil {
    return fmt.Errorf("failed to connect to Dolt: %w", err)
}
defer adapter.Close()

// Resolve session identifier via Dolt
m, err := adapter.ResolveIdentifier(sessionName)
if err != nil {
    ui.PrintSessionNotFoundError(sessionName, "Dolt storage")
    return err
}

// Update lifecycle in Dolt (single write)
m.Lifecycle = manifest.LifecycleArchived
if err := adapter.UpdateSession(m); err != nil {
    return fmt.Errorf("failed to archive session: %w", err)
}
```

**Key Changes**:
- Replaced `session.ResolveIdentifier()` with `adapter.ResolveIdentifier()`
- Removed filesystem path handling
- Single write operation to Dolt database
- Removed unused `session` import

## Testing

### Unit Tests

**File**: `internal/dolt/adapter_test.go`

1. **TestResolveIdentifier**: Tests identifier resolution by session ID, tmux name, and manifest name
2. **TestResolveIdentifierExcludesArchived**: Verifies archived sessions are not resolvable
3. **TestResolveIdentifierWithDuplicateNames**: Tests edge cases with duplicate names

Run unit tests:
```bash
cd main/agm
go test ./internal/dolt/... -v
```

### Integration Tests

**File**: `test/integration/lifecycle/archive_test.go`

1. **Archive by session ID**: Verifies archiving using session ID identifier
2. **Archive by tmux name**: Verifies archiving using tmux session name
3. **Archive by manifest name**: Verifies archiving using manifest name field
4. **Non-existent session**: Verifies clear error message
5. **Regression test**: Verifies archived sessions cannot be re-archived

Run integration tests:
```bash
# Requires running Dolt server
export DOLT_TEST_INTEGRATION=1
cd main/agm
go test ./test/integration/lifecycle/... -tags=integration -v
```

### Manual Testing

#### Prerequisites
1. Dolt server running on configured port
2. `WORKSPACE` environment variable set
3. AGM binary built with the fix

#### Test Plan

```bash
# 1. Verify bug reproduction (if testing before fix)
agm session list
# Should show STOPPED sessions

agm session archive tool-usage-compliance
# BEFORE FIX: Error "session not found: tool-usage-compliance"
# AFTER FIX: Success "Archived session: tool-usage-compliance"

# 2. Test archive by session ID
agm session list --json | jq -r '.[0].session_id'
# Copy session ID
agm session archive <session-id>
# Should succeed

# 3. Test archive by tmux session name
agm session list --json | jq -r '.[0].tmux.session_name'
# Copy tmux name
agm session archive <tmux-name>
# Should succeed

# 4. Test archive by manifest name
agm session list --json | jq -r '.[0].name'
# Copy manifest name
agm session archive <manifest-name>
# Should succeed

# 5. Verify archived sessions hidden from default list
agm session list
# Should NOT show archived sessions

agm session list --all
# Should show archived sessions with archived status

# 6. Test error handling
agm session archive non-existent-session
# Should show: "session not found: non-existent-session"

# 7. Bulk archive remaining STOPPED sessions
for session in hook-enforcement agm-send-interrupt agm-opencode; do
    agm session archive "$session"
done

agm session list
# Should show 0 STOPPED sessions
```

## Verification Checklist

- [ ] Unit tests pass: `go test ./internal/dolt/... -v`
- [ ] Integration tests pass: `DOLT_TEST_INTEGRATION=1 go test ./test/integration/lifecycle/... -tags=integration -v`
- [ ] Manual test: Archive by session ID works
- [ ] Manual test: Archive by tmux name works
- [ ] Manual test: Archive by manifest name works
- [ ] Manual test: Non-existent session shows clear error
- [ ] Manual test: Archived session hidden from default list
- [ ] Manual test: Archived session shown with `--all` flag
- [ ] Manual test: Cannot re-archive already archived session
- [ ] No compilation errors
- [ ] No lint errors: `golangci-lint run ./...`

## Migration Path

This fix is part of the larger migration to Dolt storage:

### ✅ Completed (Phase 1)
- [x] `agm session list` - Uses Dolt
- [x] `agm session archive` - Uses Dolt (THIS FIX)

### 🚧 Future Work (Phase 2)
- [ ] `agm session resume` - Migrate to `adapter.ResolveIdentifier()`
- [ ] `agm session kill` - Migrate to `adapter.ResolveIdentifier()`
- [ ] `agm session unarchive` - Migrate to `adapter.ResolveIdentifier()`

### 🗑️ Cleanup (Phase 3)
- [ ] Delete `internal/session/session.go:ResolveIdentifier()`
- [ ] Delete `cmd/agm/list.go` (list-yaml command)
- [ ] Remove filesystem manifest readers
- [ ] Remove JSONL-based session storage code
- [ ] Clean up dead imports

## Design Decisions

1. **No Fallbacks**: Dolt is the only storage backend. No filesystem fallbacks simplifies code and prevents sync issues.

2. **SQL-Based Resolution**: Single query with `OR` conditions is efficient and handles all identifier types (id, tmux name, manifest name).

3. **Exclude Archived**: `ResolveIdentifier()` explicitly filters `status != 'archived'` to prevent re-archiving and maintain clear separation.

4. **Backward Incompatibility**: Breaking change - requires Dolt server. This is intentional for simplicity.

## Troubleshooting

### Error: "failed to connect to Dolt"
**Solution**: Ensure Dolt server is running:
```bash
cd ~/src/ws/<workspace>/.dolt/dolt-db
dolt sql-server -H 127.0.0.1 -P 3307
```

### Error: "WORKSPACE environment variable not set"
**Solution**: Set workspace:
```bash
export WORKSPACE=oss  # or acme, etc.
```

### Error: "session not found" for existing session
**Possible Causes**:
1. Session is already archived (check with `--all` flag)
2. Session exists in filesystem but not in Dolt (migration incomplete)
3. Wrong workspace (check `$WORKSPACE` matches Dolt database)

**Debug**:
```bash
# Check Dolt directly
dolt sql -q "SELECT id, name, status FROM agm_sessions WHERE workspace='oss'"

# Check filesystem
ls ~/sessions/
```

## References

- **Plan Document**: `~/src/.claude/projects/-home-user-src/plans/fix-archive-dolt.md`
- **Issue**: STOPPED sessions cannot be archived - "session not found" error
- **Related Files**:
  - `internal/dolt/sessions.go` - ResolveIdentifier implementation
  - `cmd/agm/archive.go` - Archive command migration
  - `cmd/agm/list_dolt.go` - Example of Dolt usage pattern
  - `cmd/agm/storage.go` - getStorage() helper

## Changelog

### 2026-03-12
- Added `ResolveIdentifier()` method to Dolt adapter
- Migrated `agm session archive` to use Dolt storage
- Added unit tests for ResolveIdentifier
- Added integration tests for Dolt-based archiving
- Updated documentation
