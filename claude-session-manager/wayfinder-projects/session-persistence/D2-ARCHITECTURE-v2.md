# D2: Architecture - Session Persistence & Context Tracking (v2)

**Date**: December 7, 2025
**Version**: 2 (Revised after R1 feedback)
**Status**: 🔄 IN REVIEW - Round 2
**Prerequisite**: D1 Discovery ✅ Complete

**Changes from v1**:
- ✅ Status field → Lifecycle field (computed status)
- ✅ Added validation for context fields
- ✅ Added backup strategy section
- ✅ Added migration rollback plan
- ✅ Reduced scope (split into 2 phases)
- ✅ Added concurrency handling
- ✅ Consolidated commands (context+edit → set)

---

## Executive Summary

Based on D1 findings and R1 feedback, we will implement **session persistence through automatic reconstruction** in TWO phases:

**Phase 3.5 (This Phase)**: Core session persistence (auto-recreation + infrastructure)
**Phase 4**: Context tracking & lifecycle management (defer to future)

**Core Principle**: Sessions never truly "die" - they transition between states, and status is computed dynamically (not stored).

---

## 1. Enhanced Manifest Schema (v2)

### Schema Changes

```yaml
schema_version: "2.0"  # BREAKING: Schema upgrade
session_id: "session-claude-myapp"

# REMOVED: status field (computed dynamically)
# NEW: lifecycle field (only for archived state)
lifecycle: ""  # "" = active/stopped (dynamic), "archived" = archived

created_at: 2025-12-07T10:00:00Z
last_activity: 2025-12-07T12:30:00Z

# NEW: Session context tracking (OPTIONAL - can be empty)
context:
  purpose: "Implementing user authentication"  # max 256 chars
  tags: ["feature", "auth"]  # max 10 tags, each max 32 chars
  notes: ""  # max 1024 chars (empty by default)

worktree:
  path: "/home/user/projects/myapp"

claude:
  session_id: "c4eb298c-8c89-4f75-8dae-c725a1291add"
  session_env_path: "/home/user/.claude/session-env/c4eb298c-..."
  file_history_path: "/home/user/.claude/file-history/c4eb298c-..."
  started_at: 2025-12-07T10:00:00Z
  last_activity: 2025-12-07T12:30:00Z

tmux:
  session_name: "claude-myapp"
  window_name: "main"
  created_at: 2025-12-07T10:00:00Z
```

### Key Changes from v1

1. **Removed `status` field**: Status is computed, not stored
2. **Added `lifecycle` field**: Only stores "archived" state
3. **Added validation constraints**: Size limits on context fields
4. **Made context optional**: Can be completely empty

### Status Computation

```go
// Status is NEVER stored - always computed
func (m *Manifest) GetStatus() SessionStatus {
    if m.Lifecycle == "archived" {
        return StatusArchived
    }
    if tmux.SessionExists(m.Tmux.SessionName) {
        return StatusActive
    }
    return StatusStopped
}

type SessionStatus string
const (
    StatusActive   SessionStatus = "active"   // Tmux exists
    StatusStopped  SessionStatus = "stopped"  // Tmux missing, can resume
    StatusArchived SessionStatus = "archived" // Marked archived
)
```

### Context Validation

```go
type Context struct {
    Purpose string   `yaml:"purpose" validate:"max=256"`
    Tags    []string `yaml:"tags" validate:"max=10,dive,max=32"`
    Notes   string   `yaml:"notes" validate:"max=1024"`
}

func (c *Context) Validate() error {
    if len(c.Purpose) > 256 {
        return fmt.Errorf("purpose too long (max 256 chars)")
    }
    if len(c.Tags) > 10 {
        return fmt.Errorf("too many tags (max 10)")
    }
    for _, tag := range c.Tags {
        if len(tag) > 32 {
            return fmt.Errorf("tag too long: %q (max 32 chars)", tag)
        }
    }
    if len(c.Notes) > 1024 {
        return fmt.Errorf("notes too long (max 1024 chars)")
    }
    return nil
}
```

---

## 2. Schema Migration Strategy (v2 - With Rollback)

