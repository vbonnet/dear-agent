# S1: Sprint 1 - Foundation & Core Infrastructure (v2)

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Round 2
**Version**: 2.0
**Sprint Goal**: Implement foundational infrastructure for session persistence
**Prerequisites**:
- D1 Discovery ✅ Complete
- D2 Architecture ✅ Approved (8.8/10)
- D3 Implementation ✅ Approved (9.0/10)
- D4 Requirements ✅ Approved (9.3/10)
- S1 Round 1 ❌ Revision needed (8.2/10)

---

## Executive Summary

Sprint 1 focuses on implementing the **foundational infrastructure** needed for session persistence. This includes the manifest schema v2, migration logic, file locking, and core utilities.

**Scope**: 5 deliverables (of 11 total in Phase 3.5)
**Duration Estimate**: 2-3 days of focused development
**Dependencies**: None (foundational work)

**Changes from v1**:
- Lock file format exactly specified
- Migration acquires lock before processing
- All file permissions specified (0600)
- Test data preparation added
- All error messages specified
- Post-deployment verification checklist added

**Strategic Rationale**: Build the foundation first. These components are dependencies for all other features. Without schema v2, migration, locking, and utilities, we cannot build resume auto-recreation, backup, or doctor commands.

---

## Sprint Goal

**Primary Goal**: Implement core infrastructure that enables safe, concurrent manifest operations with automatic v1→v2 migration.

**Success Criteria**:
1. ✅ All v1 manifests automatically migrate to v2 with backup
2. ✅ Concurrent operations are prevented by file locking
3. ✅ All edge cases handled (corrupted files, rollback, etc.)
4. ✅ 100% test coverage for critical paths
5. ✅ Zero data loss under any failure scenario

---

## Technical Specifications

### Lock File Format

**Exact format**:
```
<PID>\n<RFC3339>\n
```

**Example**:
```
12345
2025-12-07T14:30:00-08:00
```

**Specification**:
- Line 1: Process ID (PID) as decimal number
- Line 2: RFC3339 timestamp with timezone
- Both lines terminated with `\n`
- Total: Two lines with trailing newline

**Parsing**:
```go
data, _ := os.ReadFile(lockPath)
lines := strings.Split(strings.TrimSpace(string(data)), "\n")
pid := lines[0]  // "12345"
timestamp := lines[1]  // "2025-12-07T14:30:00-08:00"
```

**Writing**:
```go
content := fmt.Sprintf("%d\n%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
os.WriteFile(lockPath, []byte(content), 0600)
```

### Migration Log Format

**Format**: `[<RFC3339>] <STATUS>: <path> [- <error>]`

**Examples**:
```
[2025-12-07T14:30:00-08:00] SUCCESS: /home/user/sessions/session-claude-1/manifest.yaml
[2025-12-07T14:31:00-08:00] FAILED: /home/user/sessions/session-claude-2/manifest.yaml - invalid YAML syntax
```

**Location**: `~/.csm/logs/migration.log`
**Permissions**: 0600 (user-only)

### One-Time Notice Text

**File**: `~/.csm/.migration-notice-shown`
**Permissions**: 0600

**Message** (shown on first migration):
```
ℹ️  CSM has upgraded to schema v2 for better session persistence.

   What's changing:
   • Your manifests will be automatically migrated to the new schema
   • Backups are saved as *.v1.bak files (safe to delete after verification)
   • Sessions now track purpose, tags, and notes for better organization

   This is a one-time migration. Your sessions will continue to work normally.

   For more info: csm help migrate
```

### Error Messages

**Lock Conflict**:
```
Error: session is locked by process 12345 (started 2025-12-07T14:30:00-08:00)

Try one of the following:
  • Wait a minute and retry (process may finish)
  • Check if process is still running: ps -p 12345
  • If process is stuck, kill it: kill 12345
  • Check for stale locks: csm doctor --fix
```

**Validation Error**:
```
Error: context validation failed: purpose too long (300 chars, max 256)

Please shorten the purpose field and try again.
```

**Migration Failure**:
```
Error: migration failed: cannot write manifest

The migration has been rolled back. Your original manifest was restored from backup.

Check disk space and file permissions, then try again.
```

