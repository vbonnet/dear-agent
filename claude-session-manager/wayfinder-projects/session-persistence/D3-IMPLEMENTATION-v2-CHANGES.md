# D3 Implementation v2 - Key Changes

**Date**: December 7, 2025
**Based on**: R1 Feedback (7.7/10 → targeting 8.5/10)

---

## Critical Fixes (Must Fix)

### 1. Remove tmux Dependency from Manifest Package

**Problem**: Circular dependency, manifest calls tmux.SessionExists()

**Solution**: Move status computation to cmd layer

```go
// BEFORE (manifest/manifest.go)
func (m *Manifest) GetStatus() string {
    if m.Lifecycle == "archived" {
        return StatusArchived
    }
    if tmux.SessionExists(m.Tmux.SessionName) {  // BAD: depends on tmux
        return StatusActive
    }
    return StatusStopped
}

// AFTER (cmd/csm/status.go - NEW FILE)
package main

import (
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

// ComputeStatus determines session status based on lifecycle and tmux state
func ComputeStatus(m *manifest.Manifest) string {
    if m.Lifecycle == manifest.LifecycleArchived {
        return manifest.StatusArchived
    }
    if tmux.SessionExists(m.Tmux.SessionName) {
        return manifest.StatusActive
    }
    return manifest.StatusStopped
}

// ComputeStatuses batch-computes status for multiple sessions
func ComputeStatuses(manifests []*manifest.Manifest) map[string]string {
    // Get all tmux sessions once
    tmuxSessions, err := tmux.ListSessions()
    tmuxMap := make(map[string]bool)
    if err == nil {
        for _, name := range tmuxSessions {
            tmuxMap[name] = true
        }
    }

    statuses := make(map[string]string)
    for _, m := range manifests {
        if m.Lifecycle == manifest.LifecycleArchived {
            statuses[m.SessionID] = manifest.StatusArchived
        } else if tmuxMap[m.Tmux.SessionName] {
            statuses[m.SessionID] = manifest.StatusActive
        } else {
            statuses[m.SessionID] = manifest.StatusStopped
        }
    }

    return statuses
}
```

**Impact**: Cleaner architecture, manifest is pure data model

---

### 2. Add Constants for Magic Values

**File**: `internal/manifest/constants.go` (NEW)

```go
package manifest

import "time"

// Schema and lifecycle constants
const (
    SchemaVersion      = "2.0"
    LifecycleActive    = ""  // Empty string means active/stopped (computed)
    LifecycleArchived  = "archived"
)

// Status constants (computed, not stored)
const (
    StatusActive   = "active"
    StatusStopped  = "stopped"
    StatusArchived = "archived"
)

// Validation limits for Context fields
const (
    MaxPurposeLen = 256
    MaxTagsCount  = 10
    MaxTagLen     = 32
    MaxNotesLen   = 1024
)

// Lock configuration
const (
    LockTimeout = 60 * time.Second
)

// Backup configuration
const (
    MaxBackupsPerSession = 10  // Keep only last N backups
)
```

**Usage in validation**:
```go
func (c *Context) Validate() error {
    if len(c.Purpose) > MaxPurposeLen {
        return fmt.Errorf("purpose too long (%d chars, max %d)", len(c.Purpose), MaxPurposeLen)
    }
    // ... use other constants
}
```

---

### 3. Fix Migration Error Handling

**Problem**: Type assertions not checked, partial initialization risk

**Solution**: Atomic struct initialization with full validation

```go
// internal/manifest/migrate.go
func migrateV1ToV2(raw map[string]interface{}, manifestPath string) (*Manifest, error) {
    // Create backup first
    backupPath := manifestPath + ".v1.bak"
    if _, err := os.Stat(backupPath); err == nil {
        // Backup already exists - migration already attempted
        return nil, fmt.Errorf("migration backup already exists (previous migration may have failed)")
    }

    err := copyFile(manifestPath, backupPath)
    if err != nil {
        return nil, fmt.Errorf("failed to create backup: %w", err)
    }

    if shouldShowMigrationMessage() {
        fmt.Printf("📝 Migrating manifest to v2 (backup: %s)\n", filepath.Base(backupPath))
    }

    // Parse into temporary struct (all or nothing)
    temp := struct {
        SessionID    string                 `yaml:"session_id"`
        CreatedAt    time.Time              `yaml:"created_at"`
        LastActivity time.Time              `yaml:"last_activity"`
        Worktree     map[string]interface{} `yaml:"worktree"`
        Claude       map[string]interface{} `yaml:"claude"`
        Tmux         map[string]interface{} `yaml:"tmux"`
    }{}

    // Unmarshal with strict validation
    data, err := os.ReadFile(manifestPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read manifest: %w", err)
    }

    err = yaml.UnmarshalStrict(data, &temp)
    if err != nil {
        return nil, fmt.Errorf("failed to parse v1 manifest: %w", err)
    }

    // Build v2 manifest (validated)
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
    }

    // Parse nested structs with validation
    if path, ok := temp.Worktree["path"].(string); ok {
        m.Worktree.Path = path
    } else {
        return nil, fmt.Errorf("invalid worktree.path in v1 manifest")
    }

    if sessionID, ok := temp.Claude["session_id"].(string); ok {
        m.Claude.SessionID = sessionID
    } else {
        return nil, fmt.Errorf("invalid claude.session_id in v1 manifest")
    }
    // ... parse other Claude fields with validation

    if sessionName, ok := temp.Tmux["session_name"].(string); ok {
        m.Tmux.SessionName = sessionName
    } else {
        return nil, fmt.Errorf("invalid tmux.session_name in v1 manifest")
    }
    // ... parse other Tmux fields

    // Validate complete manifest
    if err := m.Validate(); err != nil {
        return nil, fmt.Errorf("migrated manifest is invalid: %w", err)
    }

    // Write v2 (atomic)
    err = writeAtomic(manifestPath, m)
    if err != nil {
        // ROLLBACK
        if shouldShowMigrationMessage() {
            fmt.Printf("❌ Migration failed, rolling back...\n")
        }
        restoreErr := copyFile(backupPath, manifestPath)
        if restoreErr != nil {
            return nil, fmt.Errorf("CRITICAL: migration failed AND rollback failed: %w (original error: %v)", restoreErr, err)
        }
        return nil, fmt.Errorf("migration failed (rolled back): %w", err)
    }

    // Log migration success
    logMigration(manifestPath, true, nil)

    if shouldShowMigrationMessage() {
        fmt.Printf("✅ Migration successful (v1 → v2)\n")
    }

    return m, nil
}
```

