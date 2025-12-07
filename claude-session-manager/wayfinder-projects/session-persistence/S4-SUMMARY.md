# S4 Implementation Plan Summary

**Date**: December 7, 2025
**Status**: ✅ APPROVED (9.5/10)
**Commit**: 7546b8c

---

## Executive Summary

S4 Implementation Plan has been **APPROVED** with a score of **9.5/10** after two review rounds.

The comprehensive implementation plan provides complete details for executing all 11 deliverables from Phase 3.5 Session Persistence across three sprints (S1, S2, S3).

---

## Review Results

### Round 1: 8.17/10 ❌ (Revision Needed)

**Critical gaps identified**:
- No prerequisites section (Go version, tmux dependency)
- No development workflow (how to test, build, debug)
- No CI/CD configuration
- No configuration management strategy
- No test fixtures with examples
- Migration concurrency not handled (race condition)
- Migration not idempotent (duplicate backups)
- No TmuxInterface for mocking
- Test execution commands missing
- Deployment verification not specified

### Round 2: 9.5/10 ✅ (APPROVED)

**All critical issues resolved**:

| Reviewer | R1 Score | R2 Score | Change |
|----------|----------|----------|--------|
| Senior Go Developer | 8.5/10 | 9.5/10 | +1.0 ⬆️ |
| Software Architect | 8.5/10 | 9.5/10 | +1.0 ⬆️ |
| QA Engineer | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| DevOps/SRE | 7.5/10 | 9.5/10 | +2.0 ⬆️ |
| End User | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| Security Engineer | 8.5/10 | 9.5/10 | +1.0 ⬆️ |

**Average**: 9.5/10 ✅ **EXCEEDS THRESHOLD (8.5/10)**

---

## S4 Scope

**Goal**: Implement all 11 deliverables from Phase 3.5 Session Persistence

**Sprint Breakdown**:

**Sprint 1 - Foundation** (2-3 days, 5 deliverables):
1. D1.1: Manifest Schema v2 (6h)
2. D1.2: Context Validation (3h)
3. D1.3: File Locking (6h)
4. D1.4: Migration v1 → v2 (8h)
5. D1.5: Fileutil Package (3h)

**Sprint 2 - User Features** (2-3 days, 3 deliverables):
6. D2.1: Status Computation (6h)
7. D2.2: Enhanced Resume (8h)
8. D2.3: Backup Command (6h)

**Sprint 3 - Operations** (2-3 days, 3 deliverables):
9. D3.1: Doctor Command (10h)
10. D3.2: Log Rotation (5h)
11. D3.3: Integration & Performance Testing (10h)

**Total**: 71 hours estimated (6-9 days)

---

## Key Sections Added in v2

### 1. Prerequisites

**System Requirements**:
- Go 1.21 or later
- Tmux 3.0 or later
- POSIX filesystem (macOS, Linux)

**Dependencies**:
```go
require (
    gopkg.in/yaml.v3 v3.0.1
)
```

**Verification commands**:
```bash
go version  # go1.21+
tmux -V     # tmux 3.0+
ls ~/sessions  # exists
```

### 2. Development Workflow

**Local development**:
```bash
# Run tests
go test ./...                    # All tests
go test ./... -short -timeout=2m # Fast only
go test ./internal/manifest -v   # Specific package

# Build
go build -o csm ./cmd/csm

# Run without installing
go run ./cmd/csm list

# Debug with delve
dlv debug ./cmd/csm -- resume claude-1
```

**Code quality**:
```bash
gofmt -w .              # Format
go vet ./...            # Vet
go test ./... -cover    # Coverage
```

### 3. CI/CD Configuration

**GitHub Actions** (`.github/workflows/test.yml`):
- Fast tests on PR (< 2 min)
- Slow tests on main (nightly)
- Build verification
- Format checking

**Makefile targets**:
```makefile
make build           # Build binary
make test-short      # Fast tests
make test-coverage   # Coverage report
make install         # Install to ~/bin
make lint            # Format + vet
make smoke-test      # Deployment verification
```

**Deployment verification** (`scripts/verify-deployment.sh`):
```bash
# Check binary exists
# Check version
# Test list command
# Test doctor command
# Verify directories
```

### 4. Configuration Management

