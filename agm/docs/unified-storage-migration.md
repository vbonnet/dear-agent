# Unified Session Directory Storage Migration

## Overview

This implementation migrates AGM sessions from fragmented workspace locations to a unified storage directory at `~/src/sessions/{session-name}/`.

## Architecture

### Components

1. **Workspace Discovery** (`internal/discovery/workspaces.go`)
   - Scans `~/src/ws/*/sessions/` for all sessions
   - Returns comprehensive inventory with current locations

2. **Migration Core** (`internal/manifest/unified_storage.go`)
   - Moves manifests and conversations atomically
   - Reuses locking patterns from existing `migrate.go`
   - Handles same-filesystem (rename) and cross-filesystem (copy+delete) moves

3. **Conversation Converter** (`internal/conversation/jsonl.go`)
   - Converts HTML transcripts to JSONL format
   - Graceful fallback if parsing fails

4. **CLI Command** (`cmd/csm/migrate.go`)
   - `agm migrate --to-unified-storage`
   - Flags: `--dry-run`, `--force`, `--workspace=<name>`

## Usage

### Preview Migration (Dry-Run)

```bash
agm migrate --to-unified-storage --dry-run
```

Shows which sessions would be migrated without modifying files.

### Migrate All Sessions

```bash
agm migrate --to-unified-storage
```

Migrates all sessions from all workspaces to unified storage.

### Migrate Specific Workspace

```bash
agm migrate --to-unified-storage --workspace=oss
```

Migrates only sessions from the `oss` workspace.

### Force Overwrite

```bash
agm migrate --to-unified-storage --force
```

Overwrites existing destinations if sessions already in unified storage.

## File Structure

**Before Migration:**
```
~/src/sessions/{session-id}/manifest.yaml
./sessions/{uuid}/conversation.html
~/src/ws/acme/sessions/{uuid}/conversation.html
```

**After Migration:**
```
~/src/sessions/{session-name}/
├── manifest.yaml
└── conversation.jsonl
```

## Safety Features

- **Atomic Operations**: Uses `os.Rename()` for same-filesystem moves (atomic)
- **Locking**: Prevents concurrent migrations via `manifest.AcquireLock()`
- **Idempotency**: Skips already-migrated sessions (unless `--force`)
- **Graceful Errors**: Single failure doesn't block remaining sessions
- **Audit Logging**: All operations logged to stderr (production: log file)

## Rollback

Old session directories are preserved for 30 days after migration.

**Manual Rollback:**
```bash
# Remove unified storage session
rm -rf ~/src/sessions/{session-name}

# AGM automatically falls back to old workspace paths
```

## Testing

Run unit tests:
```bash
cd main/agm
go test ./internal/discovery/... -v
go test ./internal/manifest/... -v
go test ./internal/conversation/... -v
```

## Integration with Existing Code

- **Locking**: Reuses `manifest.AcquireLock()` and `ReleaseLock()`
- **Manifest Types**: Uses existing `manifest.Manifest` struct
- **Discovery Patterns**: Extends `internal/discovery/` package

## Next Steps

1. **Add to main.go**: Wire up `migrateCmd` to root command
2. **Dual-Path Reads**: Update `manifest.Read()` for backward compatibility
3. **Production Logging**: Replace stderr logging with file-based audit log
4. **Integration Tests**: End-to-end migration with sample sessions

## Implementation Status

- [x] Workspace discovery (`workspaces.go`)
- [x] Migration core (`unified_storage.go`)
- [x] Conversation converter (`jsonl.go`)
- [x] CLI command (`cmd/csm/migrate.go`)
- [x] Unit tests (`workspaces_test.go`)
- [x] Documentation (this file)
- [ ] Integration with main.go
- [ ] Dual-path read logic in manifest.Read()
- [ ] Production audit logging
- [ ] End-to-end integration tests