---

### 4. Add Missing Edge Case Tests

**File**: `internal/manifest/manifest_migration_test.go` (NEW)

```go
func TestMigrationRollback(t *testing.T) {
    tmpDir := t.TempDir()
    manifestPath := filepath.Join(tmpDir, "manifest.yaml")

    // Create v1 manifest
    v1Data := createV1ManifestYAML()
    os.WriteFile(manifestPath, v1Data, 0600)

    // Inject write failure (make directory read-only)
    os.Chmod(tmpDir, 0500)
    defer os.Chmod(tmpDir, 0700)

    // Attempt migration (should fail and rollback)
    _, err := Load(manifestPath)
    if err == nil {
        t.Fatal("Expected migration to fail")
    }

    // Verify rollback: original manifest restored
    data, _ := os.ReadFile(manifestPath)
    if !bytes.Equal(data, v1Data) {
        t.Error("Rollback failed - original manifest not restored")
    }

    // Verify backup exists
    backupPath := manifestPath + ".v1.bak"
    if _, err := os.Stat(backupPath); os.IsNotExist(err) {
        t.Error("Backup file not created")
    }
}

func TestMigrationWithPartialV1Data(t *testing.T) {
    // Test v1 manifest missing optional fields
    // Should migrate successfully with defaults
}

func TestMigrationBackupCollision(t *testing.T) {
    // Test .v1.bak already exists
    // Should fail (not overwrite previous backup)
}

func TestBackupFilenameCollision(t *testing.T) {
    // Create two backups in same second
    // Verify no data loss (add microseconds to timestamp)
}

func TestConcurrentDoctorAndResume(t *testing.T) {
    // Run doctor --fix and resume simultaneously
    // Verify no lock file conflicts
}
```

---

### 5. Add Migration Logging

**File**: `internal/manifest/migrate.go`

```go
// logMigration persists migration result to log file
func logMigration(manifestPath string, success bool, err error) {
    logDir := filepath.Join(os.Getenv("HOME"), ".csm", "logs")
    os.MkdirAll(logDir, 0700)

    logFile := filepath.Join(logDir, "migration.log")
    f, ferr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if ferr != nil {
        return  // Best effort logging
    }
    defer f.Close()

    timestamp := time.Now().Format(time.RFC3339)
    if success {
        fmt.Fprintf(f, "[%s] SUCCESS: %s\n", timestamp, manifestPath)
    } else {
        fmt.Fprintf(f, "[%s] FAILED: %s - %v\n", timestamp, manifestPath, err)
    }
}

// shouldShowMigrationMessage checks if we're in interactive terminal
func shouldShowMigrationMessage() bool {
    fileInfo, err := os.Stdout.Stat()
    if err != nil {
        return false
    }
    return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
```

---

## Strongly Recommended Fixes

### 6. Create Fileutil Package

**File**: `internal/fileutil/fileutil.go` (NEW)

```go
package fileutil

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
)

// CopyFile copies src to dst with validation
func CopyFile(src, dst string) error {
    if src == dst {
        return fmt.Errorf("source and destination are the same")
    }

    srcInfo, err := os.Stat(src)
    if err != nil {
        return fmt.Errorf("source file error: %w", err)
    }

    if srcInfo.IsDir() {
        return fmt.Errorf("source is a directory, use CopyDirectory")
    }

    // Open source
    srcFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()

    // Create destination
    dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
    if err != nil {
        return err
    }
    defer dstFile.Close()

    // Copy
    _, err = io.Copy(dstFile, srcFile)
    return err
}

// WriteAtomic writes data to path atomically (temp file + rename)
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
    tmpPath := path + ".tmp"

    err := os.WriteFile(tmpPath, data, perm)
    if err != nil {
        return err
    }

    return os.Rename(tmpPath, path)
}

// CopyDirectory recursively copies src directory to dst
func CopyDirectory(src, dst string) error {
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        relPath, err := filepath.Rel(src, path)
        if err != nil {
            return err
        }

        dstPath := filepath.Join(dst, relPath)

        if info.IsDir() {
            return os.MkdirAll(dstPath, info.Mode())
        }

        return CopyFile(path, dstPath)
    })
}
```

