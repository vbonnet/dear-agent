# S6 Sprint 2 Implementation - Current State Analysis

**Date**: December 7, 2025
**Phase**: S6 Sprint 2 Implementation (Enhanced Resume & Backup)
**Status**: 🔍 ANALYSIS IN PROGRESS

---

## Sprint 2 Deliverables

From S4-IMPLEMENTATION-v2.md:

**D2.1: Status Computation** (6 hours estimated):
- Create `internal/session/status.go`
- Implement `ComputeStatus()` with TmuxInterface abstraction
- Implement `ComputeStatusBatch()` for efficient bulk operations
- Write tests with mock tmux

**D2.2: Enhanced Resume (Auto-Recreation)** (8 hours estimated):
- Enhance `cmd/csm/resume.go` to auto-recreate stopped sessions
- Implement `recreateTmuxSession()` with TmuxInterface
- Restore tmux environment (working directory)
- Send `claude --resume` command to restored session

**D2.3: Backup Management** (7 hours estimated):
- Implement manifest backup rotation (max 10 per session)
- Implement file history backup rotation
- Add backup operations to `csm backup` command
- Add restoration testing

**Total Estimated**: 21 hours

---

## Current Codebase Analysis

### Existing Code (from previous work)

**Manifest Package** (Sprint 1 - Complete):
```
internal/manifest/
├── constants.go           ✅ Schema v2 constants
├── manifest.go            ✅ v2 structs (Manifest, Context)
├── validate.go            ✅ v2 validation
├── validate_test.go       ✅ v2 tests (19 passing)
├── lock.go                ✅ File locking
├── lock_test.go           ✅ Lock tests
├── migrate.go             ✅ v1 → v2 migration
├── migrate_test.go        ✅ Migration tests
├── read.go                ✅ Auto-migration on load
├── write.go               ✅ Atomic writes
└── testutil.go            ✅ Test helpers
```

**Fileutil Package** (Sprint 1 - Complete):
```
internal/fileutil/
├── atomic.go              ✅ Atomic file writes
└── atomic_test.go         ✅ Atomic write tests
```

**CLI Commands** (Existing, need investigation):
```
cmd/csm/
├── main.go                ? Need to check current state
├── list.go                ? May need updating for v2
├── resume.go              ? May exist, needs enhancement
├── archive.go             ? Need to check
└── ...                    ? Other commands
```

### What Needs to Be Created

**Session Package** (NEW for Sprint 2):
```
internal/session/
├── status.go              ❌ Status computation logic
├── status_test.go         ❌ Status tests with mock tmux
├── tmux.go                ❌ TmuxInterface definition + real implementation
└── tmux_mock.go           ❌ Mock tmux for testing
```

**Backup Package** (NEW for Sprint 2):
```
internal/backup/
├── rotation.go            ❌ Backup rotation logic
├── rotation_test.go       ❌ Rotation tests
└── restore.go             ❌ Restoration logic (optional for Sprint 2)
```

**CLI Updates** (ENHANCE for Sprint 2):
```
cmd/csm/
├── list.go                ? Update to use session.ComputeStatusBatch()
├── resume.go              ? Update for auto-recreation
├── backup.go              ❌ NEW - backup management command
└── ...
```

---

## Dependency Analysis

### External Dependencies

**Already in go.mod** (from Sprint 1):
```
gopkg.in/yaml.v3 v3.0.1
```

**Need to verify**:
- `github.com/stretchr/testify` - Used in existing tests
- Any tmux Go libraries? (likely none - will shell out to `tmux` command)

### System Dependencies

**Required**:
- `tmux` 3.0+ (already verified in S5)
- POSIX shell for tmux commands

---

## Tmux Interface Strategy

From S4-IMPLEMENTATION-v2.md, Sprint 2 introduces **TmuxInterface** abstraction for testability.

### Why TmuxInterface?

**Problem**: Direct `exec.Command("tmux", ...)` calls are:
- Untestable (require real tmux running)
- Slow in CI (need tmux installation)
- Flaky (tmux state pollution between tests)

