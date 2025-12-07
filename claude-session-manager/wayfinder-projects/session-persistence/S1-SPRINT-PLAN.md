# S1: Sprint 1 - Foundation & Core Infrastructure

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Awaiting Multi-Persona Approval
**Sprint Goal**: Implement foundational infrastructure for session persistence
**Prerequisites**:
- D1 Discovery ✅ Complete
- D2 Architecture ✅ Approved (8.8/10)
- D3 Implementation ✅ Approved (9.0/10)
- D4 Requirements ✅ Approved (9.3/10)

---

## Executive Summary

Sprint 1 focuses on implementing the **foundational infrastructure** needed for session persistence. This includes the manifest schema v2, migration logic, file locking, and core utilities.

**Scope**: 5 deliverables (of 11 total in Phase 3.5)
**Duration Estimate**: 2-3 days of focused development
**Dependencies**: None (foundational work)

**Strategic Rationale**: Build the foundation first. These components are dependencies for all other features. Without schema v2, migration, locking, and utilities, we cannot build resume auto-recreation, backup, or doctor commands.

---

## Sprint Goal

**Primary Goal**: Implement core infrastructure that enables safe, concurrent manifest operations with automatic v1→v2 migration.

**Success Criteria**:
1. ✅ All v1 manifests automatically migrate to v2 with backup
2. ✅ Concurrent operations are prevented by file locking
3. ✅ All edge cases handled (corrupted files, rollback, etc.)
4. ✅ 100% test coverage for critical paths
5. ✅ No data loss under any failure scenario

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
   - Lock timeout: 60 seconds
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

**Acceptance Criteria**:
- [ ] Writing with purpose > 256 chars returns validation error
- [ ] Writing with > 10 tags returns validation error
- [ ] Writing with tag > 32 chars returns validation error
- [ ] Tag with whitespace returns validation error
- [ ] Writing with notes > 1024 chars returns validation error
- [ ] Valid context at exact limits passes validation
- [ ] UTF-8 multibyte characters (emoji) counted correctly
- [ ] Error messages are clear and actionable

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
   - Lock file format: Line 1 = PID, Line 2 = RFC3339 timestamp
   - Use O_EXCL for atomic creation
   - Stale lock detection (60s timeout)
   - Auto-cleanup of stale locks

2. Lock lifecycle:
   - Acquire: Create `.lock` file with PID and timestamp
   - Release: Remove `.lock` file (use defer)
   - Stale detection: Check timestamp, remove if > 60s old

3. Error handling:
   - Lock conflict: Include PID of lock holder
   - Error format: "session is locked by process <PID> (try: kill <PID> or wait a minute)"
   - NFS limitation: Document in code comments

**Acceptance Criteria**:
- [ ] First process acquires lock successfully
- [ ] Second concurrent process gets lock error
- [ ] Lock file created: `manifest.yaml.lock`
- [ ] Lock file contains PID (line 1) and RFC3339 timestamp (line 2)
- [ ] Normal completion releases lock
- [ ] Error/panic releases lock via defer
- [ ] Stale lock (> 60s) removed automatically
- [ ] New lock acquired after stale removal
- [ ] Error message includes PID and remediation

**Tests**:
- `lock_test.go`: Acquire, release, concurrent access
- `lock_stale_test.go`: Stale lock detection and cleanup
- `lock_integration_test.go`: Real file system test

---

### D1.4: Schema Migration (FR-2)
**Priority**: P0 (Must Have)
**Estimated Effort**: 8 hours

**Tasks**:
1. Create `internal/manifest/migrate.go`:
   - `func migrateV1ToV2(manifestPath string) (*Manifest, error)`
   - Detect schema version (missing = v1)
   - Create backup: `manifest.yaml.v1.bak`
   - Validate: If backup exists, fail (prevents overwriting)
   - Parse v1 with strict YAML
   - Build v2 with atomic initialization
   - Validate v2 before writing
   - Rollback on failure (restore from backup)