**Config file** (`~/.csmrc`):
```yaml
claude:
  history_path: "~/.claude/history.jsonl"
  session_env_dir: "~/.claude/session-env"
  file_history_dir: "~/.claude/file-history"

csm:
  sessions_dir: "~/sessions"
  logs_dir: "~/.csm/logs"

timeouts:
  lock_timeout: 60
  max_backups_per_session: 10

logging:
  rotation_size_mb: 10
  rotation_keep_count: 5
```

**Config loading** (`internal/config/config.go`):
- Defaults set if no ~/.csmrc
- Tilde expansion (~/ → /home/user/)
- Global config instance
- Type-safe struct

### 5. Test Fixtures & Examples

**Sample manifests**:
- `v1-simple.yaml` - Basic v1
- `v2-complete.yaml` - All v2 fields
- `v2-archived.yaml` - Archived session
- `invalid-yaml.yaml` - Malformed
- `empty.yaml` - Edge case

**Sample history**:
- `simple.jsonl` - 10 messages
- `medium.jsonl` - 200 messages
- `large.jsonl` - 1000+ messages
- `malformed.jsonl` - Mix valid/invalid

**Mock tmux**:
```go
type TmuxInterface interface {
    HasSession(name string) (bool, error)
    ListSessions() ([]string, error)
    CreateSession(name, dir string) error
    SendKeys(name, keys string) error
    AttachSession(name string) error
}

type MockTmux struct {
    Sessions map[string]bool
}
```

---

## Critical Code Improvements

### Migration Concurrency Fix

**Problem**: Two processes could migrate same v1 manifest simultaneously

**Solution**: Acquire lock before migration

```go
func MigrateV1ToV2(path string) error {
    // CRITICAL: Acquire lock BEFORE migration
    if err := AcquireLock(path); err != nil {
        return fmt.Errorf("cannot acquire lock for migration: %w", err)
    }
    defer ReleaseLock(path)

    // ... migration logic ...
}
```

### Migration Idempotency

**Problem**: Multiple migrations create duplicate backups

**Solution**: Check if .v1.bak exists

```go
// Check if backup already exists
backupPath := path + ".v1.bak"
if _, err := os.Stat(backupPath); err == nil {
    // Backup exists, migration already done
    logMigration("SKIPPED", path, errors.New("backup already exists"))
    return nil
}
```

**Retry handling**:
```go
// Save v2
if err := Save(v2, path); err != nil {
    // Migration failed, remove backup to allow retry
    os.Remove(backupPath)
    logMigration("FAILED", path, err)
    return fmt.Errorf("failed to save v2: %w", err)
}
```

### TmuxInterface Abstraction

**Problem**: Hard to test without real tmux

**Solution**: Interface abstraction

```go
// Define interface
type TmuxInterface interface {
    HasSession(name string) (bool, error)
    // ...
}

// Production uses real tmux
type RealTmux struct{}

// Tests use mock
type MockTmux struct {
    Sessions map[string]bool
}

// Global variable (swappable in tests)
var tmux TmuxInterface = &RealTmux{}
```

**Usage in code**:
```go
func ComputeStatus(m *manifest.Manifest, tmux TmuxInterface) string {
    if m.Lifecycle == manifest.LifecycleArchived {
        return "archived"
    }

    exists, _ := tmux.HasSession(m.Tmux.SessionName)
    if exists {
        return "active"
    }
    return "stopped"
}
```

**Usage in tests**:
```go
func TestComputeStatus(t *testing.T) {
    mockTmux := &MockTmux{Sessions: map[string]bool{"claude-test": true}}

    m := &manifest.Manifest{Lifecycle: "", Tmux: manifest.Tmux{SessionName: "claude-test"}}

    status := ComputeStatus(m, mockTmux)
    assert.Equal(t, "active", status)
}
```

---

## Implementation Timeline

**Week 1**:
- Day 1-2: Sprint 1 (Foundation)
  - Manifest schema v2, validation, locking, migration, fileutil
  - Sprint 1 review
- Day 3: Sprint 2 Start
  - Status computation, enhanced resume (partial)

**Week 2**:
- Day 4: Sprint 2 Complete
  - Enhanced resume (complete), backup command
  - Sprint 2 review
- Day 5-6: Sprint 3 Start
  - Doctor command, log rotation
- Day 7-8: Sprint 3 Complete
  - Integration & performance testing
  - Sprint 3 review, final documentation

**Contingency**:
- Day 9: Buffer for issues, testing, polish

---