### Migration Process

```go
func Load(path string) (*Manifest, error) {
    // Read current manifest
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var raw map[string]interface{}
    err = yaml.Unmarshal(data, &raw)
    if err != nil {
        return nil, err
    }

    // Check schema version
    version := raw["schema_version"]
    if version == nil || version == "1.0" {
        // V1 manifest - needs migration
        manifest := migrateV1ToV2(raw, path)
        return manifest, nil
    }

    // V2 manifest
    var manifest Manifest
    err = yaml.Unmarshal(data, &manifest)
    return &manifest, err
}

func migrateV1ToV2(raw map[string]interface{}, path string) *Manifest {
    // STEP 1: Backup original manifest
    backupPath := path + ".v1.bak"
    err := copyFile(path, backupPath)
    if err != nil {
        log.Printf("Warning: could not backup manifest: %v", err)
    }

    // STEP 2: Convert to v2
    m := &Manifest{
        SchemaVersion: "2.0",
        // ... copy all v1 fields ...
    }

    // STEP 3: Add new fields with defaults
    m.Lifecycle = ""  // Will be computed dynamically
    m.Context = Context{
        Purpose: "",
        Tags:    []string{},
        Notes:   "",
    }

    // STEP 4: Write v2 manifest
    err = Write(path, m)
    if err != nil {
        // ROLLBACK: Restore from backup
        restoreErr := copyFile(backupPath, path)
        if restoreErr != nil {
            log.Printf("CRITICAL: Migration failed AND rollback failed!")
            log.Printf("Backup is at: %s", backupPath)
        }
        return nil
    }

    log.Printf("✓ Migrated manifest from v1 → v2 (backup: %s)", backupPath)
    return m
}
```

### Migration Safety

1. **Always backup before migration**: `manifest.yaml.v1.bak`
2. **Atomic write**: Use temp file + rename
3. **Rollback on failure**: Restore from backup
4. **Keep backup**: Don't delete .v1.bak files (user can clean up later)

---

## 3. Concurrency Handling

### Problem

Two terminals running `csm resume claude-1` simultaneously:
- Both try to write manifest
- Both try to create tmux session
- Race condition!

### Solution: File Locking

```go
// internal/manifest/lock.go
type Lock struct {
    file *os.File
}

func AcquireLock(manifestPath string) (*Lock, error) {
    lockPath := manifestPath + ".lock"

    // Try to create lock file (exclusive)
    file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
    if err != nil {
        if os.IsExist(err) {
            // Lock file exists - another process has it
            return nil, fmt.Errorf("session is locked by another process")
        }
        return nil, err
    }

    // Write PID to lock file
    fmt.Fprintf(file, "%d\n", os.Getpid())

    return &Lock{file: file}, nil
}

func (l *Lock) Release() error {
    if l.file == nil {
        return nil
    }

    lockPath := l.file.Name()
    l.file.Close()
    return os.Remove(lockPath)
}

// Usage in resume
func resumeCmd(identifier string) error {
    uuid, manifest, manifestPath := resolveSessionIdentifier(identifier)

    // Acquire lock
    lock, err := manifest.AcquireLock(manifestPath)
    if err != nil {
        return fmt.Errorf("could not acquire lock: %w", err)
    }
    defer lock.Release()

    // Now safe to modify session
    // ...
}
```

### Lock Timeout

**Problem**: What if lock file is stale (process crashed)?

**Solution**: Add timestamp to lock file, auto-cleanup if > 60 seconds old

```go
func isLockStale(lockPath string, maxAge time.Duration) bool {
    info, err := os.Stat(lockPath)
    if err != nil {
        return true  // Can't stat = treat as stale
    }

    age := time.Since(info.ModTime())
    return age > maxAge
}
```

---

## 4. Session Log Backup Strategy

### Original Requirement

From problem statement: "Should we consider backing up session logs somewhere for our own reference?"

### Strategy: On-Demand Backup

**Decision**: Don't backup automatically (to avoid duplication), but provide command to backup on demand

### Implementation

```bash
csm backup claude-1                    # Backup single session
csm backup claude-1 --format markdown  # Export as markdown
csm backup --all                       # Backup all sessions
```