**Solution**: Interface abstraction
```go
type TmuxInterface interface {
    HasSession(name string) (bool, error)
    ListSessions() ([]string, error)
    CreateSession(name, workdir string) error
    AttachSession(name string) error
    SendKeys(session, keys string) error
}
```

### Implementation Plan

1. **Real implementation** (`internal/session/tmux.go`):
   - Wraps `exec.Command("tmux", ...)` calls
   - Used in production

2. **Mock implementation** (`internal/session/tmux_mock.go`):
   - In-memory session tracking
   - Used in tests

3. **Integration**:
   - CLI commands receive TmuxInterface via dependency injection
   - Tests pass mock, production passes real implementation

---

## Code That Can Be Reused

### From Sprint 1

**Atomic Operations**:
```go
// internal/fileutil/atomic.go - Already implemented
func AtomicWrite(path string, data []byte, perm os.FileMode) error
```

**Manifest Loading**:
```go
// internal/manifest/read.go - Already implemented
func Read(path string) (*Manifest, error)  // Auto-migrates v1 → v2
```

**Manifest Writing**:
```go
// internal/manifest/write.go - Already implemented
func Write(path string, m *Manifest) error  // Sets UpdatedAt, validates
```

### From Existing CLI (Need to verify)

**Manifest discovery**:
- Likely exists in `cmd/csm/` - need to check how manifests are currently found
- May need to refactor into `internal/session/discovery.go`

---

## Files to Investigate

Before starting implementation, need to read:

1. **cmd/csm/main.go** - Current CLI structure, command registration
2. **cmd/csm/list.go** - How manifests are currently loaded and displayed
3. **cmd/csm/resume.go** - Current resume implementation (if exists)
4. **cmd/csm/archive.go** - Archive command structure (if exists)
5. **go.mod** - Verify all dependencies

---

## Expected Directory Structure After Sprint 2

```
~/src/repos/ai-tools/base/claude-session-manager/
├── cmd/
│   └── csm/
│       ├── main.go           ? (verify structure)
│       ├── list.go            ? (update for status computation)
│       ├── resume.go          ? (enhance for auto-recreation)
│       ├── backup.go          ❌ NEW
│       └── ...
├── internal/
│   ├── manifest/              ✅ (Sprint 1 - complete)
│   ├── fileutil/              ✅ (Sprint 1 - complete)
│   ├── session/               ❌ NEW (Sprint 2)
│   │   ├── status.go
│   │   ├── status_test.go
│   │   ├── tmux.go
│   │   └── tmux_mock.go
│   └── backup/                ❌ NEW (Sprint 2)
│       ├── rotation.go
│       └── rotation_test.go
├── go.mod
└── go.sum
```

---

## Acceptance Criteria Summary

### D2.1: Status Computation (15 criteria from S2-SPRINT-PLAN-v2.md + 2 new)

1. [ ] `ComputeStatus()` returns "active", "stopped", or "archived"
2. [ ] Archived manifests always return "archived"
3. [ ] Active tmux sessions return "active"
4. [ ] Stopped sessions (no tmux) return "stopped"
5. [ ] `ComputeStatusBatch()` for efficient bulk operations
6. [ ] Batch operation makes single `tmux list-sessions` call
7. [ ] Error handling for tmux failures (assume stopped)
8. [ ] Tests with mock tmux (no real tmux required)
9. [ ] Status computation uses TmuxInterface
10. [ ] Tests cover all status transitions
11. [ ] Tests verify batch optimization (single tmux call)
12. [ ] Error cases tested (tmux unavailable, permission denied)
13. [ ] Integration with `csm list` command
14. [ ] Status display in list output
15. [ ] Performance: batch < 100ms for 100 sessions
16. [ ] **NEW**: Status computation uses TmuxInterface
17. [ ] **NEW**: Tests use mock tmux (no real tmux needed)

### D2.2: Enhanced Resume (20 criteria from S2-SPRINT-PLAN-v2.md)