## Success Criteria

S4 Implementation is **DONE** when:

### Functional
- ✅ All 11 deliverables implemented
- ✅ All 127 acceptance criteria met
- ✅ All features working as specified

### Quality
- ✅ All unit tests passing
- ✅ All integration tests passing (15 scenarios)
- ✅ All performance benchmarks meeting targets (9 benchmarks)
- ✅ Test coverage >80% critical, >60% overall
- ✅ Zero critical bugs
- ✅ Multi-persona review ≥8.5/10 for each sprint

### Documentation
- ✅ Godoc comments on all exported functions
- ✅ Inline comments for complex logic
- ✅ User documentation (help text, examples)
- ✅ Developer documentation (README, CHANGELOG)

### Deployment
- ✅ All code committed to git
- ✅ Post-deployment verification passed
- ✅ Rollback procedure tested

---

## Major Improvements from v1 to v2

### Documentation Additions (+5 sections)

1. **Prerequisites** (NEW):
   - System requirements (Go 1.21+, tmux 3.0+)
   - Dependency management (go.mod)
   - Verification commands

2. **Development Workflow** (NEW):
   - Local setup (clone, dependencies)
   - Test execution (unit, integration, benchmarks)
   - Build commands (binary, with version)
   - Debugging (delve, logging)
   - Quality checks (format, vet, coverage)

3. **CI/CD Configuration** (NEW):
   - GitHub Actions workflow (fast/slow separation)
   - Makefile (10 targets)
   - Deployment verification script
   - Smoke test suite

4. **Configuration Management** (NEW):
   - ~/.csmrc file format
   - Configuration struct (config.go)
   - Default values
   - Path expansion logic

5. **Test Fixtures & Examples** (NEW):
   - 6 sample manifest files
   - 4 sample history files
   - TmuxInterface definition
   - MockTmux implementation
   - 10+ test execution commands

### Code Improvements (+3 critical fixes)

6. **Migration Concurrency**:
   - Lock before migrate (prevents race)
   - Test for concurrent migration
   - Safe for multiple processes

7. **Migration Idempotency**:
   - Check .v1.bak exists
   - Skip if already migrated
   - Remove backup on failed save (retry)
   - Test for idempotency

8. **TmuxInterface Abstraction**:
   - Interface definition
   - RealTmux for production
   - MockTmux for testing
   - Status/resume use interface

### Clarifications (+2 important details)

9. **Doctor Fix Confirmation**:
   - No interactive confirmation (automation-friendly)
   - Use --dry-run for preview
   - 60s threshold protects active locks
   - Rationale documented

10. **Status Computation Location**:
    - Moved to internal/session/ (reusable)
    - cmd layer calls it
    - Interface-based (testable)

---

## Files Created

- `S4-IMPLEMENTATION.md` (v1, 8.17/10)
- `S4-IMPLEMENTATION-v2.md` (v2, 9.5/10) ✅
- `S4-REVIEW-R1.md` (6 personas, detailed feedback)
- `S4-REVIEW-R2.md` (6 personas, final approval)
- `S4-SUMMARY.md` (this document)

**Commits**:
- `7546b8c` - S4 implementation plan and reviews

---

## Wayfinder Progress

- ✅ **D1 Discovery** - Research complete
- ✅ **D2 Architecture** - Approved 8.8/10
- ✅ **D3 Implementation Design** - Approved 9.0/10
- ✅ **D4 Requirements** - Approved 9.3/10
- ✅ **S1 Sprint Plan** - Approved 9.4/10
- ✅ **S2 Sprint Plan** - Approved 9.5/10
- ✅ **S3 Sprint Plan** - Approved 9.5/10
- ✅ **S4 Implementation Plan** - Approved 9.5/10 ← **CURRENT**
- ⏸️ **Implementation Execution** - Awaiting your approval to begin

---

## Next Steps

**I'm now paused per Wayfinder methodology.**

You can:
1. **Approve and begin implementation** - Start coding the 11 deliverables (6-9 days)
2. **Review implementation plan** - Examine S4-IMPLEMENTATION-v2.md and suggest changes
3. **Implementation order** - Confirm S1 → S2 → S3 or choose different order
4. **Different task** - Work on something else

All work is committed and ready at `~/src/repos/ai-tools/base/claude-session-manager/wayfinder-projects/session-persistence/`.

---

**End of S4 Summary**