### Backup Format

```
~/sessions/session-claude-myapp/
├── manifest.yaml
└── backups/
    └── 2025-12-07_14-30-00/
        ├── session-info.yaml          # Manifest snapshot
        ├── conversation.jsonl         # Relevant history.jsonl entries
        ├── conversation.md            # Human-readable format
        └── file-snapshots/            # Copies of file-history files
            ├── file1.go
            ├── file2.go
            └── ...
```

### Backup Command Implementation

```go
func backupCmd(identifier string, format string) error {
    uuid, manifest, _ := resolveSessionIdentifier(identifier)

    // Create backup directory
    timestamp := time.Now().Format("2006-01-02_15-04-05")
    backupDir := filepath.Join(
        filepath.Dir(manifestPath),
        "backups",
        timestamp,
    )
    err := os.MkdirAll(backupDir, 0700)

    // 1. Copy manifest
    manifestSnapshot := filepath.Join(backupDir, "session-info.yaml")
    copyFile(manifestPath, manifestSnapshot)

    // 2. Extract relevant history entries
    entries := extractHistoryEntries(uuid)
    if format == "markdown" {
        writeMarkdown(entries, filepath.Join(backupDir, "conversation.md"))
    } else {
        writeJSONL(entries, filepath.Join(backupDir, "conversation.jsonl"))
    }

    // 3. Copy file snapshots (optional, can be large)
    if includeFiles {
        copyDirectory(manifest.Claude.FileHistoryPath,
                     filepath.Join(backupDir, "file-snapshots"))
    }

    fmt.Printf("✓ Backup created: %s\n", backupDir)
    return nil
}
```

### Backup Frequency

**Not automatic** - user decides when to backup:
- Before archiving a session
- After completing a major milestone
- Before system rebuild/reinstall

---

## 5. Reduced Scope - Phase Split

### Phase 3.5: Session Persistence Core (THIS PHASE)

**Goal**: Sessions can be resumed after reboot

**Features**:
1. ✅ Schema v2 (lifecycle field, context struct)
2. ✅ Schema migration (v1 → v2 with rollback)
3. ✅ Configurable sessions directory
4. ✅ Enhanced `csm resume` (auto-recreate tmux)
5. ✅ Concurrency handling (file locking)
6. ✅ Backup command (`csm backup`)
7. ✅ Status computation (dynamic, not stored)

**Out of Scope for 3.5**:
- Context management commands (set, info)
- Archive command
- Complex lifecycle management

### Phase 4: Context & Lifecycle (FUTURE)

**Goal**: Track what sessions are for, manage old sessions

**Features**:
1. `csm set` - Set purpose/tags/notes
2. `csm info` - Show detailed session info
3. `csm archive` / `csm unarchive`
4. `csm list --archived` / `--all`
5. Smart suggestions (archive old sessions)

**Why defer**: Core persistence more important, context is enhancement

---

## 6. Enhanced `csm resume` - Automatic Tmux Reconstruction

### Implementation (Revised)

