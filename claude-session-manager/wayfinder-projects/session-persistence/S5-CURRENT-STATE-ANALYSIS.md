# S5: Current Codebase State Analysis

**Date**: December 7, 2025
**Purpose**: Understand existing v1 implementation before upgrading to v2

---

## Current State Discovery

The repository `/home/user/src/repos/ai-tools/base/claude-session-manager/` already contains a working v1 implementation of CSM.

### Existing Directory Structure

```
claude-session-manager/
├── cmd/csm/               # CLI commands
│   ├── main.go
│   ├── list.go
│   ├── resume.go
│   ├── sync.go
│   ├── doctor.go
│   ├── new.go
│   └── version.go
├── internal/
│   ├── manifest/          # Manifest v1 schema (EXISTS)
│   │   ├── manifest.go
│   │   ├── read.go
│   │   ├── write.go
│   │   ├── validate.go
│   │   └── validate_test.go
│   ├── tmux/             # Tmux operations
│   ├── config/           # Configuration
│   ├── discovery/        # Session discovery
│   ├── session/          # Session management
│   └── claude/           # Claude history parsing
```

### Existing Manifest v1 Schema

From `internal/manifest/manifest.go`:

```go
type Manifest struct {
    SchemaVersion string    `yaml:"schema_version"`  // "1.0"
    SessionID     string    `yaml:"session_id"`
    Status        string    `yaml:"status"`           // active/discovered/stale/archived
    CreatedAt     time.Time `yaml:"created_at"`
    LastActivity  time.Time `yaml:"last_activity"`
    Worktree      Worktree  `yaml:"worktree"`
    Claude        Claude    `yaml:"claude"`
    Tmux          Tmux      `yaml:"tmux"`
}
```

**Key Difference from v2**:
- v1 has `Status` field (stored, goes stale)
- v2 has `Lifecycle` field ("" or "archived" only, status is computed)
- v1 has `LastActivity` (not in v2)
- v1 has `Claude` struct (detailed session-env paths - not in v2)
- v2 has `Context` struct (project, purpose, tags, notes - not in v1)

---

## Implementation Strategy

### Approach: In-Place Upgrade (Not Parallel)

We will **MODIFY** existing files to support v2, not create parallel implementations.

**Rationale**:
- Single codebase, no duplication
- Migration logic handles v1 → v2 conversion
- Simpler to maintain

### Files to Modify (Sprint 1)

#### D1.1: Manifest Schema v2

**Create**:
- `internal/manifest/constants.go` (NEW) - Centralized constants

**Modify**:
- `internal/manifest/manifest.go` - Add v2 structs (keep v1 for migration)
- `internal/manifest/read.go` - Support both v1 and v2
- `internal/manifest/write.go` - Write v2 format

**Strategy**:
```go
// manifest.go will have BOTH:
type Manifest struct { ... }        // v2 schema
type ManifestV1 struct { ... }      // v1 schema (for migration)
```

#### D1.2: Context Validation

**Modify**:
- `internal/manifest/validate.go` - Add v2 validation rules
- `internal/manifest/validate_test.go` - Add v2 test cases

#### D1.3: File Locking

**Create**:
- `internal/manifest/lock.go` (NEW) - AcquireLock/ReleaseLock
- `internal/manifest/lock_test.go` (NEW) - Lock tests

#### D1.4: Migration v1 → v2

**Create**:
- `internal/manifest/migrate.go` (NEW) - MigrateV1ToV2
- `internal/manifest/migrate_test.go` (NEW) - Migration tests

#### D1.5: Fileutil Package

**Create**:
- `internal/fileutil/atomic.go` (NEW) - AtomicWrite
- `internal/fileutil/atomic_test.go` (NEW) - Atomic write tests

---

## Migration Path

### Auto-Migration on Load

When `Load()` is called:
1. Read file
2. Detect version (check `schema_version` field)
3. If v1: Call `MigrateV1ToV2()` (creates .v1.bak, writes v2)
4. If v2: Load directly
5. Return v2 manifest

### Backward Compatibility

**None required** - v1 manifests are automatically upgraded to v2 on first access.

Users don't need to run a migration command - it happens transparently.

---

## Next Steps

1. ✅ Analyzed current codebase
2. ⏭️ Implement constants.go
3. ⏭️ Update manifest.go with v2 schema
4. ⏭️ Update validation with v2 rules
5. ⏭️ Implement locking
6. ⏭️ Implement migration
7. ⏭️ Implement fileutil
8. ⏭️ Test all Sprint 1 deliverables
9. ⏭️ Multi-persona review

---

**Status**: ✅ Analysis Complete - Ready to implement

**Commit this analysis to**: `wayfinder-projects/session-persistence/`
