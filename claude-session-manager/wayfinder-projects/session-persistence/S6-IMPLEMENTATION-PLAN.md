# S6 Sprint 2 Implementation Plan

**Date**: December 7, 2025
**Phase**: S6 Sprint 2 Implementation (Enhanced Resume & Backup)
**Status**: 📋 READY FOR IMPLEMENTATION
**Prerequisites**: ✅ Sprint 1 (S5) Complete and Approved (9.16/10)

---

## Current State Assessment

### ✅ Existing Infrastructure (From Sprint 1)

**Manifest Package** - v2 schema complete:
- `internal/manifest/` - All v2 functionality (schema, validation, migration, locking)
- 19 tests passing, all critical paths covered
- Auto-migration v1 → v2 on Read()

**Fileutil Package** - Atomic operations:
- `internal/fileutil/atomic.go` - Atomic writes using temp + rename

**Tmux Package** - Real implementation exists:
- `internal/tmux/tmux.go` - Real tmux wrapper (HasSession, NewSession, AttachSession, SendCommand, ListSessions)
- ⚠️ No interface abstraction yet (direct functions, not interface-based)
- ⚠️ No mock implementation for testing

**Session Package** - Partial implementation:
- `internal/session/session.go` - ResolveIdentifier(), CheckHealth()
- ⚠️ Uses v1 manifest structure (needs updating for v2)
- ❌ No status computation yet

**CLI Commands** - Existing but needs updating:
- `cmd/csm/` - Cobra-based CLI with list, resume, new, sync, doctor, version commands
- ⚠️ Uses v1 manifest structure (needs updating for v2)

### ❌ What Sprint 2 Needs to Add

**Status Computation**:
- ❌ `internal/session/status.go` - ComputeStatus(), ComputeStatusBatch()
- ❌ Status tests with mock tmux

**TmuxInterface Abstraction**:
- ❌ `internal/session/tmux_interface.go` - Interface definition
- ❌ `internal/session/tmux_real.go` - Real tmux implementation (wraps internal/tmux)
- ❌ `internal/session/tmux_mock.go` - Mock tmux for testing

**Backup Management**:
- ❌ `internal/backup/rotation.go` - Backup rotation logic
- ❌ `internal/backup/rotation_test.go` - Tests
- ❌ `cmd/csm/backup.go` - Backup CLI command

**Updates to Existing Code**:
- ⚠️ `internal/session/session.go` - Update for v2 manifest (Context.Project, SessionID only)
- ⚠️ `cmd/csm/list.go` - Use session.ComputeStatusBatch()
- ⚠️ `cmd/csm/resume.go` - Add auto-recreation logic

---

## Implementation Strategy

### Decision: Refactor vs Wrapper Approach

**Option 1: Refactor internal/tmux to use interface** (Breaking change)
- Change internal/tmux from package-level functions to interface + implementation
- Pro: Clean architecture
- Con: Breaks existing code (list.go, resume.go already use internal/tmux)

**Option 2: Create wrapper in internal/session** (Additive)
- Keep internal/tmux as-is (package-level functions)
- Create TmuxInterface in internal/session that wraps internal/tmux
- Pro: No breaking changes, backward compatible
- Con: Two layers (internal/tmux and internal/session wrapper)

**DECISION**: **Option 2 - Wrapper approach**

**Rationale**:
- Existing code (list.go, resume.go) already uses `internal/tmux` package functions
- Don't want to break working code
- Can incrementally migrate to interface-based approach
- Sprint 2 only needs interface for NEW code (status computation, auto-recreation)

### Package Structure (Final)

```
internal/
├── tmux/                   ✅ Keep as-is (real tmux wrapper)
│   └── tmux.go
├── session/                ? Update and extend
│   ├── session.go          ? Update for v2 (ResolveIdentifier, CheckHealth)
│   ├── tmux_interface.go   ❌ NEW - Interface definition
│   ├── tmux_real.go        ❌ NEW - Wraps internal/tmux
│   ├── tmux_mock.go        ❌ NEW - Mock for testing
│   ├── status.go           ❌ NEW - Status computation
│   └── status_test.go      ❌ NEW - Status tests
├── backup/                 ❌ NEW - Backup management
│   ├── rotation.go
│   └── rotation_test.go
└── manifest/               ✅ Complete from Sprint 1
```

---

## Sprint 2 Deliverables (Detailed)