```go
func resumeCmd(identifier string) error {
    uuid, manifest, manifestPath := resolveSessionIdentifier(identifier)

    // Acquire lock (prevent concurrent modification)
    lock, err := AcquireLock(manifestPath)
    if err != nil {
        return err
    }
    defer lock.Release()

    // Check lifecycle state
    if manifest.Lifecycle == "archived" {
        ui.PrintWarning("This session is archived")
        confirm := ui.Confirm("Unarchive and resume?")
        if !confirm {
            return fmt.Errorf("session is archived")
        }
        manifest.Lifecycle = ""  // Unarchive
    }

    // Compute current status
    status := manifest.GetStatus()

    switch status {
    case StatusActive:
        // Tmux exists - just attach
        fmt.Printf("✓ Session is active\n")

    case StatusStopped:
        // Tmux missing - recreate
        fmt.Printf("⚠ Session stopped (tmux missing), recreating...\n")

        err := recreateTmuxSession(manifest)
        if err != nil {
            return fmt.Errorf("failed to recreate session: %w", err)
        }

        fmt.Printf("✓ Session recreated successfully\n")
    }

    // Update last activity
    manifest.LastActivity = time.Now()
    err = Write(manifestPath, manifest)
    if err != nil {
        ui.PrintWarning(fmt.Sprintf("Could not update manifest: %v", err))
    }

    // Attach to session
    return tmux.AttachSession(manifest.Tmux.SessionName)
}

func recreateTmuxSession(m *Manifest) error {
    // Validate worktree exists
    if _, err := os.Stat(m.Worktree.Path); os.IsNotExist(err) {
        return fmt.Errorf("worktree does not exist: %s\n"+
            "Suggestions:\n"+
            "  • Update worktree: csm set %s --worktree /new/path\n"+
            "  • Archive session: csm archive %s",
            m.Worktree.Path, m.Tmux.SessionName, m.Tmux.SessionName)
    }

    // Create tmux session
    err := tmux.NewSession(m.Tmux.SessionName, m.Worktree.Path)
    if err != nil {
        return fmt.Errorf("failed to create tmux session: %w", err)
    }

    // Resume Claude
    claudeCmd := fmt.Sprintf("claude --resume %s", m.Claude.SessionID)
    err = tmux.SendCommand(m.Tmux.SessionName, claudeCmd)
    if err != nil {
        // Rollback: kill tmux session
        _ = tmux.SendCommand(m.Tmux.SessionName,
            fmt.Sprintf("tmux kill-session -t %s", m.Tmux.SessionName))
        return fmt.Errorf("failed to start Claude: %w", err)
    }

    return nil
}
```

### Error Handling - Partial Failure Rollback

**Scenario**: Tmux created, but Claude fails to start

**Solution**: Rollback tmux creation
```go
err = tmux.SendCommand(tmuxName, claudeCmd)
if err != nil {
    // Rollback: kill tmux session we just created
    _ = tmux.KillSession(tmuxName)
    return fmt.Errorf("failed to start Claude: %w", err)
}
```

---

## 7. Consolidated Commands

### Addressing R1 Feedback: Too Many Commands

**v1 had**:
- `csm context` - Set context
- `csm edit` - Edit manifest fields
- `csm info` - Show session info

**v2 has** (Phase 4):
- `csm set` - Set any field (purpose, tags, notes, worktree)
- `csm info` - Show session info (unchanged)

### Example Usage (Phase 4)

```bash
# Set purpose
csm set claude-1 --purpose "Implementing user auth"

# Add tags
csm set claude-1 --tags feature,auth,backend

# Update worktree (if moved)
csm set claude-1 --worktree /new/path

# Add notes
csm set claude-1 --notes "Using JWT, testing on staging"

# Show info
csm info claude-1
```

**Benefit**: One command for all modifications

---

## 7. Health Check Command (`csm doctor`)

### Purpose

Validate session integrity and detect common issues. Essential for operations and troubleshooting.

### Usage

```bash
csm doctor                     # Check all sessions
csm doctor claude-1            # Check specific session
csm doctor --check-migrations  # Verify all migrations succeeded
csm doctor --fix               # Auto-fix issues (stale locks, etc.)
```

### Checks Performed

