# D2: Architecture - Session Persistence & Context Tracking

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Awaiting Multi-Persona Approval
**Prerequisite**: D1 Discovery ✅ Complete

---

## Executive Summary

Based on D1 findings, we will implement **session persistence through automatic reconstruction**. Claude's data already persists across reboots; we just need to rebuild the tmux wrapper and track session context.

**Core Principle**: Sessions never truly "die" - they transition between states (active → stopped → archived).

---

## Architecture Overview

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                     User Interface                          │
│  csm new | csm resume | csm list | csm archive             │
└───────────────┬─────────────────────────────────────────────┘
                │
┌───────────────▼─────────────────────────────────────────────┐
│              Session Manager (Core Logic)                   │
│  • Session lifecycle management                             │
│  • Tmux session detection & recreation                      │
│  • Status tracking & updates                                │
└───────────────┬─────────────────────────────────────────────┘
                │
┌───────────────▼─────────────────────────────────────────────┐
│                  Storage Layer                              │
│                                                             │
│  ┌─────────────────┐     ┌──────────────────────┐         │
│  │  Manifests      │     │  Claude Data         │         │
│  │  ~/sessions/    │     │  ~/.claude/          │         │
│  │  (configurable) │     │  (fixed location)    │         │
│  │                 │     │                      │         │
│  │  • manifest.yaml│     │  • history.jsonl     │         │
│  │  • metadata     │     │  • file-history/     │         │
│  │  • context      │     │  • session-env/      │         │
│  └─────────────────┘     └──────────────────────┘         │
└─────────────────────────────────────────────────────────────┘
```

---

## 1. Session Lifecycle States

### State Diagram

```
     ┌─────┐
     │ new │  (csm new)
     └──┬──┘
        │
        ▼
   ┌────────┐
   │ ACTIVE │  Tmux session exists, Claude running
   └────┬───┘
        │
        │ (reboot / crash / manual stop)
        │
        ▼
   ┌─────────┐
   │ STOPPED │  Tmux gone, Claude data persists
   └────┬────┘
        │
        │ (csm resume → auto-recreate)
        │
        ├──────────────► ACTIVE (tmux recreated)
        │
        │ (csm archive)
        │
        ▼
   ┌──────────┐
   │ ARCHIVED │  Old session, kept for reference
   └──────────┘
```

### State Definitions

**ACTIVE**:
- Tmux session exists and is running
- Claude process may or may not be active
- Manifest `last_activity` updated regularly
- Status: `active`

**STOPPED**:
- Tmux session does not exist
- Claude data persists in ~/.claude/
- Can be resumed (tmux will be recreated)
- Status: `stopped`

**ARCHIVED**:
- Session marked as completed/inactive
- Still accessible for reference (history, logs)
- Won't show in default `csm list`
- Status: `archived`

---

## 2. Enhanced Manifest Schema

### New Fields

```yaml
schema_version: "2.0"  # BREAKING: Schema upgrade
session_id: "session-claude-myapp"
status: "active"  # NEW: active | stopped | archived
created_at: 2025-12-07T10:00:00Z
last_activity: 2025-12-07T12:30:00Z

# NEW: Session context tracking
context:
  purpose: "Implementing user authentication feature"  # User-provided description
  tags: ["feature", "auth", "backend"]  # Optional tags
  notes: "Working with JWT tokens, testing login flow"  # Optional notes

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
  last_detected: 2025-12-07T12:30:00Z  # NEW: Last time we confirmed tmux exists
```

### Schema Migration Strategy

**Challenge**: Existing manifests use schema v1 (no status, no context)

**Solution**: Lazy migration
1. Add schema version detection in manifest.Load()
2. If v1, automatically upgrade to v2 on next write
3. Set default values:
   - status: "active" if tmux exists, else "stopped"
   - context.purpose: "" (empty, user can fill later)
   - tmux.last_detected: now() if exists, else created_at

**Compatibility**: CSM will read both v1 and v2, always write v2

---

## 3. Configurable Sessions Directory

### Current Problem
- Sessions hardcoded to `~/sessions/`
- Conflicts with workspace architecture goal
- Not flexible for different use cases

### Solution: Multi-Level Configuration

**Priority order** (highest to lowest):
1. CLI flag: `--sessions-dir /custom/path`
2. Environment variable: `CSM_SESSIONS_DIR`
3. Config file: `~/.config/csm/config.yaml`
4. Default: `~/sessions/`

### Configuration File Format

```yaml
# ~/.config/csm/config.yaml
sessions_dir: "~/sessions"  # Default
log_level: "info"