2. Migration logging to `~/.csm/logs/migration.log`:
   - Log format: `[RFC3339] SUCCESS|FAILED: <path> [- <error>]`
   - Create log directory if doesn't exist
   - Append-only writes
   - Best-effort (don't fail if logging fails)

3. User messaging:
   - Check if stdout is TTY
   - If TTY: Show "📝 Migrating..." and "✅ Success"
   - If pipe: Silent (no stdout pollution)
   - One-time notice: `~/.csm/.migration-notice-shown`
   - Notice content: Schema v2 upgrade info, backup location

4. Atomic struct initialization:
   - Use temporary struct for parsing
   - Validate all required fields present
   - Build final Manifest only if all valid
   - No partial initialization

**Acceptance Criteria**:
- [ ] Loading v1 manifest triggers migration
- [ ] Backup created (.v1.bak) before migration
- [ ] V2 manifest written successfully
- [ ] Subsequent loads use v2 (no re-migration)
- [ ] If backup exists, migration fails (prevents overwrite)
- [ ] Write failure triggers rollback
- [ ] Original v1 restored from backup on failure
- [ ] Error message indicates rollback occurred
- [ ] Migration with missing required fields fails
- [ ] Migration with invalid types fails
- [ ] Migration with malformed YAML fails
- [ ] Successful migration logged with timestamp and path
- [ ] Failed migration logged with error details
- [ ] In terminal: migration message shown
- [ ] In pipe: no messages to stdout
- [ ] One-time notice shown on first migration per user

**Tests**:
- `migrate_test.go`: Happy path, validation
- `migrate_rollback_test.go`: Failure scenarios, rollback verification
- `migrate_edge_test.go`: Malformed YAML, partial data, backup collision

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

3. WriteAtomic implementation:
   - Create temp file: `<path>.tmp`
   - Write data to temp
   - Rename temp to final path (atomic on POSIX)
   - Remove temp on error

4. CopyDirectory implementation:
   - Recursive copy using filepath.Walk
   - Preserve permissions
   - Handle symlinks correctly

**Acceptance Criteria**:
- [ ] Valid copy succeeds
- [ ] Same src and dst returns error
- [ ] Source is directory returns error
- [ ] Permissions preserved in CopyFile
- [ ] WriteAtomic creates temp file
- [ ] WriteAtomic renames atomically
- [ ] On error, temp file removed
- [ ] CopyDirectory copies all files
- [ ] CopyDirectory copies all subdirectories
- [ ] CopyDirectory preserves permissions
- [ ] Symbolic links handled correctly

**Tests**:
- `fileutil_test.go`: All functions, edge cases, error handling

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

---

## Dependencies & Integration

### Prerequisites
- Go 1.19+ installed
- Existing CSM codebase (Phase 3)
- yaml.v3 library

### Integration Points
1. **Manifest loading** (`cmd/csm/list.go`, `cmd/csm/resume.go`):
   - All manifest reads go through migration check
   - Automatic v1→v2 upgrade on first load

2. **Manifest writing** (all commands that modify manifests):
   - All writes go through validation
   - All writes acquire lock first

3. **Lock lifecycle**:
   - Acquire at start of operation
   - Release via defer (even on panic)
   - Used by: resume, backup, archive (future)

---

## Testing Strategy

### Unit Tests (per-file)
- `manifest_test.go`: Schema structure, serialization
- `constants_test.go`: Constant values
- `validate_test.go`: All validation rules, boundary conditions
- `lock_test.go`: Lock lifecycle, concurrent access
- `lock_stale_test.go`: Stale lock detection
- `migrate_test.go`: Happy path, validation
- `migrate_rollback_test.go`: Failure scenarios
- `migrate_edge_test.go`: Edge cases
- `fileutil_test.go`: All utility functions

### Integration Tests (S1 scope)
- V1 manifest → load → auto-migrate → verify v2
- Concurrent lock attempts → one succeeds, one fails
- Migration failure → rollback → original restored
- Corrupted v1 → migration fails gracefully

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
- ✅ Stale lock timeout as fallback

### Risk 3: UTF-8 Character Counting Errors
**Probability**: LOW
**Impact**: LOW
**Mitigation**:
- ✅ Use unicode/utf8 package (not len())
- ✅ Test with emoji and multibyte characters
- ✅ Boundary condition tests (exactly N chars)

### Risk 4: Partial File Writes
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Atomic writes (temp + rename)
- ✅ Rollback on write failure
- ✅ No partial manifests ever created

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

### User Documentation (DR-1)
- [ ] Migration behavior documented
- [ ] Lock file behavior documented
- [ ] Validation limits documented

### Developer Documentation
- [ ] README updated with schema v2 info
- [ ] CHANGELOG entry for migration
- [ ] Migration guide (DR-2) - draft started

---

## Definition of Done

S1 is **DONE** when:

1. ✅ All 5 deliverables implemented and tested
2. ✅ All P0 acceptance criteria checked off
3. ✅ Test coverage >80% for critical paths
4. ✅ All tests passing (unit + integration)
5. ✅ Code documented (godoc comments)
6. ✅ Multi-persona review score ≥8.5/10
7. ✅ No known critical or high-severity bugs
8. ✅ Integration with existing CSM verified
9. ✅ Migration tested with real v1 manifests
10. ✅ All code committed and pushed

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
  ├── migrate_test.go      # NEW - Tests
  ├── migrate_rollback_test.go  # NEW - Tests
  └── migrate_edge_test.go # NEW - Tests

internal/fileutil/
  ├── fileutil.go          # NEW - Utility functions
  └── fileutil_test.go     # NEW - Tests
```

### Modified Files
```
internal/manifest/
  ├── manifest.go          # MODIFY - Add v2 schema fields
  └── manifest_test.go     # MODIFY - Add v2 tests
```

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

- [ ] All deliverables clearly defined
- [ ] All acceptance criteria listed
- [ ] All risks identified and mitigated
- [ ] Test strategy comprehensive
- [ ] Implementation order logical
- [ ] Dependencies identified
- [ ] Success metrics defined
- [ ] Documentation requirements clear
- [ ] Files to create/modify listed
- [ ] Definition of Done complete

---

**Status**: Ready for Multi-Persona Review Round 1
**Version**: 1.0
**Last Updated**: December 7, 2025