```go
func doctorCmd(identifier string, checkMigrations bool, autoFix bool) error {
    fmt.Println("Checking CSM health...")
    fmt.Println()

    var issues []string
    var warnings []string

    // 1. Check sessions directory exists
    sessionsDir := getSessionsDir()
    if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
        issues = append(issues, fmt.Sprintf("Sessions directory missing: %s", sessionsDir))
    } else {
        fmt.Printf("✓ Sessions directory: %s\n", sessionsDir)
    }

    // 2. Load all manifests
    manifests, err := discovery.LoadAllManifests(sessionsDir)
    if err != nil {
        issues = append(issues, fmt.Sprintf("Failed to load manifests: %v", err))
    } else {
        fmt.Printf("✓ %d manifests found\n", len(manifests))
    }

    // 3. Check manifest validity
    invalidCount := 0
    for path, m := range manifests {
        if err := m.Validate(); err != nil {
            invalidCount++
            issues = append(issues, fmt.Sprintf("Invalid manifest %s: %v", path, err))
        }
    }
    if invalidCount == 0 {
        fmt.Printf("✓ All manifests are valid\n")
    } else {
        fmt.Printf("✗ %d invalid manifests\n", invalidCount)
    }

    // 4. Check for stale lock files
    staleLocks := findStaleLocks(sessionsDir, 60*time.Second)
    if len(staleLocks) > 0 {
        warnings = append(warnings, fmt.Sprintf("%d stale lock files", len(staleLocks)))
        if autoFix {
            for _, lock := range staleLocks {
                os.Remove(lock)
            }
            fmt.Printf("✓ Cleaned %d stale lock files\n", len(staleLocks))
        } else {
            fmt.Printf("⚠ %d stale lock files (run --fix to clean)\n", len(staleLocks))
        }
    } else {
        fmt.Printf("✓ No stale lock files\n")
    }

    // 5. Check Claude UUIDs exist in history.jsonl
    missingUUIDs := 0
    historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
    entries, _, _ := claude.ParseHistory(historyPath)
    sessions := claude.Deduplicate(entries)

    uuidMap := make(map[string]bool)
    for _, s := range sessions {
        uuidMap[s.UUID] = true
    }

    for _, m := range manifests {
        if !uuidMap[m.Claude.SessionID] {
            missingUUIDs++
            warnings = append(warnings, fmt.Sprintf("UUID not in history: %s", m.Claude.SessionID[:8]))
        }
    }

    if missingUUIDs == 0 {
        fmt.Printf("✓ All Claude UUIDs exist in history.jsonl\n")
    } else {
        fmt.Printf("⚠ %d UUIDs not found in history (sessions may be very old)\n", missingUUIDs)
    }

    // 6. Check worktrees exist
    missingWorktrees := 0
    for path, m := range manifests {
        if _, err := os.Stat(m.Worktree.Path); os.IsNotExist(err) {
            missingWorktrees++
            warnings = append(warnings, fmt.Sprintf("%s: missing worktree %s (suggest: csm archive %s)",
                filepath.Base(path), m.Worktree.Path, m.Tmux.SessionName))
        }
    }

    if missingWorktrees == 0 {
        fmt.Printf("✓ All worktrees exist\n")
    } else {
        fmt.Printf("⚠ %d sessions have missing worktrees\n", missingWorktrees)
    }

    // 7. Check migrations (if requested)
    if checkMigrations {
        oldBackups := findMigrationBackups(sessionsDir)
        if len(oldBackups) > 0 {
            fmt.Printf("✓ Found %d v1 backup files (migrations successful)\n", len(oldBackups))
            fmt.Printf("  You can safely delete *.v1.bak files\n")
        } else {
            fmt.Printf("✓ No migration backups (all sessions on schema v2)\n")
        }
    }

    // Summary
    fmt.Println()
    fmt.Printf("Summary: %d warnings, %d errors\n", len(warnings), len(issues))

    if len(issues) > 0 {
        fmt.Println("\nErrors:")
        for _, issue := range issues {
            fmt.Printf("  ✗ %s\n", issue)
        }
        return fmt.Errorf("health check failed")
    }

    if len(warnings) > 0 && !autoFix {
        fmt.Println("\nWarnings:")
        for _, warning := range warnings {
            fmt.Printf("  ⚠ %s\n", warning)
        }
    }

    fmt.Println("\n✓ CSM is healthy")
    return nil
}
```

### Example Output

```
Checking CSM health...

✓ Sessions directory: /home/user/sessions
✓ 15 manifests found
✓ All manifests are valid
⚠ 2 stale lock files (run --fix to clean)
✓ All Claude UUIDs exist in history.jsonl
⚠ 1 session has missing worktree: claude-old (suggest: csm archive claude-old)

Summary: 2 warnings, 0 errors

Warnings:
  ⚠ 2 stale lock files
  ⚠ session-claude-old/manifest.yaml: missing worktree /home/user/deleted-project

✓ CSM is healthy (with warnings)
```

---

## 8. Implementation Plan (Phase 3.5 Only)