# Optional workspace integration
workspace:
  enabled: false
  devlog_root: "$DEVLOG_ROOT"  # Use env var
  sessions_path: "${devlog_root}/sessions"  # Relative to devlog_root
```

### Implementation

```go
// internal/config/config.go
type Config struct {
    SessionsDir string
    LogLevel    string
    Workspace   WorkspaceConfig
}

type WorkspaceConfig struct {
    Enabled      bool
    DevlogRoot   string
    SessionsPath string
}

func Load(cfgFile string) (*Config, error) {
    // 1. Load defaults
    cfg := &Config{
        SessionsDir: expandPath("~/sessions"),
        LogLevel:    "info",
    }

    // 2. Load from config file (if exists)
    // 3. Override with env vars
    if sessionsDir := os.Getenv("CSM_SESSIONS_DIR"); sessionsDir != "" {
        cfg.SessionsDir = expandPath(sessionsDir)
    }

    // 4. CLI flags override in main.go (already implemented)

    return cfg, nil
}
```

---

## 4. Enhanced `csm resume` - Automatic Tmux Reconstruction

### Current Behavior (Phase 3)

```go
func resumeCmd(identifier string) error {
    uuid, manifestPath := resolveSessionIdentifier(identifier)
    checkSessionHealth(uuid, manifestPath)  // Fails if tmux missing
    attachToTmux(tmuxName)  // Fails if tmux missing
}
```

### New Behavior (Session Persistence)

```go
func resumeCmd(identifier string) error {
    uuid, manifest, manifestPath := resolveSessionIdentifier(identifier)

    // Check if tmux exists
    tmuxExists := tmux.SessionExists(manifest.Tmux.SessionName)

    if !tmuxExists {
        // STOPPED state - recreate tmux automatically
        fmt.Printf("⚠ Session is stopped (tmux missing), recreating...\n")

        // Create tmux session
        err := tmux.NewSession(manifest.Tmux.SessionName, manifest.Worktree.Path)
        if err != nil {
            return fmt.Errorf("failed to recreate tmux: %w", err)
        }

        // Resume Claude
        resumeCmd := fmt.Sprintf("claude --resume %s", manifest.Claude.SessionID)
        err = tmux.SendCommand(manifest.Tmux.SessionName, resumeCmd)
        if err != nil {
            return fmt.Errorf("failed to resume Claude: %w", err)
        }

        // Update manifest status
        manifest.Status = manifest.StatusActive
        manifest.Tmux.LastDetected = time.Now()
        err = manifest.Write(manifestPath, manifest)
        if err != nil {
            return fmt.Errorf("failed to update manifest: %w", err)
        }

        fmt.Printf("✓ Session recreated successfully\n")
    } else {
        // ACTIVE state - tmux exists
        fmt.Printf("✓ Session is active\n")
    }

    // Attach to session
    err := tmux.AttachSession(manifest.Tmux.SessionName)
    return err
}
```

### Error Handling

**Scenario**: Worktree directory was deleted

```
⚠ Session is stopped (tmux missing), recreating...
✗ Failed to recreate tmux: worktree directory does not exist: /home/user/deleted-project

Suggestions:
  • Update manifest worktree: csm edit claude-1 --worktree /new/path
  • Archive this session: csm archive claude-1
  • Force resume in current dir: csm resume claude-1 --here
```

---

## 5. Session Context Tracking

### Adding Context to Sessions

**Via `csm new`**:
```bash
csm new ~/projects/myapp --purpose "Implementing user auth feature"
csm new ~/projects/myapp -p "Bug fix for login flow" --tags bug,auth
```

**Via `csm context`** (new command):
```bash
csm context claude-1 "Working on database migration"
csm context claude-1 --tags migration,db,backend
csm context claude-1 --notes "Using Postgres, testing on staging"
```

**Via `csm edit`** (new command):
```bash
csm edit claude-1 --purpose "Updated purpose after pivot"
csm edit claude-1 --add-tag performance
csm edit claude-1 --worktree /new/path  # Update worktree if moved
```

### Displaying Context

**In `csm list`**:
```
UUID      TMUX      STATUS    PROJECT              PURPOSE                    MESSAGES  LAST ACTIVITY
──────────────────────────────────────────────────────────────────────────────────────────────────────
e6121188  claude-2  active ✓  ~/myapp              Implementing user auth     197       2025-12-07 12:30
c4eb298c  claude-1  stopped   ~/myapp              Bug fix for login flow     193       2025-12-06 14:05
c25b857b  claude-4  active ✓  ~/backend            Database migration         55        2025-12-07 11:00
```

**In `csm info <identifier>`** (new command):
```
Session: claude-1 (c4eb298c-8c89-4f75-8dae-c725a1291add)
Status: STOPPED (tmux session missing, can be resumed)