### D2.1: Status Computation (6 hours)

**Files to Create**:
1. `internal/session/tmux_interface.go` - Interface definition
2. `internal/session/tmux_real.go` - Real implementation (wraps internal/tmux)
3. `internal/session/tmux_mock.go` - Mock for testing
4. `internal/session/status.go` - Status computation logic
5. `internal/session/status_test.go` - Tests

**Files to Update**:
1. `cmd/csm/list.go` - Use session.ComputeStatusBatch() for status display

**Implementation**:

```go
// internal/session/tmux_interface.go
package session

type TmuxInterface interface {
    HasSession(name string) (bool, error)
    ListSessions() ([]string, error)
    CreateSession(name, workdir string) error
    AttachSession(name string) error
    SendKeys(session, keys string) error
}

// internal/session/tmux_real.go
package session

import "github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"

type RealTmux struct{}

func NewRealTmux() *RealTmux {
    return &RealTmux{}
}

func (t *RealTmux) HasSession(name string) (bool, error) {
    return tmux.HasSession(name)
}

func (t *RealTmux) ListSessions() ([]string, error) {
    return tmux.ListSessions()
}

func (t *RealTmux) CreateSession(name, workdir string) error {
    return tmux.NewSession(name, workdir)
}

func (t *RealTmux) AttachSession(name string) error {
    return tmux.AttachSession(name)
}

func (t *RealTmux) SendKeys(session, keys string) error {
    return tmux.SendCommand(session, keys)
}

// internal/session/tmux_mock.go
package session

type MockTmux struct {
    Sessions       map[string]bool  // session name -> exists
    CreatedSessions []string         // track creation order
    SentCommands   []string         // track sent commands
}

func NewMockTmux() *MockTmux {
    return &MockTmux{
        Sessions:       make(map[string]bool),
        CreatedSessions: []string{},
        SentCommands:   []string{},
    }
}

func (m *MockTmux) HasSession(name string) (bool, error) {
    exists, ok := m.Sessions[name]
    if !ok {
        return false, nil
    }
    return exists, nil
}

func (m *MockTmux) ListSessions() ([]string, error) {
    sessions := []string{}
    for name, exists := range m.Sessions {
        if exists {
            sessions = append(sessions, name)
        }
    }
    return sessions, nil
}

func (m *MockTmux) CreateSession(name, workdir string) error {
    m.Sessions[name] = true
    m.CreatedSessions = append(m.CreatedSessions, name)
    return nil
}

func (m *MockTmux) AttachSession(name string) error {
    // No-op in mock
    return nil
}

func (m *MockTmux) SendKeys(session, keys string) error {
    m.SentCommands = append(m.SentCommands, keys)
    return nil
}

// internal/session/status.go
package session

import "github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"

func ComputeStatus(m *manifest.Manifest, tmux TmuxInterface) string {
    // Check lifecycle first
    if m.Lifecycle == manifest.LifecycleArchived {
        return "archived"
    }

    // Check tmux state
    exists, err := tmux.HasSession(m.Tmux.SessionName)
    if err != nil {
        // Tmux error, assume stopped
        return "stopped"
    }

    if exists {
        return "active"
    }

    return "stopped"
}

func ComputeStatusBatch(manifests []*manifest.Manifest, tmux TmuxInterface) map[string]string {
    statuses := make(map[string]string)

    // Get all tmux sessions in one call (optimization)
    existingSessions, err := tmux.ListSessions()
    if err != nil {
        existingSessions = []string{}  // Assume no sessions on error
    }

    sessionSet := make(map[string]bool)
    for _, name := range existingSessions {
        sessionSet[name] = true
    }

    // Compute status for each manifest
    for _, m := range manifests {
        if m.Lifecycle == manifest.LifecycleArchived {
            statuses[m.Name] = "archived"
        } else if sessionSet[m.Tmux.SessionName] {
            statuses[m.Name] = "active"
        } else {
            statuses[m.Name] = "stopped"
        }
    }

    return statuses
}
```