### Deliverables

1. ✅ Manifest schema v2 with lifecycle field
2. ✅ Migration logic (v1 → v2) with rollback + user messaging
3. ✅ Context validation (size limits)
4. ✅ File locking for concurrent access (60s timeout)
5. ✅ Enhanced `csm resume` with auto-recreation
6. ✅ Partial failure rollback
7. ✅ `csm backup` command
8. ✅ Status computation logic
9. ✅ Configurable sessions directory
10. ✅ `csm doctor` command (health checks)
11. ✅ Tests for all above

### Files to Modify

```
internal/manifest/
  ├── manifest.go        # Add lifecycle, context, validation
  ├── manifest_test.go   # Test migration, validation
  ├── lock.go            # NEW: File locking
  └── migrate.go         # NEW: V1→V2 migration

cmd/csm/
  ├── resume.go          # Enhanced with auto-recreation
  ├── backup.go          # NEW: Backup command
  ├── doctor.go          # NEW: Health check command
  └── resume_test.go     # Integration tests

internal/config/
  └── config.go          # Configurable sessions-dir (already done)

internal/tmux/
  ├── tmux.go            # Add KillSession for rollback
  └── tmux_test.go       # Test session lifecycle
```

---

## 9. Success Criteria (Phase 3.5)

### Functional

1. ✅ Kill all tmux sessions, run `csm resume claude-1` → recreates and works
2. ✅ Run `csm resume claude-1` twice concurrently → one succeeds, one waits/fails gracefully
3. ✅ Migrate v1 manifest → v2 backup created, migration succeeds
4. ✅ Set purpose with 500 chars → validation error
5. ✅ `csm backup claude-1` → creates backup directory with conversation

### Performance

1. ✅ `csm resume` auto-recreation < 3 seconds
2. ✅ `csm list` with 50 sessions < 1 second
3. ✅ Migration doesn't block other operations

### UX

1. ✅ Clear messages when recreating session
2. ✅ Helpful errors when worktree missing
3. ✅ Migration is transparent (user doesn't notice)
4. ✅ Lock conflicts have clear error messages

---

## 10. Open Questions (Resolved in v2)

### Q1: Backup history.jsonl entries?
**Answer**: Yes, via `csm backup` command (on-demand, not automatic)

### Q2: Archived sessions in separate directory?
**Answer**: No (deferred to Phase 4), keep in same directory with lifecycle field

### Q3: Detect orphaned tmux sessions?
**Answer**: Defer to Phase 4 (out of scope for persistence core)

---

## 11. Risk Mitigation

### Risk 1: Migration Bugs
**Mitigation**:
- ✅ Always backup before migration (`.v1.bak`)
- ✅ Rollback on write failure
- ✅ Keep backup files (don't auto-delete)
- ✅ Add `csm doctor --check-migrations` to verify

### Risk 2: Race Conditions
**Mitigation**:
- ✅ File locking with PID tracking
- ✅ Timeout for stale locks (5 min)
- ✅ Clear error messages

### Risk 3: Partial Failures
**Mitigation**:
- ✅ Rollback tmux if Claude fails
- ✅ Rollback manifest if write fails
- ✅ Atomic operations where possible

### Risk 4: Validation Bypass
**Mitigation**:
- ✅ Validate on every manifest write
- ✅ Truncate instead of reject (UX choice)
- ✅ Warn user if truncated

---

## Changes from v1

1. ✅ **Status → Lifecycle**: Store only archived state, compute active/stopped
2. ✅ **Validation**: Added size limits for context fields
3. ✅ **Backup strategy**: Defined `csm backup` command
4. ✅ **Migration rollback**: Backup + restore on failure
5. ✅ **Scope reduction**: Split Phase 4 features to future
6. ✅ **Concurrency**: File locking with timeout
7. ✅ **Command consolidation**: context+edit → set (Phase 4)
8. ✅ **Partial failure rollback**: Kill tmux if Claude fails

---

## Ready for Round 2 Review

**Version**: 2
**Status**: Awaiting Multi-Persona Approval
**Target Score**: ≥8.5/10