**Rollback Message**:
```
⚠️  Migration failed, rolling back to original manifest...
✅ Rollback successful - your original manifest has been restored from backup
```

**Success Message** (in terminal):
```
📝 Migrating session-claude-1 to schema v2...
✅ Migration successful - backup saved as manifest.yaml.v1.bak
```

### File Permissions

All files created by S1 components:
- **Lock files** (`.lock`): 0600 (user-only read/write)
- **Log files** (`migration.log`): 0600
- **Notice file** (`.migration-notice-shown`): 0600
- **Backup files** (`.v1.bak`): Inherit from original manifest
- **Temp files** (`.tmp`): 0600

### Path Validation

All file paths must be validated to prevent directory traversal:

```go
func validatePath(path string, baseDir string) error {
    // Clean the path
    clean := filepath.Clean(path)

    // Make absolute
    abs, err := filepath.Abs(clean)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }

    // Check it's within base directory
    if !strings.HasPrefix(abs, baseDir) {
        return fmt.Errorf("path outside sessions directory")
    }

    return nil
}
```

---

## Deliverables

### D1.1: Manifest Schema v2 (FR-1)
**Priority**: P0 (Must Have)
**Estimated Effort**: 4 hours

**Tasks**:
1. Update `internal/manifest/manifest.go`:
   - Add `SchemaVersion string` field
   - Add `Lifecycle string` field (replaces Status)
   - Add `Context` struct with Purpose, Tags, Notes
   - Remove `Status` field (computed dynamically)
   - Update all timestamp fields to use RFC3339

2. Create `internal/manifest/constants.go`:
   - Schema version constant: `"2.0"`
   - Lifecycle constants: `""`, `"archived"`
   - Status constants: `"active"`, `"stopped"`, `"archived"`
   - Validation limits: MaxPurposeLen, MaxTagsCount, MaxTagLen, MaxNotesLen
   - Lock timeout: 60 seconds (not 5 minutes - v1 error corrected)
   - Backup retention: 10 max

3. Update YAML serialization:
   - Ensure RFC3339 timestamp format
   - Test roundtrip (write → read → verify)

**Acceptance Criteria**:
- [ ] Manifest struct includes all v2 fields
- [ ] Constants file centralized
- [ ] YAML serialization works correctly
- [ ] No backward references to v1 Status field
- [ ] All timestamps use RFC3339 format
- [ ] Lock timeout is 60 seconds (consistent everywhere)

**Tests**:
- `manifest_test.go`: Schema structure, serialization
- `constants_test.go`: Constant values validation

---

### D1.2: Context Field Validation (FR-1.2, FR-3)
**Priority**: P0 (Must Have)
**Estimated Effort**: 3 hours

**Tasks**:
1. Create `internal/manifest/validate.go`:
   - `func (c *Context) Validate() error`
   - `func (m *Manifest) Validate() error` (calls Context.Validate)
   - UTF-8 character counting (not byte length)
   - Boundary condition handling (exactly N chars = PASS)
   - Clear error messages with actual vs max

2. Validation rules:
   - Purpose: max 256 UTF-8 characters
   - Tags: max 10 tags
   - Each tag: max 32 UTF-8 characters, no whitespace
   - Notes: max 1024 UTF-8 characters

3. Error message format:
   - Template: "context validation failed: <field> <issue> (<actual> chars, max <limit>)"
   - Example: "context validation failed: purpose too long (300 chars, max 256)"

4. Validation timing:
   - Called before YAML serialization (catch errors early)
   - Called on write only, not on read
   - Never truncate (always reject invalid data)

**Acceptance Criteria**:
- [ ] Writing with purpose > 256 chars returns validation error
- [ ] Writing with > 10 tags returns validation error
- [ ] Writing with tag > 32 chars returns validation error
- [ ] Tag with whitespace returns validation error
- [ ] Writing with notes > 1024 chars returns validation error
- [ ] Valid context at exact limits passes validation
- [ ] UTF-8 multibyte characters (emoji) counted correctly
- [ ] Error messages are clear and actionable
- [ ] Validation called before serialization
- [ ] Invalid data never truncated (always rejected)

**Tests**:
- `validate_test.go`: All boundary conditions, UTF-8 handling, error messages