Purpose: Implementing user authentication feature
Tags: feature, auth, backend
Notes: Working with JWT tokens, testing login flow

Project: /home/user/projects/myapp
Tmux: claude-myapp (last detected: 2025-12-06 14:05)
Messages: 193
Duration: 8.2 hours
Last Activity: 2025-12-06 14:05

Resume: csm resume claude-1
```

---

## 6. Session Status Detection

### Status Update Strategy

**When to update status?**

1. **On resume**: Always check tmux existence, update status
2. **On list**: Quick check (don't update manifest, just display)
3. **On background sync** (optional future): Periodic status refresh

### Tmux Detection Logic

```go
// internal/tmux/tmux.go
func SessionExists(name string) bool {
    cmd := exec.Command("tmux", "has-session", "-t", name)
    err := cmd.Run()
    return err == nil  // Exit code 0 = session exists
}

// Fast batch check for multiple sessions
func ListSessions() ([]string, error) {
    cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
    output, err := cmd.Output()
    if err != nil {
        if strings.Contains(err.Error(), "no server running") {
            return []string{}, nil  // No tmux server = no sessions
        }
        return nil, err
    }

    names := strings.Split(strings.TrimSpace(string(output)), "\n")
    return names, nil
}
```

### Status in `csm list`

**Performance concern**: Checking tmux for every session on every `csm list` could be slow

**Solution**: Batch check
```go
// Get all tmux sessions once
tmuxSessions := tmux.ListSessions()
tmuxMap := make(map[string]bool)
for _, name := range tmuxSessions {
    tmuxMap[name] = true
}

// Check each manifest's tmux name
for _, manifest := range manifests {
    if tmuxMap[manifest.Tmux.SessionName] {
        manifest.Status = "active"
    } else {
        manifest.Status = "stopped"
    }
}
```

---

## 7. Archive Command

### Purpose
- Mark old sessions as archived
- Remove from default `csm list`
- Keep data for reference

### Usage

```bash
csm archive claude-1                # Archive single session
csm archive --older-than 30d        # Archive sessions inactive >30 days
csm archive --interactive           # Pick sessions to archive

csm list --archived                 # Show only archived sessions
csm list --all                      # Show all (active + stopped + archived)
```

### Implementation

```go
func archiveCmd(identifier string) error {
    uuid, manifest, manifestPath := resolveSessionIdentifier(identifier)

    // Confirm with user
    fmt.Printf("Archive session %s (%s)?\n", manifest.Tmux.SessionName, uuid[:8])
    fmt.Printf("Purpose: %s\n", manifest.Context.Purpose)

    confirm := ui.Confirm("Archive this session?")
    if !confirm {
        return nil
    }

    // Update status
    manifest.Status = manifest.StatusArchived
    manifest.LastActivity = time.Now()

    err := manifest.Write(manifestPath, manifest)
    if err != nil {
        return err
    }

    fmt.Printf("✓ Session archived: %s\n", manifest.Tmux.SessionName)
    fmt.Printf("  Data preserved in: %s\n", manifestPath)
    fmt.Printf("  To unarchive: csm unarchive %s\n", manifest.Tmux.SessionName)

    return nil
}
```

---

## 8. Migration Path

### For Existing Users

**Problem**: Users already have sessions with v1 manifests

**Solution**: Automatic upgrade on first use

1. **On startup**: Detect schema version
2. **On write**: Always write v2
3. **On read**: Support both v1 and v2

### Migration Steps

```go
// internal/manifest/manifest.go
func Load(path string) (*Manifest, error) {
    var raw map[string]interface{}
    // ... load YAML ...

    // Check schema version
    version := raw["schema_version"]
    if version == nil || version == "1.0" {
        // Migrate v1 → v2
        manifest := migrateV1ToV2(raw)
        return manifest, nil
    }

    // Parse v2
    var manifest Manifest
    // ... unmarshal ...
    return &manifest, nil
}