**Acceptance Criteria** (17 total):
- [ ] TmuxInterface defined with all required methods
- [ ] RealTmux wraps internal/tmux functions correctly
- [ ] MockTmux provides in-memory session tracking
- [ ] ComputeStatus() returns correct status for each case
- [ ] Archived manifests always return "archived"
- [ ] Active tmux sessions return "active"
- [ ] Stopped sessions return "stopped"
- [ ] ComputeStatusBatch() optimizes with single ListSessions() call
- [ ] Error handling for tmux failures (assume stopped)
- [ ] Tests use MockTmux (no real tmux required)
- [ ] Tests cover all status transitions
- [ ] Tests verify batch optimization
- [ ] Error cases tested (tmux unavailable)
- [ ] Integration with `csm list` command
- [ ] Status display in list output
- [ ] Performance: batch < 100ms for 100 sessions
- [ ] All tests passing

### D2.2: Enhanced Resume (8 hours)

**Files to Update**:
1. `cmd/csm/resume.go` - Add auto-recreation logic
2. `internal/session/resume.go` - If exists, update for v2

**Implementation**:

```go
// cmd/csm/resume.go (update RunE function)
func runResume(identifier string) error {
    // Resolve identifier to manifest
    m, manifestPath, err := session.ResolveIdentifier(identifier, cfg.SessionsDir)
    if err != nil {
        return fmt.Errorf("failed to resolve session: %w", err)
    }

    // Check if archived
    if m.Lifecycle == manifest.LifecycleArchived {
        return fmt.Errorf("session is archived (use 'csm unarchive' first)")
    }

    // Check if tmux session exists
    tmuxClient := session.NewRealTmux()
    exists, err := tmuxClient.HasSession(m.Tmux.SessionName)
    if err != nil {
        return fmt.Errorf("failed to check tmux session: %w", err)
    }

    if !exists {
        // Auto-recreate
        fmt.Printf("Session stopped. Recreating tmux session '%s'...\\n", m.Tmux.SessionName)

        if err := recreateSession(m, tmuxClient); err != nil {
            return fmt.Errorf("failed to recreate session: %w", err)
        }

        fmt.Println("Session recreated successfully.")
    }

    // Attach to session
    return tmuxClient.AttachSession(m.Tmux.SessionName)
}

func recreateSession(m *manifest.Manifest, tmux session.TmuxInterface) error {
    // Validate working directory exists
    if _, err := os.Stat(m.Context.Project); err != nil {
        return fmt.Errorf("working directory does not exist: %s", m.Context.Project)
    }

    // Create tmux session
    if err := tmux.CreateSession(m.Tmux.SessionName, m.Context.Project); err != nil {
        return fmt.Errorf("tmux create failed: %w", err)
    }

    // Send claude resume command
    resumeCmd := fmt.Sprintf("claude --resume %s", m.SessionID)
    if err := tmux.SendKeys(m.Tmux.SessionName, resumeCmd); err != nil {
        return fmt.Errorf("failed to send claude resume command: %w", err)
    }

    return nil
}
```

**Acceptance Criteria** (20 total):
- [ ] `csm resume <identifier>` resolves session ID, name, or UUID
- [ ] Auto-detects if tmux session exists (uses TmuxInterface)
- [ ] If active: attaches to existing session
- [ ] If stopped: recreates tmux session
- [ ] Recreation restores working directory from Context.Project
- [ ] Recreation sends `claude --resume <session-id>` command
- [ ] Error handling for missing working directory
- [ ] Error handling for tmux creation failures
- [ ] Archived sessions cannot be resumed (error message)
- [ ] Tests with MockTmux
- [ ] Tests verify recreation workflow
- [ ] Tests verify attach workflow
- [ ] User feedback messages (recreating vs attaching)
- [ ] Tests verify Claude resume command sent correctly
- [ ] Tests verify working directory validation
- [ ] Tests verify archived session rejection
- [ ] Session name used correctly
- [ ] Integration test (if possible)
- [ ] Error messages are helpful
- [ ] All tests passing

### D2.3: Backup Management (7 hours)

**Files to Create**:
1. `internal/backup/rotation.go` - Backup rotation logic
2. `internal/backup/rotation_test.go` - Tests
3. `cmd/csm/backup.go` - CLI command for backup operations

**Implementation**:

```go
// internal/backup/rotation.go
package backup

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"

    "github.com/vbonnet/ai-tools/claude-session-manager/internal/fileutil"
)

const MaxBackups = 10

// CreateBackup creates a numbered backup of a file
// Returns backup number created
func CreateBackup(sourcePath string) (int, error) {
    // Find next backup number
    backups, err := ListBackups(sourcePath)
    if err != nil {
        return 0, err
    }

    nextNum := 1
    if len(backups) > 0 {
        nextNum = backups[len(backups)-1] + 1
    }

    // Create backup
    backupPath := fmt.Sprintf("%s.%d", sourcePath, nextNum)
    data, err := os.ReadFile(sourcePath)
    if err != nil {
        return 0, fmt.Errorf("failed to read source: %w", err)
    }

    if err := fileutil.AtomicWrite(backupPath, data, 0600); err != nil {
        return 0, fmt.Errorf("failed to write backup: %w", err)
    }

    // Rotate if needed
    if err := RotateBackups(sourcePath, MaxBackups); err != nil {
        return nextNum, fmt.Errorf("backup created but rotation failed: %w", err)
    }

    return nextNum, nil
}

// ListBackups returns sorted list of backup numbers for a file
func ListBackups(sourcePath string) ([]int, error) {
    dir := filepath.Dir(sourcePath)
    base := filepath.Base(sourcePath)

    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, fmt.Errorf("failed to read directory: %w", err)
    }

    backups := []int{}
    for _, entry := range entries {
        name := entry.Name()
        // Match pattern: manifest.yaml.1, manifest.yaml.2, etc.
        if strings.HasPrefix(name, base+".") {
            numStr := strings.TrimPrefix(name, base+".")
            num, err := strconv.Atoi(numStr)
            if err == nil {
                backups = append(backups, num)
            }
        }
    }

    sort.Ints(backups)
    return backups, nil
}

// RotateBackups deletes oldest backups if count exceeds maxBackups
func RotateBackups(sourcePath string, maxBackups int) error {
    backups, err := ListBackups(sourcePath)
    if err != nil {
        return err
    }

    // Delete oldest backups
    for len(backups) > maxBackups {
        oldest := backups[0]
        backupPath := fmt.Sprintf("%s.%d", sourcePath, oldest)
        if err := os.Remove(backupPath); err != nil {
            return fmt.Errorf("failed to remove old backup: %w", err)
        }
        backups = backups[1:]
    }

    return nil
}

// RestoreBackup restores a backup to the source file
// Creates a backup of current state before restoring
func RestoreBackup(sourcePath string, backupNum int) error {
    backupPath := fmt.Sprintf("%s.%d", sourcePath, backupNum)

    // Verify backup exists
    if _, err := os.Stat(backupPath); err != nil {
        return fmt.Errorf("backup not found: %s", backupPath)
    }

    // Backup current state first
    if _, err := os.Stat(sourcePath); err == nil {
        if _, err := CreateBackup(sourcePath); err != nil {
            return fmt.Errorf("failed to backup current state: %w", err)
        }
    }

    // Restore backup
    data, err := os.ReadFile(backupPath)
    if err != nil {
        return fmt.Errorf("failed to read backup: %w", err)
    }

    if err := fileutil.AtomicWrite(sourcePath, data, 0600); err != nil {
        return fmt.Errorf("failed to restore backup: %w", err)
    }

    return nil
}
```

**Acceptance Criteria** (18 total):
- [ ] CreateBackup() creates numbered backup (.1, .2, etc.)
- [ ] Backup numbering is sequential
- [ ] ListBackups() returns sorted backup numbers
- [ ] RotateBackups() deletes oldest when > MaxBackups
- [ ] MaxBackups = 10 enforced
- [ ] RestoreBackup() restores specified backup
- [ ] Restoration creates backup of current state first
- [ ] Restoration is atomic (uses fileutil.AtomicWrite)
- [ ] `csm backup list <identifier>` shows backups
- [ ] `csm backup restore <identifier> <num>` restores
- [ ] Tests for CreateBackup
- [ ] Tests for rotation (create 15, verify 10 remain)
- [ ] Tests for RestoreBackup
- [ ] Error handling for missing backups
- [ ] Error handling for I/O failures
- [ ] Performance: backup < 50ms
- [ ] Backup file permissions (0600)
- [ ] All tests passing

---

## Additional Updates Required

### Update internal/session/session.go for v2

Current code references v1 fields:
- `m.Claude.SessionID` → Should use `m.SessionID`
- `m.Worktree.Path` → Should use `m.Context.Project`
- `m.Claude.SessionEnvPath` → Remove (not in v2)
- `m.Claude.FileHistoryPath` → Remove (not in v2)