---

### D1.3: File Locking (FR-4)
**Priority**: P0 (Must Have)
**Estimated Effort**: 6 hours

**Tasks**:
1. Create `internal/manifest/lock.go`:
   - `func AcquireLock(manifestPath string) (*Lock, error)`
   - `func (l *Lock) Release() error`
   - Lock file format: See Technical Specifications above
   - Use O_EXCL for atomic creation (prevents symlink attacks)
   - Stale lock detection (60s timeout, confirmed)
   - Auto-cleanup of stale locks
   - File permissions: 0600 (user-only)
   - Path validation (prevent directory traversal)

2. Lock lifecycle:
   - Acquire: Create `.lock` file with PID and timestamp (format specified above)
   - Release: Remove `.lock` file (use defer)
   - Stale detection: Check timestamp, remove if > 60s old
   - No retry on conflict (fail fast with clear error)

3. Error handling:
   - Lock conflict: Include PID, timestamp, and suggestions
   - Error format: See Technical Specifications above
   - NFS limitation: Document in code comments
   - O_EXCL symlink protection: Document security property

4. Lock scope:
   - Lock manifest file only (not entire directory)
   - Sufficient for S1 (single manifest operations)
   - Future: May need directory-level locking for multi-file operations

**Acceptance Criteria**:
- [ ] First process acquires lock successfully
- [ ] Second concurrent process gets lock error
- [ ] Lock file created: `manifest.yaml.lock`
- [ ] Lock file contains PID (line 1) and RFC3339 timestamp (line 2)
- [ ] Lock file format exactly as specified
- [ ] Normal completion releases lock
- [ ] Error/panic releases lock via defer
- [ ] Stale lock (> 60s) removed automatically
- [ ] New lock acquired after stale removal
- [ ] Error message includes PID, timestamp, and suggestions
- [ ] Lock file created with 0600 permissions
- [ ] O_EXCL prevents symlink attacks
- [ ] Path validation prevents directory traversal
- [ ] No retry on lock conflict (fail fast)

**Tests**:
- `lock_test.go`: Acquire, release, concurrent access
- `lock_stale_test.go`: Stale lock detection and cleanup
- `lock_integration_test.go`: Real file system test
- `lock_security_test.go`: Symlink attack prevention

---

### D1.4: Schema Migration (FR-2)
**Priority**: P0 (Must Have)
**Estimated Effort**: 8 hours

**Tasks**:
1. **Test Data Preparation** (NEW):
   - Create `testdata/manifests/v1/` directory
   - Add real v1 manifest samples (5+ variations)
   - Add corrupted manifest samples (invalid YAML, missing fields)
   - Add manifests with data that violates v2 limits

2. Create `internal/manifest/migrate.go`:
   - `func migrateV1ToV2(manifestPath string) (*Manifest, error)`
   - **CRITICAL**: Acquire lock before migration (prevents race)
   - Detect schema version (missing = v1)
   - Create backup: `manifest.yaml.v1.bak`
   - Validate: If backup exists, fail (prevents overwriting)
   - Parse v1 with strict YAML
   - Build v2 with atomic initialization (example below)
   - Validate v2 before writing
   - Rollback on failure (restore from backup)
   - Release lock after migration

3. Atomic struct initialization (code example):
   ```go
   // Parse into temporary struct (all or nothing)
   temp := struct {
       SessionID    string                 `yaml:"session_id"`
       CreatedAt    time.Time              `yaml:"created_at"`
       LastActivity time.Time              `yaml:"last_activity"`
       Worktree     map[string]interface{} `yaml:"worktree"`
       Claude       map[string]interface{} `yaml:"claude"`
       Tmux         map[string]interface{} `yaml:"tmux"`
   }{}

   err := yaml.UnmarshalStrict(data, &temp)
   if err != nil {
       return nil, fmt.Errorf("invalid v1 manifest: %w", err)
   }

   // Validate all required fields present (fail fast)
   if temp.SessionID == "" {
       return nil, fmt.Errorf("missing session_id")
   }
   // ... more validation ...

   // Build v2 manifest (only if all valid)
   m := &Manifest{
       SchemaVersion: "2.0",
       SessionID:     temp.SessionID,
       CreatedAt:     temp.CreatedAt,
       LastActivity:  temp.LastActivity,
       Lifecycle:     "",  // Computed dynamically
       Context: Context{
           Purpose: "",
           Tags:    []string{},
           Notes:   "",
       },
       // ... parse nested structs ...
   }

   return m, nil
   ```