func migrateV1ToV2(raw map[string]interface{}) *Manifest {
    m := &Manifest{
        SchemaVersion: "2.0",
        // ... copy existing fields ...
    }

    // Add new fields with defaults
    m.Status = detectInitialStatus(m)
    m.Context = Context{
        Purpose: "",  // User can fill later
        Tags:    []string{},
        Notes:   "",
    }

    return m
}

func detectInitialStatus(m *Manifest) string {
    // Check if tmux exists
    if tmux.SessionExists(m.Tmux.SessionName) {
        return StatusActive
    }
    return StatusStopped
}
```

---

## 9. Implementation Phases

### Phase A: Core Infrastructure (Priority 1)
1. ✅ Add status field to manifest
2. ✅ Add context fields to manifest
3. ✅ Implement schema migration (v1 → v2)
4. ✅ Add configurable sessions directory
5. ✅ Implement batch tmux status detection

### Phase B: Resume Enhancement (Priority 1)
6. ✅ Enhance `csm resume` to auto-recreate tmux
7. ✅ Add status updates on resume
8. ✅ Add error handling for missing worktree
9. ✅ Test end-to-end resume after "reboot" (kill tmux)

### Phase C: Context Management (Priority 2)
10. ✅ Add `--purpose` flag to `csm new`
11. ✅ Implement `csm context` command
12. ✅ Implement `csm edit` command
13. ✅ Update `csm list` to show purpose
14. ✅ Implement `csm info` command

### Phase D: Lifecycle Management (Priority 2)
15. ✅ Implement `csm archive` command
16. ✅ Add `--archived` and `--all` flags to list
17. ✅ Implement `csm unarchive` command
18. ✅ Add archive suggestions for old sessions

---

## 10. Success Criteria

### Functional Requirements

1. ✅ Sessions can be resumed after tmux termination (simulated reboot)
2. ✅ Users can add purpose/context to sessions
3. ✅ `csm list` shows session status (active/stopped/archived)
4. ✅ Sessions directory is configurable
5. ✅ Schema migration works transparently

### Performance Requirements

1. ✅ `csm list` completes in < 1 second (even with 50+ sessions)
2. ✅ `csm resume` auto-recreation completes in < 3 seconds
3. ✅ Batch tmux detection (not N checks for N sessions)

### UX Requirements

1. ✅ Zero-config for basic use (works out of box)
2. ✅ Clear error messages when worktree missing
3. ✅ Resuming stopped session feels natural (not scary)
4. ✅ Context is optional (not forced on users)

---

## 11. Open Questions

### Q1: Should we backup history.jsonl entries?

**Context**: history.jsonl is append-only, could grow large

**Options**:
- A: Don't backup (rely on Claude's file)
- B: Copy relevant entries to session dir
- C: Symlink to Claude's file

**Recommendation**: **A** - Keep it simple for now, add backup later if needed

### Q2: Should archived sessions be in separate directory?

**Options**:
- A: Keep in same directory, just change status field
- B: Move to `~/sessions/.archived/`
- C: Delete manifest, keep only in ~/.claude/

**Recommendation**: **A** - Simpler, easier to unarchive

### Q3: Should we detect orphaned tmux sessions?

**Context**: User might create tmux session manually without CSM

**Options**:
- A: Ignore (out of scope)
- B: Detect and offer to import (like auto-import)
- C: Warn user about unknown tmux sessions

**Recommendation**: **B** - Consistent with auto-import feature

---

## 12. Risk Analysis

### Risk 1: Schema Migration Bugs
**Impact**: High - could corrupt manifests
**Mitigation**:
- Write comprehensive tests
- Backup manifest before migration
- Add `--dry-run` flag to test migration

### Risk 2: Race Condition on Status Updates
**Impact**: Medium - stale status in manifest
**Mitigation**:
- Status is advisory, not critical
- Always check tmux on resume (don't trust manifest)
- Document that status may be stale

### Risk 3: Configurable Directory Confusion
**Impact**: Medium - users forget where sessions are
**Mitigation**:
- `csm info` shows config values
- `csm list` always searches correct directory
- Error messages show current sessions-dir

---

## Next Steps for D3

1. Create detailed implementation plan with file changes
2. Design test strategy for each feature
3. Create migration testing checklist
4. Design rollback strategy if issues found

**Status**: Ready for Multi-Persona Review