1. [ ] `csm resume <identifier>` accepts session ID or name
2. [ ] Auto-detects if tmux session exists
3. [ ] If active: attaches to existing session
4. [ ] If stopped: recreates tmux session with same name
5. [ ] Recreation restores working directory from Context.Project
6. [ ] Recreation sends `claude --resume <session-id>` command
7. [ ] Sanitizes session names (no special chars)
8. [ ] Error handling for tmux creation failures
9. [ ] Tests with mock tmux
10. [ ] Tests verify recreation workflow
11. [ ] Tests verify attach workflow
12. [ ] Integration test (if possible without real tmux)
13. [ ] User feedback messages (recreating vs attaching)
14. [ ] Archived sessions cannot be resumed (error message)
15. [ ] TmuxInterface abstraction used throughout
16. [ ] Mock tests don't require real tmux
17. [ ] Error messages are helpful (suggest alternatives)
18. [ ] Session name collisions handled gracefully
19. [ ] Working directory validation (exists, readable)
20. [ ] Claude resume command properly formatted

### D2.3: Backup Management (18 criteria from S2-SPRINT-PLAN-v2.md)

1. [ ] Manifest backup on update (manifest.yaml.1, .2, etc.)
2. [ ] Rotation limit: max 10 backups per session
3. [ ] Oldest backups deleted when limit exceeded
4. [ ] File history backups (copy to ~/.csm/backups/<session-id>/)
5. [ ] File history rotation: max 10 per session
6. [ ] `csm backup list <identifier>` shows available backups
7. [ ] `csm backup restore <identifier> <backup-number>` restores manifest
8. [ ] Restoration is atomic (backup → temp → atomic rename)
9. [ ] Restoration creates new backup of current state first
10. [ ] Tests for rotation logic
11. [ ] Tests for restoration
12. [ ] Backup numbering is sequential and consistent
13. [ ] Backup metadata (timestamp, reason for backup)
14. [ ] Error handling for backup failures
15. [ ] Disk space checks before backup creation
16. [ ] Backup directory structure documented
17. [ ] Integration with write operations (auto-backup on manifest update)
18. [ ] Performance: backup creation < 50ms

**Total**: 55 acceptance criteria

---

## Implementation Strategy

### Phase 1: Investigation (Current)

1. ✅ Read S4-IMPLEMENTATION-v2.md Sprint 2 section
2. ⏭️ Read existing cmd/csm/ files to understand current structure
3. ⏭️ Verify go.mod dependencies
4. ⏭️ Document current CLI command structure

### Phase 2: Foundation

1. Create `internal/session/` package with TmuxInterface
2. Implement real tmux wrapper
3. Implement mock tmux for tests
4. Write interface tests

### Phase 3: D2.1 Status Computation

1. Implement `status.go` with ComputeStatus/ComputeStatusBatch
2. Write comprehensive tests with mock tmux
3. Update `cmd/csm/list.go` to use new status computation
4. Verify all tests passing

### Phase 4: D2.2 Enhanced Resume

1. Enhance `cmd/csm/resume.go` for auto-recreation
2. Implement `recreateTmuxSession()`
3. Write tests with mock tmux
4. Verify all tests passing

### Phase 5: D2.3 Backup Management

1. Create `internal/backup/rotation.go`
2. Implement manifest backup rotation
3. Implement file history backup
4. Create `cmd/csm/backup.go` command
5. Write tests
6. Verify all tests passing

### Phase 6: Integration & Review

1. Run full test suite
2. Integration testing (manual)
3. Update documentation
4. Multi-persona review

---

## Next Steps

1. ⏭️ Read existing CLI files (main.go, list.go, resume.go, archive.go)
2. ⏭️ Document current command structure and manifest loading
3. ⏭️ Create implementation plan for Sprint 2
4. ⏭️ Begin implementation with TmuxInterface foundation

---

**Status**: 🔍 ANALYSIS IN PROGRESS - Need to investigate existing CLI code