---

### 7. Add Backup Retention

**File**: `cmd/csm/backup.go`

```go
func runBackup(identifier string, format string, includeFiles bool) error {
    // ... create backup ...

    // Clean old backups (keep last 10)
    backupsBaseDir := filepath.Join(filepath.Dir(manifestPath), "backups")
    err = cleanOldBackups(backupsBaseDir, manifest.MaxBackupsPerSession)
    if err != nil {
        ui.PrintWarning(fmt.Sprintf("Could not clean old backups: %v", err))
    }

    // Create 'latest' symlink
    latestLink := filepath.Join(backupsBaseDir, "latest")
    os.Remove(latestLink)  // Remove old symlink
    os.Symlink(filepath.Base(backupDir), latestLink)

    ui.PrintSuccess(fmt.Sprintf("\n✓ Backup complete: %s", backupDir))
    ui.PrintInfo(fmt.Sprintf("   Latest: %s/latest", backupsBaseDir))
    return nil
}

// cleanOldBackups keeps only the last N backups
func cleanOldBackups(backupsDir string, keep int) error {
    entries, err := os.ReadDir(backupsDir)
    if err != nil {
        return err
    }

    // Filter out non-directories and 'latest' symlink
    var backups []os.DirEntry
    for _, entry := range entries {
        if entry.IsDir() {
            backups = append(backups, entry)
        }
    }

    // Sort by name (which is timestamp)
    sort.Slice(backups, func(i, j int) bool {
        return backups[i].Name() > backups[j].Name()  // Newest first
    })

    // Delete old backups
    for i := keep; i < len(backups); i++ {
        oldBackup := filepath.Join(backupsDir, backups[i].Name())
        err := os.RemoveAll(oldBackup)
        if err != nil {
            return fmt.Errorf("failed to remove old backup %s: %w", oldBackup, err)
        }
    }

    return nil
}
```

---

### 8. Improve UX for Migration Messages

Already shown in section 5 (shouldShowMigrationMessage)

Additional improvement: Show one-time notice

```go
// File: cmd/csm/main.go
func init() {
    // Check if first migration in this session
    noticeFile := filepath.Join(os.Getenv("HOME"), ".csm", ".migration-notice-shown")
    if _, err := os.Stat(noticeFile); os.IsNotExist(err) {
        // Show one-time notice
        fmt.Println("ℹ️  CSM has upgraded to schema v2 for better session persistence.")
        fmt.Println("   Your manifests will be automatically migrated.")
        fmt.Println("   Backups are saved as *.v1.bak files (safe to delete after verification).")
        fmt.Println()

        // Create notice file
        os.MkdirAll(filepath.Dir(noticeFile), 0700)
        os.WriteFile(noticeFile, []byte(time.Now().String()), 0600)
    }
}
```

---

### 9. Add Context.Context Support

**File**: `cmd/csm/backup.go`

```go
func runBackup(ctx context.Context, identifier string, format string, includeFiles bool) error {
    // Check cancellation at key points
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // ... resolve identifier ...

    // Check before heavy operation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // ... parse history (could be slow) ...

    // Check before copy
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // ... copy files ...

    return nil
}

// Update command to use context
var backupCmd = &cobra.Command{
    Use: "backup [identifier]",
    RunE: func(cmd *cobra.Command, args []string) error {
        ctx := cmd.Context()
        // ...
        return runBackup(ctx, identifier, format, includeFiles)
    },
}
```

---

## Summary of Changes

### New Files
1. `cmd/csm/status.go` - Status computation (removed from manifest)
2. `internal/manifest/constants.go` - All magic values
3. `internal/fileutil/fileutil.go` - File operation utilities
4. `internal/manifest/manifest_migration_test.go` - Migration edge cases

### Modified Files
1. `internal/manifest/manifest.go` - Remove GetStatus method
2. `internal/manifest/migrate.go` - Better error handling, logging
3. `internal/manifest/validate.go` - Use constants
4. `cmd/csm/backup.go` - Add retention, context support
5. `cmd/csm/doctor.go` - Add quiet mode
6. `cmd/csm/resume.go` - Use ComputeStatus()

### Testing Additions
- Migration rollback tests
- Backup collision tests
- Concurrent operation tests
- Edge case coverage

### Documentation Additions
- Migration logging to ~/.csm/logs/migration.log
- Backup retention (keep last 10)
- Latest symlink for easy access
- One-time migration notice

---

**Expected Improvement**: 7.7/10 → 8.5+/10