4. Migration logging to `~/.csm/logs/migration.log`:
   - Log format: See Technical Specifications above
   - Create log directory if doesn't exist
   - Append-only writes
   - File permissions: 0600
   - Best-effort: Silently ignore logging errors (don't fail migration)

5. User messaging:
   - Check if stdout is TTY (`os.Stdout.Stat() & os.ModeCharDevice`)
   - If TTY: Show messages (see Technical Specifications)
   - If pipe: Silent (no stdout pollution)
   - One-time notice: Create `~/.csm/.migration-notice-shown` after first migration
   - Notice permissions: 0600

6. Error wrapping strategy:
   - Always use `fmt.Errorf("...: %w", err)` for wrapping
   - Enables error inspection with `errors.Is/errors.As`
   - Example: `return nil, fmt.Errorf("migration failed: %w", err)`

**Acceptance Criteria**:
- [ ] Lock acquired before migration starts
- [ ] Loading v1 manifest triggers migration
- [ ] Backup created (.v1.bak) before migration
- [ ] V2 manifest written successfully
- [ ] Subsequent loads use v2 (no re-migration)
- [ ] If backup exists, migration fails (prevents overwrite)
- [ ] Write failure triggers rollback
- [ ] Original v1 restored from backup on failure
- [ ] Error message indicates rollback occurred (see Technical Specs)
- [ ] Migration with missing required fields fails
- [ ] Migration with invalid types fails
- [ ] Migration with malformed YAML fails
- [ ] Successful migration logged with timestamp and path
- [ ] Failed migration logged with error details
- [ ] Log file created with 0600 permissions
- [ ] In terminal: migration messages shown (see Technical Specs)
- [ ] In pipe: no messages to stdout
- [ ] One-time notice shown on first migration per user
- [ ] Notice file created with 0600 permissions
- [ ] Errors wrapped with %w for inspection
- [ ] Lock released after migration (success or failure)

**Tests**:
- `migrate_test.go`: Happy path, validation
- `migrate_rollback_test.go`: Failure scenarios, rollback verification
- `migrate_edge_test.go`: Malformed YAML, partial data, backup collision
- `migrate_concurrent_test.go`: Two processes load same v1 simultaneously (NEW)
- `migrate_security_test.go`: File permissions, path validation (NEW)

---

### D1.5: Fileutil Package (FR-10)
**Priority**: P1 (Should Have)
**Estimated Effort**: 4 hours

**Tasks**:
1. Create `internal/fileutil/fileutil.go`:
   - `func CopyFile(src, dst string) error`
   - `func WriteAtomic(path string, data []byte, perm os.FileMode) error`
   - `func CopyDirectory(src, dst string) error`

2. CopyFile validation:
   - Source != destination
   - Source exists and is not a directory
   - Destination is writable
   - Preserve permissions
   - Path validation (prevent directory traversal)

3. WriteAtomic implementation:
   - Create temp file: `<path>.tmp` with permissions 0600
   - Write data to temp
   - Rename temp to final path (atomic on POSIX)
   - Remove temp on error
   - Path validation

4. CopyDirectory implementation:
   - Recursive copy using filepath.Walk
   - Preserve permissions
   - **Symlink handling**: Copy symlink as symlink (not target)
   - Use `os.Lstat` (not `os.Stat`) to detect symlinks
   - Use `os.Readlink` and `os.Symlink` to copy links

5. Error wrapping:
   - All errors wrapped with context
   - Example: `return fmt.Errorf("failed to copy file: %w", err)`

**Acceptance Criteria**:
- [ ] Valid copy succeeds
- [ ] Same src and dst returns error
- [ ] Source is directory returns error
- [ ] Permissions preserved in CopyFile
- [ ] WriteAtomic creates temp file with 0600
- [ ] WriteAtomic renames atomically
- [ ] On error, temp file removed
- [ ] CopyDirectory copies all files
- [ ] CopyDirectory copies all subdirectories
- [ ] CopyDirectory preserves permissions
- [ ] Symlinks copied as symlinks (not targets)
- [ ] Path validation prevents directory traversal
- [ ] All errors wrapped with context

**Tests**:
- `fileutil_test.go`: All functions, edge cases, error handling
- `fileutil_symlink_test.go`: Symlink handling (NEW)
- `fileutil_security_test.go`: Path validation, permissions (NEW)

---

## Out of Scope (Later Sprints)

The following are **NOT** included in S1:

- Enhanced resume with auto-recreation (S2)
- Backup command (S2)
- Doctor command (S3)
- Status computation (S2)
- Configurable sessions directory (already implemented in Phase 3)
- Integration tests (S3)
- Performance benchmarks (S3)
- Log rotation for migration.log (S3 - OR-3)

---

## Testing Strategy

### Test Data Preparation (NEW)

**Location**: `internal/manifest/testdata/`

**V1 Manifests**:
1. `v1-simple.yaml`: Basic v1 manifest
2. `v1-complete.yaml`: All optional fields populated
3. `v1-minimal.yaml`: Only required fields
4. `v1-invalid-types.yaml`: Wrong field types
5. `v1-missing-fields.yaml`: Missing required fields

**Corrupted Manifests**:
1. `corrupted-yaml.yaml`: Invalid YAML syntax
2. `corrupted-utf8.yaml`: Invalid UTF-8 sequences
3. `empty.yaml`: Empty file
4. `truncated.yaml`: Incomplete YAML

**V2 Manifests** (for validation testing):
1. `v2-valid-boundaries.yaml`: All fields at exact limits
2. `v2-invalid-purpose.yaml`: Purpose > 256 chars
3. `v2-invalid-tags.yaml`: > 10 tags
4. `v2-emoji.yaml`: Emoji in all fields (UTF-8 test)

### Unit Tests (per-file)
- `manifest_test.go`: Schema structure, serialization
- `constants_test.go`: Constant values
- `validate_test.go`: All validation rules, boundary conditions
- `lock_test.go`: Lock lifecycle, concurrent access
- `lock_stale_test.go`: Stale lock detection
- `lock_security_test.go`: Permissions, symlink attacks (NEW)
- `migrate_test.go`: Happy path, validation
- `migrate_rollback_test.go`: Failure scenarios
- `migrate_edge_test.go`: Edge cases
- `migrate_concurrent_test.go`: Concurrent migration (NEW)
- `migrate_security_test.go`: Permissions, path validation (NEW)
- `fileutil_test.go`: All utility functions
- `fileutil_symlink_test.go`: Symlink handling (NEW)
- `fileutil_security_test.go`: Permissions, path validation (NEW)

### Integration Tests (S1 scope)

**TS-S1-1: V1 Migration Flow**
- Load v1 manifest → auto-migrate → verify v2 → verify backup created

**TS-S1-2: Concurrent Migration** (NEW from Round 1 feedback)
- Two processes load same v1 manifest simultaneously
- First acquires lock, migrates successfully
- Second waits for lock, then reads migrated v2 (no double migration)

**TS-S1-3: Concurrent Lock Conflict**
- Process 1 acquires lock
- Process 2 attempts acquire → fails with clear error
- Process 1 releases lock
- Process 2 acquires lock successfully

**TS-S1-4: Migration Rollback**
- Inject write failure (read-only directory)
- Migration fails gracefully
- Original v1 restored from backup
- Error message shows rollback occurred

**TS-S1-5: Stale Lock Recovery**
- Create lock file with timestamp > 60s old
- Next operation detects stale lock
- Stale lock removed automatically
- Operation proceeds normally

**TS-S1-6: Validation Boundary Conditions** (NEW)
- Purpose with exactly 256 UTF-8 characters → PASS
- Purpose with exactly 257 UTF-8 characters → FAIL
- Tag with exactly 32 UTF-8 characters (emoji) → PASS
- Tag with exactly 33 UTF-8 characters → FAIL

**TS-S1-7: Lock File Orphaned** (NEW)
- Create lock file
- Simulate process crash (no Release call)
- Wait 61 seconds
- Next operation cleans up and proceeds

**TS-S1-8: Backup File Already Exists** (NEW)
- v1 manifest exists
- .v1.bak already exists
- Migration fails with clear error (prevents overwriting backup)

**TS-S1-9: Validation During Migration** (NEW)
- v1 manifest with purpose > 256 chars
- Migration should fail (no truncation)

**TS-S1-10: Atomic Write Failure** (NEW)
- Temp file created
- Rename fails (permissions changed mid-operation)
- Temp file cleaned up
- Clear error message

### Stress Tests (NEW)

**Concurrent Operations** (10 processes):
- 10 processes attempt to acquire same lock
- Only 1 succeeds
- Others fail with clear errors
- No deadlocks, no corruption

**UTF-8 Character Tests** (NEW):
- Purpose with emoji (🚀 = 4 bytes, 1 char)
- Tag with combining characters (é = 2 chars vs 1 char)
- Zero-width characters
- Verify byte count ≠ character count

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All FR with P0: 100%

---

## Implementation Order

Day 1 (Foundation):
1. Morning: Constants (1h) + Manifest Schema (3h)
2. Afternoon: Validation (3h) + Fileutil (3h)

Day 2 (Core Logic):
3. Morning: File Locking (4h)
4. Afternoon: Migration (4h)

Day 3 (Testing & Polish):
5. Morning: Complete all unit tests (4h)
6. Afternoon: Integration tests (2h) + Documentation (2h)

---

## Risk Management

### Risk 1: Migration Bugs Corrupt Manifests
**Probability**: MEDIUM
**Impact**: HIGH
**Mitigation**:
- ✅ Acquire lock before migration (prevents race)
- ✅ Always backup before migration (.v1.bak)
- ✅ Rollback on any failure
- ✅ Atomic struct initialization (all-or-nothing)
- ✅ Extensive testing with real v1 manifests
- ✅ Keep backup files (don't auto-delete)

### Risk 2: Lock Conflicts on NFS
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Document NFS limitation in code and docs (OR-6)
- ✅ Use standard O_EXCL (works on most NFS)
- ✅ Warn users not to use CSM on NFS mounts
- ✅ Stale lock timeout (60s, not 5 min) as fallback

### Risk 3: UTF-8 Character Counting Errors
**Probability**: LOW
**Impact**: LOW
**Mitigation**:
- ✅ Use `unicode/utf8.RuneCountInString()` (not `len()`)
- ✅ Test with emoji and multibyte characters
- ✅ Boundary condition tests (exactly N chars)
- ✅ Verify byte count ≠ character count in tests

### Risk 4: Partial File Writes
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Atomic writes (temp + rename)
- ✅ Rollback on write failure
- ✅ No partial manifests ever created
- ✅ Temp files created with 0600 permissions

### Risk 5: Symlink Attacks
**Probability**: LOW
**Impact**: LOW
**Mitigation**:
- ✅ O_EXCL prevents following symlinks on lock creation
- ✅ Path validation prevents directory traversal
- ✅ All files created with 0600 permissions
- ✅ Document security properties in code

---

## Success Metrics

### Functional
- [ ] All 5 deliverables implemented
- [ ] All P0 acceptance criteria met
- [ ] Zero data loss scenarios
- [ ] All edge cases handled

### Quality
- [ ] >80% test coverage for critical paths
- [ ] >60% test coverage overall
- [ ] All unit tests passing
- [ ] All integration tests passing
- [ ] Zero known bugs

### Performance
- [ ] Manifest load (with migration) < 100ms
- [ ] Lock acquire/release < 10ms
- [ ] Validation < 1ms

---

## Documentation Requirements

### Code Documentation
- [ ] Godoc comments on all exported functions
- [ ] Inline comments for complex logic
- [ ] Lock file format documented in code
- [ ] NFS limitation documented in lock.go
- [ ] O_EXCL symlink protection documented
- [ ] Error wrapping strategy documented

### User Documentation (DR-1)
- [ ] Migration behavior documented
- [ ] Lock file behavior documented
- [ ] Validation limits documented
- [ ] All error messages documented (see Technical Specs)
- [ ] One-time notice text documented (see Technical Specs)

### Developer Documentation
- [ ] README updated with schema v2 info
- [ ] CHANGELOG entry for migration
- [ ] Migration guide (DR-2) - draft started
- [ ] Developer onboarding guide (NEW):
   - S1 component overview
   - Where to start for new contributors
   - File organization explanation

---

## Post-Deployment Verification (NEW)

After deploying S1 to any environment, verify:

### 1. Migration Works
```bash
# Create a test v1 manifest
cp testdata/manifests/v1-simple.yaml /tmp/test-manifest.yaml

# Load it (triggers migration)
csm list

# Verify:
# - .v1.bak file created
# - manifest.yaml now has schema_version: "2.0"
# - migration.log has SUCCESS entry
```

### 2. Locking Works
```bash
# Terminal 1
csm resume claude-1  # This will hold lock

# Terminal 2 (while T1 running)
csm resume claude-1  # Should fail with clear lock error

# Verify error message includes:
# - PID of process in T1
# - Timestamp when lock was acquired
# - Helpful suggestions
```

### 3. Validation Works
```bash
# Try to set invalid context
csm set claude-1 --purpose "$(printf 'x%.0s' {1..300})"

# Verify:
# - Returns validation error
# - Error shows actual (300) vs max (256)
# - Manifest not modified
```

### 4. Backups Created
```bash
# After migration, check:
ls -la ~/sessions/session-*/

# Verify:
# - .v1.bak files exist
# - Permissions are 0600 or inherited
```

### 5. Logging Works
```bash
# Check migration log
cat ~/.csm/logs/migration.log

# Verify:
# - All migrations logged
# - Timestamps in RFC3339 format
# - SUCCESS/FAILED status clear
# - File permissions 0600
```

### 6. Stale Lock Cleanup
```bash
# Create artificial stale lock
echo "99999" > ~/sessions/session-claude-1/manifest.yaml.lock
echo "2025-12-07T00:00:00-08:00" >> ~/sessions/session-claude-1/manifest.yaml.lock

# Wait 61 seconds or modify timestamp to be > 60s old

# Run operation
csm list

# Verify:
# - Stale lock detected and removed
# - Operation proceeds normally
```

---

## Rollback Procedure (NEW)

If S1 deployment has critical bugs and needs to be rolled back:

### 1. Immediate Rollback (Git)
```bash
# Revert to previous commit
git revert <s1-commit-hash>

# Redeploy
go build -o csm ./cmd/csm
```

### 2. Manual Manifest Rollback
If manifests already migrated to v2 and old CSM version can't read them:

```bash
# Find all .v1.bak files
find ~/sessions -name "*.v1.bak"

# For each session, restore backup
for backup in ~/sessions/*/manifest.yaml.v1.bak; do
    manifest="${backup%.v1.bak}"
    echo "Restoring $manifest"
    mv "$backup" "$manifest"
done
```

### 3. Clean Up Migration Artifacts
```bash
# Remove lock files
find ~/sessions -name "*.lock" -delete

# Remove notice file
rm ~/.csm/.migration-notice-shown

# Review migration log for failures
cat ~/.csm/logs/migration.log | grep FAILED
```

### 4. Verify Old CSM Works
```bash
# Test with old CSM version
csm list

# Verify all sessions show correctly
```

**When to Rollback**:
- Migration success rate < 95%
- Data corruption detected
- Critical bugs in locking causing deadlocks

**When NOT to Rollback**:
- Minor UI issues
- Non-critical bugs
- Individual migration failures (can restore from .v1.bak per session)

---

## Definition of Done

S1 is **DONE** when:

1. ✅ All 5 deliverables implemented and tested
2. ✅ All P0 acceptance criteria checked off
3. ✅ Test coverage >80% for critical paths
4. ✅ All tests passing (unit + integration + stress)
5. ✅ Code documented (godoc comments + inline)
6. ✅ Technical specifications all implemented
7. ✅ All error messages match specifications
8. ✅ Multi-persona review score ≥8.5/10
9. ✅ No known critical or high-severity bugs
10. ✅ Integration with existing CSM verified
11. ✅ Migration tested with real v1 manifests
12. ✅ Post-deployment verification checklist completed
13. ✅ Rollback procedure tested
14. ✅ All code committed and pushed

---

## Files to Create/Modify

### New Files
```
internal/manifest/
  ├── constants.go         # NEW - All magic values
  ├── validate.go          # NEW - Context validation
  ├── lock.go              # NEW - File locking
  ├── migrate.go           # NEW - V1→V2 migration
  ├── constants_test.go    # NEW - Tests
  ├── validate_test.go     # NEW - Tests
  ├── lock_test.go         # NEW - Tests
  ├── lock_stale_test.go   # NEW - Tests
  ├── lock_security_test.go  # NEW - Tests (permissions, symlinks)
  ├── migrate_test.go      # NEW - Tests
  ├── migrate_rollback_test.go  # NEW - Tests
  ├── migrate_edge_test.go # NEW - Tests
  ├── migrate_concurrent_test.go  # NEW - Tests (concurrency)
  ├── migrate_security_test.go  # NEW - Tests (permissions, paths)
  └── testdata/            # NEW - Test data directory
      └── manifests/
          ├── v1-simple.yaml
          ├── v1-complete.yaml
          ├── corrupted-yaml.yaml
          └── ... (more test files)

internal/fileutil/
  ├── fileutil.go          # NEW - Utility functions
  ├── fileutil_test.go     # NEW - Tests
  ├── fileutil_symlink_test.go  # NEW - Tests (symlink handling)
  └── fileutil_security_test.go  # NEW - Tests (permissions, paths)
```

### Modified Files
```
internal/manifest/
  ├── manifest.go          # MODIFY - Add v2 schema fields
  └── manifest_test.go     # MODIFY - Add v2 tests
```

---

## Changes from v1

1. ✅ **Lock file format exactly specified**: `"<PID>\n<RFC3339>\n"`
2. ✅ **Migration acquires lock**: Prevents concurrent migration race
3. ✅ **All file permissions specified**: 0600 for lock, log, notice files
4. ✅ **Test data preparation added**: Real v1 manifests, corrupted samples
5. ✅ **All error messages specified**: Lock conflict, validation, migration, rollback
6. ✅ **One-time notice text specified**: Exact message shown to users
7. ✅ **Post-deployment verification**: Checklist for testing S1 works
8. ✅ **Rollback procedure**: Step-by-step guide for emergency rollback
9. ✅ **Atomic initialization example**: Code example showing technique
10. ✅ **Stale lock timeout confirmed**: 60 seconds everywhere (not 5 min)
11. ✅ **Symlink handling specified**: Copy as symlink, not target
12. ✅ **Path validation added**: Prevent directory traversal attacks
13. ✅ **Error wrapping strategy**: Always use %w for error context
14. ✅ **Security tests added**: Permissions, symlinks, paths
15. ✅ **Concurrent migration test**: TS-S1-2 added
16. ✅ **Stress tests added**: 10 concurrent processes, UTF-8 edge cases
17. ✅ **Developer onboarding guide**: README section for new contributors

---

## Next Sprints Preview (Not in S1 Scope)

### S2: Enhanced Resume & Backup
- Enhanced resume with auto-recreation (FR-5)
- Status computation (FR-8)
- Backup command (FR-6)
- Estimated: 2-3 days

### S3: Health & Operations
- Doctor command (FR-7)
- Log rotation (OR-3)
- Integration tests
- Performance benchmarks
- Estimated: 2-3 days

### S4: Polish & Documentation
- Complete migration guide (DR-2)
- Error message consistency (DR-4)
- Help text for all commands (DR-1)
- User acceptance testing
- Estimated: 1-2 days

---

## Review Checklist

Before submitting for multi-persona review:

- [x] All deliverables clearly defined
- [x] All acceptance criteria listed
- [x] All risks identified and mitigated
- [x] Test strategy comprehensive
- [x] Implementation order logical
- [x] Dependencies identified
- [x] Success metrics defined
- [x] Documentation requirements clear
- [x] Files to create/modify listed
- [x] Definition of Done complete
- [x] Technical specifications added
- [x] All error messages specified
- [x] Post-deployment verification added
- [x] Rollback procedure added
- [x] Security considerations addressed
- [x] All Round 1 feedback addressed

---

**Status**: Ready for Multi-Persona Review Round 2
**Version**: 2.0
**Last Updated**: December 7, 2025
