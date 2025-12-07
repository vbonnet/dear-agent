# D3: Implementation Design - Session Persistence

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Awaiting Multi-Persona Approval
**Prerequisites**:
- D1 Discovery ✅ Complete
- D2 Architecture ✅ Approved (8.8/10)

---

## Executive Summary

This document provides detailed implementation steps for Phase 3.5 (Session Persistence Core). Each section includes code changes, test plans, and migration strategies.

**Implementation Strategy**: Incremental delivery in 3 stages
1. **Stage 1**: Schema v2 + Migration (Days 1-2)
2. **Stage 2**: Enhanced Resume + Locking (Days 3-4)
3. **Stage 3**: Backup + Doctor Commands (Days 5-6)

---

## Table of Contents

1. [Manifest Schema v2 Implementation](#1-manifest-schema-v2-implementation)
2. [Schema Migration (v1 → v2)](#2-schema-migration-v1--v2)
3. [Context Validation](#3-context-validation)
4. [File Locking System](#4-file-locking-system)
5. [Enhanced Resume with Auto-Recreation](#5-enhanced-resume-with-auto-recreation)
6. [Partial Failure Rollback](#6-partial-failure-rollback)
7. [Backup Command](#7-backup-command)
8. [Doctor Command](#8-doctor-command)
9. [Status Computation](#9-status-computation)
10. [Testing Strategy](#10-testing-strategy)
11. [Deployment Plan](#11-deployment-plan)

---

## 1. Manifest Schema v2 Implementation

### 1.1 Updated Manifest Struct

**File**: `internal/manifest/manifest.go`

```go
package manifest

import (
    "time"
)

const (
    SchemaVersion   = "2.0"  // Updated from "1.0"
    StatusActive    = "active"
    StatusStopped   = "stopped"
    StatusArchived  = "archived"
)

// Manifest represents a Claude session with tmux integration
type Manifest struct {
    SchemaVersion string    `yaml:"schema_version"`
    SessionID     string    `yaml:"session_id"`

    // NEW: Lifecycle field (only stores "archived", rest is computed)
    Lifecycle     string    `yaml:"lifecycle"`  // "" or "archived"

    CreatedAt     time.Time `yaml:"created_at"`
    LastActivity  time.Time `yaml:"last_activity"`

    // NEW: Context tracking (optional)
    Context       Context   `yaml:"context,omitempty"`

    Worktree      Worktree  `yaml:"worktree"`
    Claude        Claude    `yaml:"claude"`
    Tmux          Tmux      `yaml:"tmux"`
}

// NEW: Context struct with validation
type Context struct {
    Purpose string   `yaml:"purpose,omitempty"`  // max 256 chars
    Tags    []string `yaml:"tags,omitempty"`     // max 10 tags, each max 32 chars
    Notes   string   `yaml:"notes,omitempty"`    // max 1024 chars
}

// Worktree (unchanged from v1)
type Worktree struct {
    Path string `yaml:"path"`
}

// Claude (unchanged from v1)
type Claude struct {
    SessionID       string    `yaml:"session_id"`
    SessionEnvPath  string    `yaml:"session_env_path"`
    FileHistoryPath string    `yaml:"file_history_path"`
    StartedAt       time.Time `yaml:"started_at"`
    LastActivity    time.Time `yaml:"last_activity"`
}

// Tmux (unchanged from v1)
type Tmux struct {
    SessionName string    `yaml:"session_name"`
    WindowName  string    `yaml:"window_name"`
    CreatedAt   time.Time `yaml:"created_at"`
}

// NEW: GetStatus computes status dynamically
func (m *Manifest) GetStatus() string {
    if m.Lifecycle == "archived" {
        return StatusArchived
    }

    // Check if tmux session exists
    if tmux.SessionExists(m.Tmux.SessionName) {
        return StatusActive
    }

    return StatusStopped
}
```

### 1.2 Changes Summary

**Added**:
- `Lifecycle` field (string)
- `Context` struct with 3 optional fields
- `GetStatus()` method for dynamic status computation

**Unchanged**:
- All other fields remain the same
- YAML serialization compatible with v1 (minus removed fields)

**Migration Path**: v1 manifests will be automatically upgraded on first write

---

## 2. Schema Migration (v1 → v2)

### 2.1 Migration Logic

**File**: `internal/manifest/migrate.go` (NEW)

```go
package manifest

import (
    "fmt"
    "os"
    "path/filepath"
    "time"

    "gopkg.in/yaml.v3"
)

// migrateV1ToV2 converts a v1 manifest to v2
// Returns the migrated manifest and any error
func migrateV1ToV2(raw map[string]interface{}, manifestPath string) (*Manifest, error) {
    // STEP 1: Create backup of original manifest
    backupPath := manifestPath + ".v1.bak"
    err := copyFile(manifestPath, backupPath)
    if err != nil {
        return nil, fmt.Errorf("failed to create backup: %w", err)
    }

    fmt.Printf("📝 Migrating manifest to v2 (backup: %s)\n", filepath.Base(backupPath))

    // STEP 2: Parse v1 manifest (best effort)
    var m Manifest

    // Copy all known v1 fields
    if sessionID, ok := raw["session_id"].(string); ok {
        m.SessionID = sessionID
    }

    if createdAt, ok := raw["created_at"].(time.Time); ok {
        m.CreatedAt = createdAt
    }

    if lastActivity, ok := raw["last_activity"].(time.Time); ok {
        m.LastActivity = lastActivity
    }

    // Parse nested structs
    if worktree, ok := raw["worktree"].(map[string]interface{}); ok {
        if path, ok := worktree["path"].(string); ok {
            m.Worktree.Path = path
        }
    }

    if claude, ok := raw["claude"].(map[string]interface{}); ok {
        if sessionID, ok := claude["session_id"].(string); ok {
            m.Claude.SessionID = sessionID
        }
        // ... parse other Claude fields
    }

    if tmux, ok := raw["tmux"].(map[string]interface{}); ok {
        if sessionName, ok := tmux["session_name"].(string); ok {
            m.Tmux.SessionName = sessionName
        }
        // ... parse other Tmux fields
    }

    // STEP 3: Set v2 schema version
    m.SchemaVersion = "2.0"

    // STEP 4: Initialize new v2 fields
    m.Lifecycle = ""  // Will be computed dynamically
    m.Context = Context{
        Purpose: "",
        Tags:    []string{},
        Notes:   "",
    }

    // STEP 5: Write v2 manifest (atomic)
    err = writeAtomic(manifestPath, &m)
    if err != nil {
        // ROLLBACK: Restore from backup
        fmt.Printf("❌ Migration failed, rolling back...\n")
        restoreErr := copyFile(backupPath, manifestPath)
        if restoreErr != nil {
            return nil, fmt.Errorf("CRITICAL: migration failed AND rollback failed: %w (original error: %v)", restoreErr, err)
        }
        return nil, fmt.Errorf("migration failed (rolled back): %w", err)
    }

    fmt.Printf("✅ Migration successful (v1 → v2)\n")
    return &m, nil
}

// copyFile copies src to dst
func copyFile(src, dst string) error {
    data, err := os.ReadFile(src)
    if err != nil {
        return err
    }
    return os.WriteFile(dst, data, 0600)
}

// writeAtomic writes manifest atomically (temp file + rename)
func writeAtomic(path string, m *Manifest) error {
    // Write to temp file first
    tmpPath := path + ".tmp"

    data, err := yaml.Marshal(m)
    if err != nil {
        return err
    }

    err = os.WriteFile(tmpPath, data, 0600)
    if err != nil {
        return err
    }

    // Atomic rename
    return os.Rename(tmpPath, path)
}
```

### 2.2 Integration into Load()

**File**: `internal/manifest/manifest.go`

```go
// Load reads a manifest from disk, auto-migrating v1 to v2
func Load(path string) (*Manifest, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    // Parse as generic map first to check version
    var raw map[string]interface{}
    err = yaml.Unmarshal(data, &raw)
    if err != nil {
        return nil, fmt.Errorf("invalid YAML: %w", err)
    }

    // Check schema version
    version, _ := raw["schema_version"].(string)

    if version == "" || version == "1.0" {
        // V1 manifest - needs migration
        return migrateV1ToV2(raw, path)
    }

    if version != "2.0" {
        return nil, fmt.Errorf("unsupported schema version: %s", version)
    }

    // V2 manifest - parse normally
    var m Manifest
    err = yaml.Unmarshal(data, &m)
    if err != nil {
        return nil, fmt.Errorf("failed to parse manifest: %w", err)
    }

    return &m, nil
}
```

### 2.3 Migration User Messaging

**Output when migrating**:
```
📝 Migrating manifest to v2 (backup: manifest.yaml.v1.bak)
✅ Migration successful (v1 → v2)
```

**One-time notice** (first migration in a session):
```
ℹ️  CSM has upgraded to schema v2 for better session persistence.
   Your manifests have been automatically migrated.
   Backups are saved as *.v1.bak files (safe to delete after verification).
```

---

## 3. Context Validation

### 3.1 Validation Implementation

**File**: `internal/manifest/validate.go`

```go
package manifest

import (
    "fmt"
    "strings"
)

// Validate checks manifest and context field constraints
func (m *Manifest) Validate() error {
    // Existing validation (session_id, worktree, etc.)
    if m.SessionID == "" {
        return fmt.Errorf("session_id is required")
    }

    if m.Worktree.Path == "" {
        return fmt.Errorf("worktree.path is required")
    }

    // NEW: Context validation
    if err := m.Context.Validate(); err != nil {
        return fmt.Errorf("context validation failed: %w", err)
    }

    // NEW: Lifecycle validation
    if m.Lifecycle != "" && m.Lifecycle != "archived" {
        return fmt.Errorf("invalid lifecycle value: %q (must be empty or 'archived')", m.Lifecycle)
    }

    return nil
}

// Validate checks Context field constraints
func (c *Context) Validate() error {
    // Purpose: max 256 chars
    if len(c.Purpose) > 256 {
        return fmt.Errorf("purpose too long (%d chars, max 256)", len(c.Purpose))
    }

    // Tags: max 10 tags
    if len(c.Tags) > 10 {
        return fmt.Errorf("too many tags (%d, max 10)", len(c.Tags))
    }

    // Tags: each max 32 chars
    for i, tag := range c.Tags {
        if len(tag) > 32 {
            return fmt.Errorf("tag %d too long (%d chars, max 32): %q", i, len(tag), tag)
        }

        // Tags: no whitespace
        if strings.ContainsAny(tag, " \t\n") {
            return fmt.Errorf("tag %d contains whitespace: %q", i, tag)
        }
    }

    // Notes: max 1024 chars
    if len(c.Notes) > 1024 {
        return fmt.Errorf("notes too long (%d chars, max 1024)", len(c.Notes))
    }

    return nil
}

// Sanitize truncates fields to max length (instead of rejecting)
// Returns true if any fields were modified
func (c *Context) Sanitize() bool {
    modified := false

    if len(c.Purpose) > 256 {
        c.Purpose = c.Purpose[:256]
        modified = true
    }

    if len(c.Tags) > 10 {
        c.Tags = c.Tags[:10]
        modified = true
    }

    for i, tag := range c.Tags {
        if len(tag) > 32 {
            c.Tags[i] = tag[:32]
            modified = true
        }
    }

    if len(c.Notes) > 1024 {
        c.Notes = c.Notes[:1024]
        modified = true
    }

    return modified
}
```

### 3.2 Usage in Write()

```go
// Write saves manifest to disk with validation
func Write(path string, m *Manifest) error {
    // Validate before writing
    err := m.Validate()
    if err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Atomic write
    return writeAtomic(path, m)
}
```

---

## 4. File Locking System

### 4.1 Lock Implementation

**File**: `internal/manifest/lock.go` (NEW)

```go
package manifest

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// Lock represents a file lock for a manifest
type Lock struct {
    path string
    file *os.File
}

// AcquireLock attempts to acquire exclusive lock on manifest
// Returns error if lock is held by another process
func AcquireLock(manifestPath string) (*Lock, error) {
    lockPath := manifestPath + ".lock"

    // Try to create lock file (exclusive)
    file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
    if err != nil {
        if os.IsExist(err) {
            // Lock exists - check if stale
            if isLockStale(lockPath, 60*time.Second) {
                // Remove stale lock and retry
                os.Remove(lockPath)
                return AcquireLock(manifestPath)
            }

            // Lock is held
            return nil, fmt.Errorf("session is locked by another process (retry in a few seconds)")
        }
        return nil, fmt.Errorf("failed to create lock: %w", err)
    }

    // Write PID and timestamp to lock file
    fmt.Fprintf(file, "%d\n%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
    file.Sync()

    return &Lock{
        path: lockPath,
        file: file,
    }, nil
}

// Release removes the lock file
func (l *Lock) Release() error {
    if l == nil || l.file == nil {
        return nil
    }

    l.file.Close()
    return os.Remove(l.path)
}

// isLockStale checks if lock file is older than maxAge
func isLockStale(lockPath string, maxAge time.Duration) bool {
    info, err := os.Stat(lockPath)
    if err != nil {
        return true  // Can't stat = treat as stale
    }

    age := time.Since(info.ModTime())
    return age > maxAge
}

// FindStaleLocks finds all stale lock files in directory
func FindStaleLocks(dir string, maxAge time.Duration) ([]string, error) {
    var staleLocks []string

    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, err
    }

    for _, entry := range entries {
        if !entry.IsDir() && filepath.Ext(entry.Name()) == ".lock" {
            lockPath := filepath.Join(dir, entry.Name())
            if isLockStale(lockPath, maxAge) {
                staleLocks = append(staleLocks, lockPath)
            }
        }
    }

    return staleLocks, nil
}
```

### 4.2 Usage in Resume Command

```go
// cmd/csm/resume.go
func resumeCmd(identifier string) error {
    uuid, manifest, manifestPath := resolveSessionIdentifier(identifier)

    // Acquire lock (blocks concurrent modifications)
    lock, err := manifest.AcquireLock(manifestPath)
    if err != nil {
        return fmt.Errorf("could not acquire lock: %w", err)
    }
    defer lock.Release()

    // Now safe to modify session
    // ...
}
```

---

## 5. Enhanced Resume with Auto-Recreation

### 5.1 Updated Resume Flow

**File**: `cmd/csm/resume.go`

```go
func resumeCmd(cmd *cobra.Command, args []string) error {
    // Get identifier from args
    var identifier string
    if len(args) > 0 {
        identifier = args[0]
    } else {
        return fmt.Errorf("interactive picker not yet implemented - please provide identifier")
    }

    // Resolve identifier to manifest
    uuid, manifest, manifestPath, err := resolveSessionIdentifier(identifier)
    if err != nil {
        ui.PrintError(err, "Failed to resolve session identifier", "")
        return err
    }

    ui.PrintSuccess(fmt.Sprintf("Resolved %q to UUID: %s", identifier, uuid[:8]))

    // Acquire lock
    lock, err := manifest.AcquireLock(manifestPath)
    if err != nil {
        return fmt.Errorf("could not acquire lock: %w", err)
    }
    defer lock.Release()

    // Handle archived sessions
    if manifest.Lifecycle == "archived" {
        ui.PrintWarning("This session is archived")
        confirm := ui.Confirm("Unarchive and resume?")
        if !confirm {
            return fmt.Errorf("session is archived")
        }
        manifest.Lifecycle = ""  // Unarchive
        ui.PrintSuccess("Session unarchived")
    }

    // Compute current status
    status := manifest.GetStatus()

    switch status {
    case manifest.StatusActive:
        // Tmux exists - just attach
        ui.PrintSuccess("Session is active")

    case manifest.StatusStopped:
        // Tmux missing - recreate
        ui.PrintWarning("Session stopped (tmux missing), recreating...")

        err := recreateTmuxSession(manifest)
        if err != nil {
            ui.PrintError(err, "Failed to recreate session", "")
            return err
        }

        ui.PrintSuccess("Session recreated successfully")
    }

    // Update last activity
    manifest.LastActivity = time.Now()
    err = manifest.Write(manifestPath, manifest)
    if err != nil {
        ui.PrintWarning(fmt.Sprintf("Could not update manifest: %v", err))
    }

    // Attach to session
    ui.PrintInfo(fmt.Sprintf("Attaching to tmux session: %s", manifest.Tmux.SessionName))
    return tmux.AttachSession(manifest.Tmux.SessionName)
}
```

### 5.2 Tmux Recreation Logic

```go
// recreateTmuxSession creates a new tmux session for a stopped session
func recreateTmuxSession(m *manifest.Manifest) error {
    // STEP 1: Validate worktree exists
    if _, err := os.Stat(m.Worktree.Path); os.IsNotExist(err) {
        return fmt.Errorf(
            "worktree does not exist: %s\n\n"+
            "Suggestions:\n"+
            "  • Update worktree path (if moved): csm set %s --worktree /new/path\n"+
            "  • Archive this session: csm archive %s\n"+
            "  • Resume in current directory: csm resume %s --here",
            m.Worktree.Path,
            m.Tmux.SessionName,
            m.Tmux.SessionName,
            m.Tmux.SessionName,
        )
    }

    // STEP 2: Create tmux session
    err := tmux.NewSession(m.Tmux.SessionName, m.Worktree.Path)
    if err != nil {
        return fmt.Errorf("failed to create tmux session: %w", err)
    }

    // STEP 3: Resume Claude in tmux
    claudeCmd := fmt.Sprintf("claude --resume %s", m.Claude.SessionID)
    err = tmux.SendCommand(m.Tmux.SessionName, claudeCmd)
    if err != nil {
        // ROLLBACK: Kill tmux session we just created
        _ = tmux.KillSession(m.Tmux.SessionName)
        return fmt.Errorf("failed to start Claude: %w", err)
    }

    return nil
}
```

---

## 6. Partial Failure Rollback

### 6.1 Tmux Rollback

**File**: `internal/tmux/tmux.go`

```go
// KillSession terminates a tmux session
func KillSession(name string) error {
    cmd := exec.Command("tmux", "kill-session", "-t", name)
    return cmd.Run()
}
```

### 6.2 Usage in Recreation

Already shown in section 5.2:
```go
err = tmux.SendCommand(m.Tmux.SessionName, claudeCmd)
if err != nil {
    // ROLLBACK: Kill tmux session we just created
    _ = tmux.KillSession(m.Tmux.SessionName)
    return fmt.Errorf("failed to start Claude: %w", err)
}
```

---

## 7. Backup Command

### 7.1 Command Implementation

**File**: `cmd/csm/backup.go` (NEW)

```go
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/spf13/cobra"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var backupCmd = &cobra.Command{
    Use:   "backup [identifier]",
    Short: "Backup session logs and conversation history",
    Long: `Backup a Claude session's conversation history and metadata.

Creates a timestamped backup directory containing:
- Session manifest snapshot
- Conversation history (JSONL or Markdown)
- Optionally: file snapshots from file-history/

Examples:
  csm backup claude-1                    # Backup to JSONL
  csm backup claude-1 --format markdown  # Export as Markdown
  csm backup claude-1 --include-files    # Include file snapshots`,
    RunE: func(cmd *cobra.Command, args []string) error {
        if len(args) == 0 {
            return fmt.Errorf("session identifier required")
        }

        identifier := args[0]
        format, _ := cmd.Flags().GetString("format")
        includeFiles, _ := cmd.Flags().GetBool("include-files")

        return runBackup(identifier, format, includeFiles)
    },
}

func runBackup(identifier string, format string, includeFiles bool) error {
    // Resolve identifier
    uuid, manifest, manifestPath, err := resolveSessionIdentifier(identifier)
    if err != nil {
        return err
    }

    ui.PrintSuccess(fmt.Sprintf("Backing up session: %s (%s)", manifest.Tmux.SessionName, uuid[:8]))

    // Create backup directory
    timestamp := time.Now().Format("2006-01-02_15-04-05")
    backupDir := filepath.Join(
        filepath.Dir(manifestPath),
        "backups",
        timestamp,
    )

    err = os.MkdirAll(backupDir, 0700)
    if err != nil {
        return fmt.Errorf("failed to create backup directory: %w", err)
    }

    // 1. Copy manifest
    manifestBackup := filepath.Join(backupDir, "session-info.yaml")
    err = copyFile(manifestPath, manifestBackup)
    if err != nil {
        return fmt.Errorf("failed to backup manifest: %w", err)
    }
    ui.PrintSuccess(fmt.Sprintf("✓ Manifest: %s", manifestBackup))

    // 2. Extract conversation history
    historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
    entries, _, err := claude.ParseHistory(historyPath)
    if err != nil {
        return fmt.Errorf("failed to parse history: %w", err)
    }

    // Filter entries for this session
    var sessionEntries []claude.RawEntry
    for _, entry := range entries {
        if entry.SessionID == manifest.Claude.SessionID {
            sessionEntries = append(sessionEntries, entry)
        }
    }

    ui.PrintSuccess(fmt.Sprintf("✓ Found %d messages", len(sessionEntries)))

    // 3. Write conversation in requested format
    if format == "markdown" {
        mdPath := filepath.Join(backupDir, "conversation.md")
        err = writeMarkdown(sessionEntries, mdPath, manifest)
        if err != nil {
            return fmt.Errorf("failed to write markdown: %w", err)
        }
        ui.PrintSuccess(fmt.Sprintf("✓ Conversation: %s", mdPath))
    } else {
        jsonlPath := filepath.Join(backupDir, "conversation.jsonl")
        err = writeJSONL(sessionEntries, jsonlPath)
        if err != nil {
            return fmt.Errorf("failed to write JSONL: %w", err)
        }
        ui.PrintSuccess(fmt.Sprintf("✓ Conversation: %s", jsonlPath))
    }

    // 4. Optionally copy file snapshots
    if includeFiles {
        fileHistoryPath := manifest.Claude.FileHistoryPath
        if _, err := os.Stat(fileHistoryPath); err == nil {
            snapshotsDir := filepath.Join(backupDir, "file-snapshots")
            err = copyDirectory(fileHistoryPath, snapshotsDir)
            if err != nil {
                ui.PrintWarning(fmt.Sprintf("Failed to copy file snapshots: %v", err))
            } else {
                ui.PrintSuccess(fmt.Sprintf("✓ File snapshots: %s", snapshotsDir))
            }
        }
    }

    ui.PrintSuccess(fmt.Sprintf("\n✓ Backup complete: %s", backupDir))
    return nil
}

func init() {
    backupCmd.Flags().String("format", "jsonl", "Output format: jsonl or markdown")
    backupCmd.Flags().Bool("include-files", false, "Include file snapshots from file-history/")

    rootCmd.AddCommand(backupCmd)
}
```

### 7.2 Markdown Export

```go
// writeMarkdown exports conversation as human-readable Markdown
func writeMarkdown(entries []claude.RawEntry, path string, m *manifest.Manifest) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()

    // Header
    fmt.Fprintf(f, "# Session: %s\n\n", m.Tmux.SessionName)
    fmt.Fprintf(f, "**UUID**: %s\n", m.Claude.SessionID)
    fmt.Fprintf(f, "**Project**: %s\n", m.Worktree.Path)
    fmt.Fprintf(f, "**Messages**: %d\n", len(entries))
    fmt.Fprintf(f, "**Created**: %s\n\n", m.CreatedAt.Format(time.RFC1123))

    if m.Context.Purpose != "" {
        fmt.Fprintf(f, "**Purpose**: %s\n\n", m.Context.Purpose)
    }

    fmt.Fprintf(f, "---\n\n")

    // Messages
    for i, entry := range entries {
        timestamp := time.Unix(0, int64(entry.Timestamp)*int64(time.Millisecond))
        fmt.Fprintf(f, "## Message %d - %s\n\n", i+1, timestamp.Format("2006-01-02 15:04:05"))
        fmt.Fprintf(f, "%s\n\n", entry.Display)
        fmt.Fprintf(f, "---\n\n")
    }

    return nil
}
```

---

## 8. Doctor Command

### 8.1 Command Implementation

**File**: `cmd/csm/doctor.go` (NEW)

Already specified in detail in D2-ARCHITECTURE-v2.md section 7. Implementation follows the spec exactly.

Key checks:
1. Sessions directory exists
2. All manifests load and validate
3. Stale lock files (with --fix option)
4. Claude UUIDs exist in history.jsonl
5. Worktrees exist
6. Migration backups (with --check-migrations)

---

## 9. Status Computation

### 9.1 GetStatus Implementation

Already shown in section 1.1. Additional helper in tmux package:

**File**: `internal/tmux/tmux.go`

```go
// SessionExists checks if a tmux session exists
func SessionExists(name string) bool {
    cmd := exec.Command("tmux", "has-session", "-t", name)
    err := cmd.Run()
    return err == nil  // Exit code 0 = exists
}
```

### 9.2 Batch Status Check for List Command

```go
// GetSessionStatuses returns status for multiple sessions efficiently
func GetSessionStatuses(manifests []*manifest.Manifest) map[string]string {
    // Get all tmux sessions once (batch)
    tmuxSessions, err := tmux.ListSessions()
    if err != nil {
        // No tmux server running = all stopped
        statuses := make(map[string]string)
        for _, m := range manifests {
            statuses[m.SessionID] = m.GetStatus()  // Will be "stopped" or "archived"
        }
        return statuses
    }

    // Build lookup map
    tmuxMap := make(map[string]bool)
    for _, name := range tmuxSessions {
        tmuxMap[name] = true
    }

    // Compute statuses
    statuses := make(map[string]string)
    for _, m := range manifests {
        if m.Lifecycle == "archived" {
            statuses[m.SessionID] = "archived"
        } else if tmuxMap[m.Tmux.SessionName] {
            statuses[m.SessionID] = "active"
        } else {
            statuses[m.SessionID] = "stopped"
        }
    }

    return statuses
}
```

---

## 10. Testing Strategy

### 10.1 Unit Tests

**File**: `internal/manifest/manifest_test.go`

```go
func TestContextValidation(t *testing.T) {
    tests := []struct {
        name    string
        context manifest.Context
        wantErr bool
    }{
        {
            name: "valid context",
            context: manifest.Context{
                Purpose: "Test purpose",
                Tags:    []string{"test", "feature"},
                Notes:   "Some notes",
            },
            wantErr: false,
        },
        {
            name: "purpose too long",
            context: manifest.Context{
                Purpose: strings.Repeat("a", 257),
            },
            wantErr: true,
        },
        {
            name: "too many tags",
            context: manifest.Context{
                Tags: make([]string, 11),
            },
            wantErr: true,
        },
        {
            name: "tag too long",
            context: manifest.Context{
                Tags: []string{strings.Repeat("a", 33)},
            },
            wantErr: true,
        },
        {
            name: "notes too long",
            context: manifest.Context{
                Notes: strings.Repeat("a", 1025),
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.context.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

func TestMigrationV1ToV2(t *testing.T) {
    // Create temp directory for test
    tmpDir := t.TempDir()

    // Create v1 manifest
    v1Manifest := map[string]interface{}{
        "schema_version": "1.0",
        "session_id":     "test-session",
        "worktree": map[string]interface{}{
            "path": "/tmp/test",
        },
        // ... more v1 fields
    }

    manifestPath := filepath.Join(tmpDir, "manifest.yaml")
    data, _ := yaml.Marshal(v1Manifest)
    os.WriteFile(manifestPath, data, 0600)

    // Load (should trigger migration)
    m, err := manifest.Load(manifestPath)
    if err != nil {
        t.Fatalf("Load() failed: %v", err)
    }

    // Verify migration
    if m.SchemaVersion != "2.0" {
        t.Errorf("Expected schema v2, got %s", m.SchemaVersion)
    }

    // Verify backup exists
    backupPath := manifestPath + ".v1.bak"
    if _, err := os.Stat(backupPath); os.IsNotExist(err) {
        t.Error("Backup file not created")
    }
}
```

### 10.2 Integration Tests

**File**: `cmd/csm/resume_test.go`

```go
func TestResumeWithAutoRecreation(t *testing.T) {
    // Setup: Create manifest for stopped session
    tmpDir := t.TempDir()
    manifestPath := filepath.Join(tmpDir, "manifest.yaml")

    m := &manifest.Manifest{
        SchemaVersion: "2.0",
        SessionID:     "test-session",
        Lifecycle:     "",
        Worktree: manifest.Worktree{
            Path: tmpDir,  // Use temp dir as worktree
        },
        Claude: manifest.Claude{
            SessionID: "c4eb298c-8c89-4f75-8dae-c725a1291add",
        },
        Tmux: manifest.Tmux{
            SessionName: "test-tmux-resume",
        },
    }

    manifest.Write(manifestPath, m)

    // Ensure tmux session doesn't exist
    tmux.KillSession("test-tmux-resume")

    // Run resume (should recreate tmux)
    err := resumeCmd(nil, []string{"test-tmux-resume"})

    // Verify tmux was created
    exists := tmux.SessionExists("test-tmux-resume")
    if !exists {
        t.Error("Tmux session was not recreated")
    }

    // Cleanup
    tmux.KillSession("test-tmux-resume")
}

func TestConcurrentResume(t *testing.T) {
    // Test that concurrent resume operations don't corrupt manifest
    // Use goroutines to simulate concurrent access

    tmpDir := t.TempDir()
    manifestPath := filepath.Join(tmpDir, "manifest.yaml")

    // Create manifest
    m := createTestManifest()
    manifest.Write(manifestPath, m)

    // Launch 5 concurrent resume operations
    errChan := make(chan error, 5)
    for i := 0; i < 5; i++ {
        go func() {
            err := resumeCmd(nil, []string{"test-session"})
            errChan <- err
        }()
    }

    // Collect results
    successCount := 0
    lockFailCount := 0
    for i := 0; i < 5; i++ {
        err := <-errChan
        if err == nil {
            successCount++
        } else if strings.Contains(err.Error(), "locked") {
            lockFailCount++
        }
    }

    // At least one should succeed, others should get lock error
    if successCount < 1 {
        t.Error("No resume operations succeeded")
    }
    if successCount + lockFailCount != 5 {
        t.Error("Unexpected error types")
    }
}
```

### 10.3 Test Coverage Goals

- **Unit tests**: > 80% coverage
- **Integration tests**: All critical paths
- **Edge cases**: All identified in D2 review

---

## 11. Deployment Plan

### 11.1 Staged Rollout

**Stage 1: Schema + Migration (Days 1-2)**
- Implement manifest v2 structs
- Implement migration logic
- Add tests for migration
- Deploy (users get auto-migration on first use)

**Stage 2: Resume + Locking (Days 3-4)**
- Implement file locking
- Enhance resume command
- Add rollback logic
- Deploy (resume now auto-recreates)

**Stage 3: Backup + Doctor (Days 5-6)**
- Implement backup command
- Implement doctor command
- Final integration tests
- Deploy (all Phase 3.5 features complete)

### 11.2 Rollback Plan

**If migration fails**:
1. Backup files (.v1.bak) provide recovery
2. User can manually restore: `mv manifest.yaml.v1.bak manifest.yaml`
3. Report bug, wait for fix, try again

**If critical bug found**:
1. Git revert to previous version
2. Users with v2 manifests can continue (forward compatible)
3. Fix bug, redeploy

### 11.3 User Communication

**Before deployment**:
- Update CHANGELOG.md with breaking changes
- Document migration process
- Announce in release notes

**During deployment**:
- Auto-migration messages in CLI
- Link to migration guide if issues occur

**After deployment**:
- Monitor for migration issues
- Provide `csm doctor --check-migrations` for verification

---

## 12. Success Criteria

### 12.1 Functional Criteria

1. ✅ All 11 deliverables implemented and tested
2. ✅ Migration from v1 to v2 works transparently
3. ✅ Resume auto-recreates tmux after "reboot" (kill tmux)
4. ✅ File locking prevents corruption
5. ✅ Backup command creates usable backups
6. ✅ Doctor command catches common issues
7. ✅ All tests pass (unit + integration)

### 12.2 Performance Criteria

1. ✅ Migration adds < 100ms to first load
2. ✅ Resume auto-recreation < 3 seconds
3. ✅ csm list with 50 sessions < 1 second
4. ✅ Backup of 200-message session < 5 seconds

### 12.3 Quality Criteria

1. ✅ No data loss during migration
2. ✅ No manifest corruption from concurrent access
3. ✅ Clear error messages for all failure modes
4. ✅ Code coverage > 80%

---

## 13. Open Questions

### Q1: Should we support v1 manifests indefinitely?
**Recommendation**: Support v1 read for 6 months, then require migration

### Q2: Should backup be automatic on archive?
**Recommendation**: No - keep it explicit. Document workflow: backup then archive

### Q3: Should we add `csm config` command in this phase?
**Recommendation**: Defer to Phase 4 - not critical for persistence

---

## Next Steps for Multi-Persona Review

This implementation design is ready for review. Key areas for reviewers:

1. **Code structure**: Are the changes well-organized?
2. **Error handling**: Are all failure modes covered?
3. **Testing**: Is test coverage adequate?
4. **Deployment**: Is the rollout plan safe?
5. **Performance**: Any performance concerns?

**Target**: ≥8.5/10 approval to proceed to D4 (Requirements)