**Changes**:

```go
// ResolveIdentifier (update)
func ResolveIdentifier(identifier string, sessionsDir string) (*manifest.Manifest, string, error) {
    // Try as UUID - NO CHANGE (v2 still has SessionID)
    if err := manifest.ValidateUUID(identifier); err == nil {
        manifests, err := manifest.List(sessionsDir)
        if err != nil {
            return nil, "", fmt.Errorf("failed to list manifests: %w", err)
        }

        for _, m := range manifests {
            if m.SessionID == identifier {  // ✅ v2: SessionID is top-level
                manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
                return m, manifestPath, nil
            }
        }
        return nil, "", fmt.Errorf("session not found: %s", identifier)
    }

    // ... rest unchanged (SessionID, Tmux.SessionName still in v2)
}

// CheckHealth (update for v2)
func CheckHealth(m *manifest.Manifest) (*HealthReport, error) {
    report := &HealthReport{
        Issues: []string{},
    }

    // Check working directory (v2: Context.Project)
    if _, err := os.Stat(m.Context.Project); err != nil {
        report.WorktreeExists = false
        report.Issues = append(report.Issues,
            fmt.Sprintf("Working directory does not exist: %s", m.Context.Project))
    } else {
        report.WorktreeExists = true
    }

    // ❌ Remove SessionEnvPath check (not in v2)
    // ❌ Remove FileHistoryPath check (not in v2)

    return report, nil
}
```

---

## Testing Strategy

### Unit Tests (All with Mocks)

**D2.1 Status Computation**:
- TestComputeStatus_Active
- TestComputeStatus_Stopped
- TestComputeStatus_Archived
- TestComputeStatusBatch_Optimization (single ListSessions call)
- TestComputeStatus_TmuxError

**D2.2 Enhanced Resume**:
- TestResumeExistingSession
- TestResumeStoppedSession_Recreate
- TestResumeArchivedSession_Error
- TestRecreateSession_WorkdirValidation
- TestRecreateSession_SendsClaudeCommand

**D2.3 Backup Management**:
- TestCreateBackup
- TestListBackups
- TestRotateBackups
- TestRestoreBackup
- TestRotateBackups_MaxLimit

### Integration Tests (If Possible)

- Full resume flow with MockTmux
- Backup rotation with real files (use t.TempDir())

---

## Estimated Timeline

| Deliverable                     | Estimated | Tasks                                    |
|---------------------------------|-----------|------------------------------------------|
| D2.1: Status Computation        | 6h        | Interface, real/mock impl, status, tests |
| D2.2: Enhanced Resume           | 8h        | Update resume.go, recreation, tests      |
| D2.3: Backup Management         | 7h        | Rotation logic, CLI command, tests       |
| Updates (session.go, list.go)   | 2h        | Update existing code for v2              |
| Testing & Integration           | 2h        | Full test suite, manual testing          |
| Documentation                   | 1h        | Update docs, commit messages             |
| **Total**                       | **26h**   |                                          |

---

## Acceptance Criteria Summary

**Total**: 55 acceptance criteria across 3 deliverables

**Completion Criteria**:
- [ ] All 55 acceptance criteria met
- [ ] All unit tests passing (target: 30+ new tests)
- [ ] Integration tests passing (if applicable)
- [ ] Code coverage > 60% for new code
- [ ] All existing tests still passing (19 from Sprint 1)
- [ ] Manual testing complete (resume flow, backup operations)
- [ ] Multi-persona review ≥8.5/10

---

## Next Steps

1. ⏭️ Begin D2.1 implementation (TmuxInterface foundation)
2. ⏭️ Update internal/session/session.go for v2
3. ⏭️ Implement status computation
4. ⏭️ Update cmd/csm/list.go
5. ⏭️ Implement D2.2 enhanced resume
6. ⏭️ Implement D2.3 backup management
7. ⏭️ Run full test suite
8. ⏭️ Multi-persona review

---

**Status**: 📋 READY FOR IMPLEMENTATION

**Blocker**: ❌ STOP - Need user approval to proceed with S6 Sprint 2 implementation per Wayfinder command

Per Wayfinder instructions:
> ✅ STOP when: You have multi-persona approval (≥8.5/10) for $1

S5 (Sprint 1) has been approved (9.16/10). Now waiting for user approval to proceed to S6 (Sprint 2).
