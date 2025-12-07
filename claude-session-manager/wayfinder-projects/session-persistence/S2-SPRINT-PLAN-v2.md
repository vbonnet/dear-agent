# S2: Sprint 2 - Enhanced Resume & Backup (v2)

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Round 2
**Version**: 2.0
**Sprint Goal**: Implement enhanced resume with auto-recreation and session backup
**Prerequisites**:
- S1 Foundation ✅ Complete (schema v2, migration, locking, validation, fileutil)
- D4 Requirements ✅ Approved (9.3/10)
- S2 Round 1 ❌ Revision needed (7.8/10)

---

## Executive Summary

Sprint 2 builds on the foundation from S1 to implement **user-facing features** that enable session persistence across reboots. This includes automatic tmux session recreation, dynamic status computation, and session backup functionality.

**Scope**: 3 deliverables (of 11 total in Phase 3.5)
**Duration Estimate**: 2-3 days of focused development
**Dependencies**: S1 (manifest schema, locking, migration, fileutil)

**Changes from v1**:
- User messages & error specifications added (like S1)
- Backup file permissions specified (0600/0700)
- Tmux command sanitization added
- Path validation added
- Concurrent operation tests added (TS-S2-11 through TS-S2-16)
- Post-deployment verification checklist added
- Rollback procedure added
- Help text drafts added
- Monitoring guidance added
- Backup timestamp always includes microseconds
- History.jsonl parsing strategy specified
- Backup atomic creation specified

**Strategic Rationale**: S2 delivers the core user value proposition - sessions that survive reboots. With auto-recreation and backup, users can confidently restart their machines knowing their Claude sessions will be restored. This is the "killer feature" that makes session persistence valuable.

---

## Sprint Goal

**Primary Goal**: Enable sessions to survive reboots through automatic tmux recreation and preserve conversation history through backups.

**Success Criteria**:
1. ✅ After reboot, `csm resume` recreates stopped sessions automatically
2. ✅ Users can backup session conversations in JSONL or Markdown
3. ✅ Session status computed dynamically (never stale)
4. ✅ All operations respect locks from S1
5. ✅ Zero data loss during recreation or backup

---

## Technical Specifications

### User Messages & Error Specifications

#### Resume Messages

**Active Session (Attach Directly)**:
```
Attaching to active session 'claude-myapp'...
```

**Stopped Session (Auto-Recreation)**:
```
Session 'claude-myapp' stopped, recreating...
✓ Created tmux session
✓ Started Claude (UUID: e6121188-...)
✓ Session recreated successfully

Attaching to session...
```

**Archived Session (Prompt)**:
```
This session is archived. Unarchive and resume? (y/n):
```

If yes:
```
Unarchiving session...
✓ Session unarchived
Session stopped, recreating...
[... recreation messages ...]
```

If no:
```
Resume cancelled. To unarchive: csm unarchive claude-myapp
```

**Success Messages**:
```
✅ Session recreated successfully
✅ Attached to session 'claude-myapp'
```

#### Backup Messages

**Backup Progress**:
```
Creating backup for session 'claude-myapp'...
✓ Manifest: backups/2025-12-07_14-30-00-123456/session-info.yaml
✓ Found 193 messages in conversation history
✓ Conversation: backups/2025-12-07_14-30-00-123456/conversation.jsonl
✓ File snapshots: backups/2025-12-07_14-30-00-123456/file-snapshots/ (24 files)

✅ Backup complete: ~/sessions/session-claude-myapp/backups/2025-12-07_14-30-00-123456
   Latest: ~/sessions/session-claude-myapp/backups/latest
```

**Backup (Markdown format)**:
```
Creating backup for session 'claude-myapp'...
✓ Manifest: backups/2025-12-07_14-30-00-123456/session-info.yaml
✓ Found 193 messages in conversation history
✓ Conversation: backups/2025-12-07_14-30-00-123456/conversation.md (formatted)

✅ Backup complete: ~/sessions/session-claude-myapp/backups/2025-12-07_14-30-00-123456
```

**Backup Cleanup**:
```
Cleaning up old backups (keeping last 10)...
✓ Removed 2 old backups
```

#### Error Messages

**Worktree Missing**:
```
Error: worktree directory not found: /home/user/projects/myapp

The project directory has been moved or deleted.

Try one of the following:
  • Update worktree path: csm set claude-myapp --worktree <new-path>
  • Archive session: csm archive claude-myapp
  • Force resume in current dir: csm resume claude-myapp --force
```

**Claude Failed to Start**:
```
Error: Claude failed to start in tmux session

The tmux session was created but Claude did not start successfully.
This may indicate an invalid session UUID or Claude CLI issue.

Check:
  • Claude CLI is installed: which claude
  • Session UUID is valid: cat ~/sessions/session-claude-myapp/manifest.yaml
  • Try: csm doctor claude-myapp
```

**History File Not Found**:
```
Error: Claude history file not found: ~/.claude/history.jsonl

Check that Claude CLI is installed correctly.

Try: claude --version
```

**No Messages Found**:
```
Warning: no messages found for session UUID e6121188-...

This session may not have any conversation history yet.
Creating backup with empty conversation file.
```

**Disk Full During Backup**:
```
Error: backup failed: no space left on device

Partial backup has been cleaned up.

Free up disk space and try again: df -h
```

**Tmux Command Failure**:
```
Error: failed to create tmux session: tmux command failed

Check:
  • tmux is installed: which tmux
  • tmux server is running: tmux info
  • No conflicting session name: tmux ls
```

**Lock Conflict** (from S1, used in resume/backup):
```
Error: session is locked by process 12345 (started 2025-12-07T14:30:00-08:00)

Try one of the following:
  • Wait a minute and retry (process may finish)
  • Check if process is still running: ps -p 12345
  • If process is stuck, kill it: kill 12345
  • Check for stale locks: csm doctor --fix
```

**Path Validation Error**:
```
Error: invalid path: directory traversal detected

The provided path is outside the sessions directory.
```

**Command Injection Attempt**:
```
Error: invalid session name: contains prohibited characters

Session names must contain only alphanumeric characters, hyphens, and underscores.
```

### File Permissions

All files created by S2 components:
- **Backup files** (session-info.yaml, conversation.jsonl, conversation.md): 0600
- **Backup directory** (backups/2025-12-07_14-30-00-123456/): 0700
- **Symlinks** (latest): Inherit from target
- **File snapshots**: Preserve original permissions

### Path Validation

All file paths must be validated to prevent directory traversal:

```go
func validateBackupPath(sessionDir string, backupName string) error {
    // Clean the backup name
    clean := filepath.Clean(backupName)

    // Check for directory traversal
    if strings.Contains(clean, "..") {
        return fmt.Errorf("invalid backup name: contains directory traversal")
    }

    // Build full path
    fullPath := filepath.Join(sessionDir, "backups", clean)

    // Make absolute
    abs, err := filepath.Abs(fullPath)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }

    // Check it's within session directory
    sessionAbs, _ := filepath.Abs(sessionDir)
    if !strings.HasPrefix(abs, sessionAbs) {
        return fmt.Errorf("path outside session directory")
    }

    return nil
}
```

### Tmux Command Sanitization

Session names used in tmux commands must be sanitized to prevent command injection:

```go
func sanitizeSessionName(name string) (string, error) {
    // Allow only: alphanumeric, hyphen, underscore
    validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

    if !validPattern.MatchString(name) {
        return "", fmt.Errorf("invalid session name: contains prohibited characters")
    }

    return name, nil
}

func executeTmuxCommand(sessionName string, args ...string) error {
    // Sanitize session name before use
    sanitized, err := sanitizeSessionName(sessionName)
    if err != nil {
        return err
    }

    // Build command with sanitized name
    cmd := exec.Command("tmux", append([]string{"-t", sanitized}, args...)...)
    return cmd.Run()
}
```

### Backup Timestamp Format

**Always include microseconds to prevent collisions**:

```go
func generateBackupTimestamp() string {
    now := time.Now()
    // Format: YYYY-MM-DD_HH-MM-SS-microseconds
    return now.Format("2006-01-02_15-04-05") + fmt.Sprintf("-%06d", now.Nanosecond()/1000)
}

// Example: 2025-12-07_14-30-00-123456
```

### History.jsonl Parsing Strategy

**Stream processing to handle large files efficiently**:

```go
func extractConversation(historyPath string, sessionUUID string) ([]HistoryEntry, error) {
    file, err := os.Open(historyPath)
    if err != nil {
        return nil, fmt.Errorf("cannot open history file: %w", err)
    }
    defer file.Close()

    var entries []HistoryEntry
    var skippedCount int

    scanner := bufio.NewScanner(file)
    // Set max line size to 10MB (handle large messages)
    scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

    for scanner.Scan() {
        line := scanner.Bytes()

        var entry HistoryEntry
        err := json.Unmarshal(line, &entry)
        if err != nil {
            // Skip malformed lines, continue processing
            skippedCount++
            continue
        }

        // Filter by session UUID
        if entry.SessionID == sessionUUID {
            entries = append(entries, entry)
        }
    }

    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("error reading history: %w", err)
    }

    if skippedCount > 0 {
        log.Printf("Skipped %d malformed entries in history.jsonl", skippedCount)
    }

    return entries, nil
}
```

**Key properties**:
- Stream processing (don't load entire file into memory)
- Skip malformed JSON lines (don't fail entire backup)
- Report skipped count in logs
- Handle large messages (up to 10MB per line)

### Backup Atomic Creation

**Create in temp directory, move when complete**:

```go
func createBackupAtomic(sessionDir string, manifest *Manifest) error {
    timestamp := generateBackupTimestamp()

    // Create temp directory
    tempDir := filepath.Join(sessionDir, "backups", ".tmp-"+timestamp)
    finalDir := filepath.Join(sessionDir, "backups", timestamp)

    // Create temp directory with 0700
    if err := os.MkdirAll(tempDir, 0700); err != nil {
        return fmt.Errorf("cannot create temp backup dir: %w", err)
    }

    // Clean up temp dir on failure
    defer func() {
        if _, err := os.Stat(tempDir); err == nil {
            os.RemoveAll(tempDir)
        }
    }()

    // Create all backup files in temp directory
    if err := createBackupFiles(tempDir, manifest); err != nil {
        return err
    }

    // Atomic move: temp → final
    if err := os.Rename(tempDir, finalDir); err != nil {
        return fmt.Errorf("cannot finalize backup: %w", err)
    }

    return nil
}
```

---

## Deliverables

### D2.1: Status Computation (FR-8)
**Priority**: P0 (Must Have)
**Estimated Effort**: 3 hours
**Dependencies**: S1 (manifest schema with lifecycle field)

**Tasks**:
1. Create `cmd/csm/status.go`:
   - `func ComputeStatus(m *manifest.Manifest) string`
   - `func ComputeStatuses(manifests []*manifest.Manifest) map[string]string`
   - Status determination logic:
     - If `lifecycle == "archived"` → return "archived"
     - Else if tmux session exists → return "active"
     - Else → return "stopped"

2. Integrate with existing commands:
   - Update `cmd/csm/list.go` to use batch status computation
   - Update `cmd/csm/resume.go` to use single status computation
   - Remove any old status references

3. Batch optimization:
   - Single call to `tmux list-sessions` for all sessions
   - Cache results in map
   - O(1) lookup per session

4. Performance target:
   - List 50 sessions in < 1 second (NFR-1.2)
   - Single tmux query, not N queries

**Acceptance Criteria**:
- [ ] Archived session: status = "archived"
- [ ] Tmux exists + lifecycle empty: status = "active"
- [ ] Tmux missing + lifecycle empty: status = "stopped"
- [ ] Status never stored in manifest (computed on read)
- [ ] List 50 sessions: only 1 call to `tmux list-sessions`
- [ ] Statuses computed from cached tmux list
- [ ] Performance: list completes in < 1 second

**Tests**:
- `status_test.go`: Status determination logic
- `status_batch_test.go`: Batch computation, performance test

---

### D2.2: Enhanced Resume with Auto-Recreation (FR-5)
**Priority**: P0 (Must Have)
**Estimated Effort**: 10 hours (increased from 8h for security additions)
**Dependencies**: S1 (locking, manifest schema), D2.1 (status computation)

**Tasks**:
1. Update `cmd/csm/resume.go`:
   - Use status computation to detect active/stopped/archived
   - Implement auto-recreation workflow for stopped sessions
   - Add worktree validation before recreation
   - Add unarchive flow for archived sessions
   - Add partial failure rollback (kill tmux if Claude fails)
   - **NEW**: Add tmux command sanitization
   - **NEW**: Add path validation for worktree
   - **NEW**: Add --yes flag for non-interactive mode

2. Auto-recreation workflow (FR-5.2):
   ```
   1. Acquire lock
   2. ComputeStatus(manifest) → "stopped"
   3. Sanitize session name (alphanumeric + hyphen + underscore only)
   4. Validate worktree path (resolve symlinks, check exists, no traversal)
   5. Create tmux session:
      tmux new-session -d -s <sanitized-name> -c <validated-worktree>
   6. Send Claude command:
      tmux send-keys -t <sanitized-name> 'claude --resume <uuid>' C-m
   7. Attach to session:
      tmux attach-session -t <sanitized-name>
   8. Update manifest last_activity
   9. Release lock
   ```

3. Exact commands (from D4):
   - Check: `tmux has-session -t <name>` (exit code 0 = exists)
   - Create: `tmux new-session -d -s <name> -c <worktree>`
   - Send: `tmux send-keys -t <name> 'claude --resume <uuid>' C-m`
   - Attach: `tmux attach-session -t <name>`

4. Worktree validation (FR-5.3):
   - Use `filepath.EvalSymlinks()` to resolve symlinks
   - Validate path doesn't escape sessions directory
   - Check if path exists with `os.Stat()`
   - If missing: error with suggestions (see Technical Specs)

5. Partial failure rollback (FR-5.4):
   - If Claude fails to start, kill tmux session
   - Use `tmux kill-session -t <name>`
   - Error message: "Claude failed to start" (not "tmux failed")

6. Unarchive flow (FR-5.6):
   - Detect archived status
   - If --yes flag: Unarchive and proceed
   - Else: Prompt: "This session is archived. Unarchive and resume? (y/n)"
   - If yes: Set lifecycle to "", proceed with resume
   - If no: Abort with clear message

7. Lock integration:
   - Acquire lock before any manifest modifications
   - Release lock via defer (even on panic)
   - Use lock from S1 (`manifest.AcquireLock()`)

8. User messaging:
   - All messages specified in Technical Specifications section
   - Active session: "Attaching to active session..."
   - Stopped session: "Session stopped, recreating..."
   - Success: "✅ Session recreated successfully"
   - Worktree missing: Clear error with suggestions
   - Archived: Prompt for unarchive

**Acceptance Criteria**:
- [ ] Active session (tmux exists): skips recreation, attaches directly
- [ ] Stopped session (tmux missing): triggers auto-recreation
- [ ] Archived session: prompts user to unarchive (or uses --yes)
- [ ] Stopped session detected correctly
- [ ] Session name sanitized before tmux commands
- [ ] Invalid session name (special chars) rejected with clear error
- [ ] Tmux session created with correct sanitized name
- [ ] Tmux session started in validated worktree directory
- [ ] Worktree path validated (no directory traversal)
- [ ] Claude resumed with correct UUID
- [ ] User attached to tmux session
- [ ] Worktree exists: recreation proceeds
- [ ] Worktree is symlink: resolved to target, checked
- [ ] Worktree missing: error with helpful suggestions
- [ ] Claude failure: tmux session killed automatically
- [ ] Error indicates Claude failure, not tmux
- [ ] last_activity timestamp updated to now() in RFC3339 format
- [ ] Manifest written to disk
- [ ] If write fails, warning shown but resume continues
- [ ] Archived session detected
- [ ] User prompted for unarchive (unless --yes)
- [ ] --yes flag bypasses prompt for non-interactive use
- [ ] Lock acquired before manifest modification
- [ ] Lock released on completion or error
- [ ] Resume with auto-recreation completes in < 3 seconds (NFR-1.1)
- [ ] All user messages match Technical Specifications
- [ ] All error messages match Technical Specifications

**Tests**:
- `resume_test.go`: Status detection, worktree validation, sanitization
- `resume_integration_test.go`: Full auto-recreation workflow
- `resume_rollback_test.go`: Partial failure scenarios
- `resume_archive_test.go`: Unarchive flow
- `resume_security_test.go`: Path validation, command injection prevention

---

### D2.3: Backup Command (FR-6)
**Priority**: P1 (Should Have)
**Estimated Effort**: 10 hours (increased from 8h for security additions)
**Dependencies**: S1 (fileutil, manifest schema)

**Tasks**:
1. Create `cmd/csm/backup.go`:
   - `func runBackup(identifier string, format string, includeFiles bool) error`
   - Implement backup creation workflow with atomic creation
   - Support JSONL and Markdown formats
   - Optional file snapshots
   - Backup retention (keep last 10)
   - Latest symlink creation
   - **NEW**: Backup file permissions (0600/0700)
   - **NEW**: Path validation
   - **NEW**: Atomic creation (temp dir → rename)
   - **NEW**: History.jsonl stream processing

2. Backup creation workflow (FR-6.1):
   ```
   1. Acquire lock
   2. Resolve identifier → manifest path
   3. Load manifest
   4. Validate backup path (prevent directory traversal)
   5. Create temp backup directory: ~/sessions/<session>/backups/.tmp-<timestamp>/
   6. Copy manifest → session-info.yaml (0600)
   7. Extract conversation from history.jsonl → conversation.jsonl (0600)
      - Stream processing (don't load entire file)
      - Skip malformed lines
      - Report skipped count
   8. (Optional) Copy file-history/ → file-snapshots/ (preserve permissions)
   9. Atomic move: .tmp-<timestamp> → <timestamp>
   10. Set directory permissions: 0700
   11. Create/update 'latest' symlink
   12. Clean old backups (keep last 10)
   13. Release lock
   ```

3. Backup directory structure (from D4):
   ```
   ~/sessions/session-<name>/backups/2025-12-07_14-30-00-123456/
   ├── session-info.yaml       # Manifest snapshot (0600)
   ├── conversation.jsonl      # Filtered history entries (0600)
   ├── conversation.md         # (if --format markdown) (0600)
   └── file-snapshots/         # (if --include-files) (preserve original)
   ```

4. Timestamp format:
   - Format: `2025-12-07_14-30-00-123456` (YYYY-MM-DD_HH-MM-SS-µs)
   - Always include microseconds (prevents collisions)
   - Use local time zone
   - See Technical Specifications for code example

5. Conversation extraction:
   - Read `~/.claude/history.jsonl` via stream processing
   - Filter entries by session UUID
   - Skip malformed JSON lines (continue processing)
   - Report skipped count in logs
   - Write only matching entries to `conversation.jsonl`
   - See Technical Specifications for implementation

6. Markdown format (FR-6.2):
   - Header: Session name, UUID, date range
   - Each message:
     - Timestamp
     - Role (user/assistant)
     - Content (formatted)
   - Code blocks properly escaped
   - Long messages: Include in full (no truncation)
   - Example:
     ```markdown
     # Session: claude-myapp
     UUID: e6121188-...
     Created: 2025-12-01

     ## 2025-12-01 14:30:00 - User
     How do I implement feature X?

     ## 2025-12-01 14:30:15 - Assistant
     Here's how to implement feature X...
     ```

7. File snapshots (FR-6.3):
   - If `--include-files` flag provided
   - Copy entire `~/.claude/file-history/<uuid>/` directory
   - Use `fileutil.CopyDirectory()` from S1
   - Preserve permissions
   - Skip if directory doesn't exist (not an error)

8. Backup retention (FR-6.4):
   - After creating backup, list all backups for this session
   - Sort by timestamp (newest first)
   - If count > 10, delete oldest backups
   - Only delete backup directories, not 'latest' symlink
   - Best-effort cleanup: Log warning on failure, don't fail backup

9. Latest symlink (FR-6.5):
   - Create/update `backups/latest` → most recent backup
   - Use relative path for symlink target
   - On Windows without symlink support: show warning, continue
   - Example: `latest → 2025-12-07_14-30-00-123456`

10. User messaging (FR-6.6):
    - Show progress for each step
    - All messages in Technical Specifications section
    - Example output:
      ```
      Creating backup for session 'claude-myapp'...
      ✓ Manifest: backups/2025-12-07_14-30-00-123456/session-info.yaml
      ✓ Found 193 messages in conversation history
      ✓ Conversation: backups/2025-12-07_14-30-00-123456/conversation.jsonl

      ✅ Backup complete: ~/sessions/session-claude-myapp/backups/2025-12-07_14-30-00-123456
         Latest: ~/sessions/session-claude-myapp/backups/latest
      ```

11. Lock integration:
    - Acquire lock before reading manifest
    - Release lock via defer
    - Backup is read-only operation, but lock prevents concurrent modifications

12. Error handling:
    - History file doesn't exist: Error with suggestion to check Claude installation
    - No messages found for UUID: Warning, create empty conversation file
    - Disk full during backup: Error, cleanup partial backup
    - File-history directory missing: Skip with info message (not error)
    - All errors match Technical Specifications

13. File permissions:
    - Backup directory: 0700
    - session-info.yaml: 0600
    - conversation.jsonl: 0600
    - conversation.md: 0600
    - file-snapshots: Preserve original permissions

**Acceptance Criteria**:
- [ ] Backup directory created with timestamp (microseconds included)
- [ ] Backup directory permissions: 0700
- [ ] Backup created atomically (temp dir → rename)
- [ ] Partial backup cleaned up on failure
- [ ] Manifest copied to session-info.yaml (0600)
- [ ] Conversation extracted from history.jsonl via streaming
- [ ] Malformed JSON lines skipped, count reported
- [ ] Only entries for this session UUID included
- [ ] `csm backup claude-1` creates conversation.jsonl (0600)
- [ ] `csm backup claude-1 --format markdown` creates conversation.md (0600)
- [ ] Markdown format includes headers, timestamps, formatted messages
- [ ] Code blocks in messages properly escaped
- [ ] Long messages included in full (no truncation)
- [ ] `csm backup claude-1 --include-files` copies file-history/ directory
- [ ] File snapshots copied to backup/file-snapshots/
- [ ] File snapshot permissions preserved
- [ ] Without flag: file snapshots not copied
- [ ] Create 11 backups for a session → cleanup triggered
- [ ] 11th backup triggers cleanup
- [ ] Oldest backup deleted
- [ ] Only 10 most recent backups remain
- [ ] Cleanup failure logged as warning, doesn't fail backup
- [ ] After backup, `backups/latest` symlink exists (if supported)
- [ ] Symlink points to most recent backup directory (relative path)
- [ ] Subsequent backup updates symlink
- [ ] On Windows without symlink support: warning shown, backup succeeds
- [ ] Shows progress for each step
- [ ] Shows final backup location
- [ ] Shows latest symlink location (if created)
- [ ] All progress messages match Technical Specifications
- [ ] All error messages match Technical Specifications
- [ ] Lock acquired before manifest read
- [ ] Lock released on completion or error
- [ ] Path validation prevents directory traversal
- [ ] Backing up 200-message session completes in < 5 seconds (NFR-1.4)

**Tests**:
- `backup_test.go`: Backup creation, conversation extraction, streaming
- `backup_format_test.go`: JSONL and Markdown formats, escaping
- `backup_retention_test.go`: Cleanup old backups
- `backup_symlink_test.go`: Latest symlink creation
- `backup_integration_test.go`: Full backup workflow
- `backup_atomic_test.go`: Atomic creation, partial failure cleanup
- `backup_security_test.go`: Path validation, file permissions

---

## Integration with S1 Components

### Manifest Loading
```go
// All commands use the same pattern
manifest, err := manifest.Load(manifestPath)
// This triggers automatic migration if v1
```

### Lock Acquisition
```go
// Resume command
lock, err := manifest.AcquireLock(manifestPath)
if err != nil {
    return fmt.Errorf("failed to acquire lock: %w", err)
}
defer lock.Release()

// ... modify manifest ...
```

### Validation
```go
// Before writing manifest
if err := manifest.Validate(); err != nil {
    return fmt.Errorf("invalid manifest: %w", err)
}
```

### File Operations
```go
// Use fileutil from S1
fileutil.CopyFile(src, dst)
fileutil.CopyDirectory(src, dst)
fileutil.WriteAtomic(path, data, 0600)
```

---

## Out of Scope (Later Sprints)

The following are **NOT** included in S2:

- Doctor command (S3)
- Log rotation (S3)
- Configurable sessions directory (already in Phase 3)
- Archive/unarchive commands (Phase 4)
- Integration tests (S3)
- Performance benchmarks (S3)

---

## Testing Strategy

### Unit Tests (per-deliverable)

**D2.1 Status Computation**:
- `status_test.go`: All status combinations (active/stopped/archived)
- `status_batch_test.go`: Batch computation, performance test with 50 sessions

**D2.2 Enhanced Resume**:
- `resume_test.go`: Status detection, worktree validation, sanitization
- `resume_integration_test.go`: Full auto-recreation workflow
- `resume_rollback_test.go`: Partial failure scenarios
- `resume_archive_test.go`: Unarchive flow
- `resume_security_test.go`: Path validation, command injection prevention

**D2.3 Backup**:
- `backup_test.go`: Backup creation, conversation extraction, streaming
- `backup_format_test.go`: JSONL and Markdown formats, escaping
- `backup_retention_test.go`: Cleanup old backups
- `backup_symlink_test.go`: Latest symlink creation
- `backup_integration_test.go`: Full backup workflow
- `backup_atomic_test.go`: Atomic creation, partial failure cleanup
- `backup_security_test.go`: Path validation, file permissions

### Integration Tests (S2 scope)

**TS-S2-1: Resume Active Session**
- Given: Session with active tmux
- When: User runs `csm resume claude-1`
- Then: Attaches directly, no recreation

**TS-S2-2: Resume Stopped Session (Auto-Recreation)**
- Given: Session manifest exists, tmux session missing
- When: User runs `csm resume claude-1`
- Then: Tmux created, Claude resumed, user attached

**TS-S2-3: Resume with Missing Worktree**
- Given: Worktree directory deleted
- When: User runs `csm resume claude-1`
- Then: Error with suggestions, no tmux created

**TS-S2-4: Resume Archived Session**
- Given: Session with lifecycle="archived"
- When: User runs `csm resume claude-1`
- Then: Prompt to unarchive, proceed if yes

**TS-S2-5: Partial Failure Rollback**
- Given: Invalid Claude UUID
- When: Resume creates tmux but Claude fails
- Then: Tmux killed, clear error message

**TS-S2-6: Backup Creation (JSONL)**
- Given: Session with 100 messages
- When: User runs `csm backup claude-1`
- Then: Backup created with all 100 messages in JSONL

**TS-S2-7: Backup Creation (Markdown)**
- Given: Session with messages
- When: User runs `csm backup claude-1 --format markdown`
- Then: Backup created with formatted Markdown

**TS-S2-8: Backup with File Snapshots**
- Given: Session with file-history
- When: User runs `csm backup claude-1 --include-files`
- Then: File snapshots copied to backup

**TS-S2-9: Backup Retention**
- Given: Session has 10 backups
- When: User creates 11th backup
- Then: Oldest backup deleted, 10 remain

**TS-S2-10: Batch Status Computation**
- Given: 50 sessions (mix of active/stopped/archived)
- When: User runs `csm list`
- Then: Single tmux query, all statuses correct, < 1 second

**TS-S2-11: Concurrent Backup and Resume** (NEW)
- Given: Session manifest exists
- When: Terminal 1 runs `csm backup claude-1` (takes time)
  AND: Terminal 2 runs `csm resume claude-1` (modifies manifest)
- Then: Lock prevents conflict, operations serialize correctly

**TS-S2-12: Corrupted History File** (NEW)
- Given: history.jsonl contains 5 valid + 3 malformed JSON lines
- When: User runs `csm backup claude-1`
- Then: Backup created with 5 valid entries, 3 skipped, count reported in logs

**TS-S2-13: Broken Symlink in Worktree** (NEW)
- Given: Worktree is symlink to non-existent directory
- When: User runs `csm resume claude-1`
- Then: Clear error "worktree directory not found", suggestions shown, no tmux created

**TS-S2-14: Tmux Command Failure** (NEW)
- Given: Mock `tmux new-session` to fail (e.g., tmux not installed)
- When: User runs `csm resume claude-1`
- Then: Clear error "failed to create tmux session", suggestions shown, no partial state

**TS-S2-15: Backup with Large Messages** (NEW)
- Given: Session with 5 messages, each >1MB
- When: User runs `csm backup claude-1`
- Then: Completes successfully, all messages in backup, no truncation

**TS-S2-16: Disk Full During Backup** (NEW)
- Given: Start backup with sufficient space
- When: Simulate disk full mid-backup (after creating temp dir)
- Then: Error "no space left on device", partial backup cleaned up

**TS-S2-17: Session Name Command Injection** (NEW)
- Given: Session with name containing backticks or semicolons
- When: User runs `csm resume claude-$(whoami)`
- Then: Error "invalid session name: contains prohibited characters", no command executed

**TS-S2-18: Backup Path Traversal** (NEW)
- Given: Attempt to create backup with path traversal
- When: Internal call with backup name "../../../etc/passwd"
- Then: Error "invalid backup name: contains directory traversal", no file created

**TS-S2-19: Resume with --yes Flag** (NEW)
- Given: Archived session
- When: User runs `csm resume claude-1 --yes`
- Then: Unarchived and resumed without prompt

**TS-S2-20: Backup Atomic Failure** (NEW)
- Given: Start backup creation
- When: Rename operation fails (permissions changed mid-operation)
- Then: Temp directory cleaned up, clear error, no partial backup

**TS-S2-21: History.jsonl Not Found** (NEW)
- Given: `~/.claude/history.jsonl` doesn't exist
- When: User runs `csm backup claude-1`
- Then: Error "Claude history file not found", suggestions shown

### Performance Tests

**Resume Performance** (NFR-1.1):
- Measure: Time from command execution to tmux attach
- Target: < 3 seconds average (10 runs)
- No run > 5 seconds

**List Performance** (NFR-1.2):
- Create 50 test sessions
- Measure: `csm list` execution time
- Target: < 1 second average (10 runs)

**Backup Performance** (NFR-1.4):
- Create session with 200 messages
- Measure: Backup execution time
- Target: < 5 seconds

**Backup Streaming Performance** (NEW):
- Create history.jsonl with 10,000 total messages
- Session has 200 matching messages
- Measure: Memory usage during backup
- Target: < 100MB peak memory (proves streaming works)

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All P0 requirements: 100%

---

## Implementation Order

### Day 1 (Status & Resume Foundation)
1. Morning: Status Computation (3h)
2. Afternoon: Resume - Status Detection + Worktree Validation + Sanitization (4h)

### Day 2 (Resume Auto-Recreation)
3. Morning: Resume - Auto-Recreation Workflow (4h)
4. Afternoon: Resume - Rollback + Unarchive + Security (4h)

### Day 3 (Backup)
5. Morning: Backup - Core Functionality + Atomic Creation (5h)
6. Afternoon: Backup - Formats + Retention + Security (5h)

### Optional Day 4 (if needed)
7. Integration tests
8. Performance tests
9. Documentation

---

## Risk Management

### Risk 1: Tmux Auto-Recreation Fails
**Probability**: MEDIUM
**Impact**: HIGH
**Mitigation**:
- ✅ Sanitize session names (prevent command injection)
- ✅ Validate worktree exists before creation
- ✅ Test exact tmux commands in integration tests
- ✅ Rollback on partial failure (kill tmux if Claude fails)
- ✅ Clear error messages with suggestions
- ✅ Lock prevents concurrent recreation attempts

### Risk 2: Backup Extraction Slow
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Performance target: 200 messages in < 5 seconds
- ✅ Stream processing (don't load entire file)
- ✅ Filter during read (not after)
- ✅ Benchmark tests to verify performance
- ✅ Max line size: 10MB (handles large messages)

### Risk 3: History.jsonl Corrupted
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Skip malformed lines (don't fail entire backup)
- ✅ Report count of skipped entries
- ✅ Create backup even if some entries corrupted
- ✅ Test with corrupted history file (TS-S2-12)

### Risk 4: Symlink Support on Windows
**Probability**: MEDIUM
**Impact**: LOW
**Mitigation**:
- ✅ Detect symlink support before creation
- ✅ Show warning if unsupported
- ✅ Continue with backup (symlink optional)
- ✅ Test on Windows (or document known limitation)

### Risk 5: Concurrent Resume Attempts
**Probability**: LOW
**Impact**: LOW
**Mitigation**:
- ✅ Lock prevents concurrent modifications
- ✅ Clear error if locked (from S1)
- ✅ Test concurrent resume scenario (TS-S2-11)

### Risk 6: Command Injection via Session Names
**Probability**: LOW
**Impact**: HIGH
**Mitigation**:
- ✅ Sanitize all session names before tmux commands
- ✅ Allow only alphanumeric + hyphen + underscore
- ✅ Reject invalid names with clear error
- ✅ Test injection attempts (TS-S2-17)

### Risk 7: Directory Traversal in Backup Paths
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Validate all backup paths
- ✅ Reject paths with ".."
- ✅ Verify within session directory
- ✅ Test traversal attempts (TS-S2-18)

---

## Success Metrics

### Functional
- [ ] All 3 deliverables implemented
- [ ] All P0 acceptance criteria met
- [ ] All P1 acceptance criteria met
- [ ] Zero data loss scenarios
- [ ] All edge cases handled

### Quality
- [ ] >80% test coverage for critical paths
- [ ] >60% test coverage overall
- [ ] All unit tests passing
- [ ] All integration tests passing
- [ ] All performance tests passing
- [ ] Zero known bugs

### Performance
- [ ] Resume auto-recreation < 3s (average)
- [ ] List 50 sessions < 1s
- [ ] Backup 200 messages < 5s
- [ ] Backup memory usage < 100MB (streaming)

---

## Documentation Requirements

### Code Documentation
- [ ] Godoc comments on all exported functions
- [ ] Inline comments for complex logic
- [ ] tmux commands documented in code
- [ ] Backup format documented
- [ ] Sanitization logic documented
- [ ] Path validation logic documented

### User Documentation (DR-1)
- [ ] Help text for `csm resume` (enhanced)
- [ ] Help text for `csm backup` (new command)
- [ ] Examples for backup formats
- [ ] Examples for auto-recreation
- [ ] See Help Text Drafts section below

### Developer Documentation
- [ ] README updated with backup feature
- [ ] CHANGELOG entry for auto-recreation
- [ ] CHANGELOG entry for backup command

---

## Help Text Drafts

### csm resume --help

```
Usage: csm resume <session-name|session-id> [flags]

Resume a Claude session in tmux. If the session is stopped, it will be
automatically recreated.

Arguments:
  <session-name|session-id>  Name or ID of session to resume

Flags:
  --yes              Skip confirmation prompts (for scripting)
  --force            Resume in current directory if worktree missing
  -h, --help         Show this help message

Behavior:
  • Active session: Attach to existing tmux session
  • Stopped session: Recreate tmux session and start Claude
  • Archived session: Prompt to unarchive (or use --yes)

Examples:
  # Resume an active or stopped session
  csm resume claude-myapp

  # Resume archived session without prompt
  csm resume claude-myapp --yes

  # Force resume in current directory (if worktree moved)
  csm resume claude-myapp --force

Auto-Recreation:
  When a session is stopped (tmux session missing), CSM will:
  1. Validate the worktree directory exists
  2. Create a new tmux session with the original name
  3. Start Claude with the session UUID
  4. Attach you to the session

  This allows sessions to survive reboots.

Troubleshooting:
  If resume fails, try:
  • Check session status: csm list
  • Verify worktree exists: ls <worktree-path>
  • Check for issues: csm doctor <session-name>
  • View logs: csm logs <session-name>

See also: csm list, csm doctor, csm archive
```

### csm backup --help

```
Usage: csm backup <session-name|session-id> [flags]

Create a backup of a session's conversation history and metadata.

Arguments:
  <session-name|session-id>  Name or ID of session to backup

Flags:
  --format string       Backup format: jsonl or markdown (default: jsonl)
  --include-files       Include file snapshots in backup
  -h, --help            Show this help message

Backup Location:
  Backups are stored in: ~/sessions/<session-name>/backups/

  Each backup is a timestamped directory containing:
  • session-info.yaml: Snapshot of session metadata
  • conversation.jsonl or .md: Conversation history
  • file-snapshots/: File history (if --include-files used)

Backup Retention:
  CSM keeps the last 10 backups per session. Older backups are
  automatically deleted when new backups are created.

  A 'latest' symlink always points to the most recent backup.

Examples:
  # Create JSONL backup (default)
  csm backup claude-myapp

  # Create Markdown backup for easy reading
  csm backup claude-myapp --format markdown

  # Include file snapshots in backup
  csm backup claude-myapp --include-files

  # View latest backup
  cat ~/sessions/claude-myapp/backups/latest/conversation.md

Backup Formats:
  • jsonl: Machine-readable, one message per line
           Compatible with Claude's history format
           Best for automated processing

  • markdown: Human-readable, formatted text
              Easy to read and share
              Best for documentation

Notes:
  • Backups are read-only snapshots (don't modify original session)
  • Conversation extracted from ~/.claude/history.jsonl
  • Only messages for this session UUID are included
  • Malformed history entries are skipped (logged in CSM logs)

See also: csm list, csm resume
```

---

## Post-Deployment Verification

After deploying S2 to any environment, verify:

### 1. Status Computation Works

```bash
# Create test sessions: active, stopped, archived
csm create claude-active
csm create claude-stopped
tmux kill-session -t claude-stopped  # Make it stopped
csm archive claude-archived

# List all sessions
csm list

# Verify:
# - claude-active shows "active"
# - claude-stopped shows "stopped"
# - claude-archived shows "archived"
```

### 2. Auto-Recreation Works

```bash
# Create session and kill tmux
csm create claude-test
tmux kill-session -t claude-test

# Resume (should auto-recreate)
csm resume claude-test

# Verify:
# - Sees "Session stopped, recreating..." message
# - Tmux session created
# - Claude started
# - Attached to session
# - Inside tmux: ps aux | grep claude (should see Claude running)
```

### 3. Worktree Validation Works

```bash
# Create session in /tmp/test-worktree
mkdir -p /tmp/test-worktree
cd /tmp/test-worktree
csm create claude-worktree-test

# Delete worktree
rm -rf /tmp/test-worktree

# Try to resume
csm resume claude-worktree-test

# Verify:
# - Error "worktree directory not found"
# - Shows suggestions (update path, archive, force)
# - No tmux session created
```

### 4. Backup Works (JSONL)

```bash
# Create backup
csm backup claude-test

# Verify:
ls -la ~/sessions/session-claude-test/backups/
# - Timestamped directory exists (YYYY-MM-DD_HH-MM-SS-microseconds)
# - Directory permissions: 0700
# - session-info.yaml exists (0600)
# - conversation.jsonl exists (0600)
# - latest symlink points to backup

# Check content
cat ~/sessions/session-claude-test/backups/latest/conversation.jsonl
# - Valid JSON lines
# - Contains messages from session
```

### 5. Backup Works (Markdown)

```bash
# Create Markdown backup
csm backup claude-test --format markdown

# Verify:
cat ~/sessions/session-claude-test/backups/latest/conversation.md
# - Formatted Markdown
# - Headers with session info
# - Messages with timestamps and roles
# - Code blocks properly formatted
```

### 6. Backup Retention Works

```bash
# Create 11 backups rapidly
for i in {1..11}; do
    csm backup claude-test
    sleep 1  # Ensure different timestamps
done

# Verify:
ls ~/sessions/session-claude-test/backups/ | grep -v latest | wc -l
# - Should show 10 (oldest deleted)
```

### 7. Concurrent Operations Protected

```bash
# Terminal 1
csm resume claude-test  # This will hold lock

# Terminal 2 (while T1 running)
csm backup claude-test  # Should wait or fail with lock error

# Verify:
# - T2 shows lock error with PID and timestamp
# - No corruption or race conditions
```

### 8. Tmux Sanitization Works

```bash
# Try to create session with special characters
csm create 'claude-test; echo hacked'

# Verify:
# - Error "invalid session name: contains prohibited characters"
# - No command executed
# - No session created
```

### 9. Archived Session Flow Works

```bash
# Archive a session
csm archive claude-test

# Try to resume
csm resume claude-test

# Verify:
# - Prompt "This session is archived. Unarchive and resume? (y/n)"
# - If y: unarchives and resumes
# - If n: cancels with message

# Try with --yes flag
csm resume claude-test --yes

# Verify:
# - No prompt shown
# - Unarchives and resumes automatically
```

### 10. Performance Acceptable

```bash
# Create 50 test sessions
for i in {1..50}; do
    csm create claude-perf-$i
done

# Time list command
time csm list

# Verify:
# - Completes in < 1 second
# - All 50 sessions show correct status

# Time resume (auto-recreation)
tmux kill-session -t claude-perf-1
time csm resume claude-perf-1

# Verify:
# - Completes in < 3 seconds
# - Session recreated and attached
```

---

## Rollback Procedure

If S2 deployment has critical bugs and needs to be rolled back:

### 1. Immediate Rollback (Git)

```bash
# Identify S2 commit
git log --oneline | grep "S2"

# Revert to previous commit
git revert <s2-commit-hash>

# Rebuild and redeploy
go build -o csm ./cmd/csm
```

### 2. Verify Old CSM Works

```bash
# Test basic commands with old CSM
csm list
csm resume claude-test

# Verify:
# - List shows sessions
# - Resume works (no auto-recreation, manual tmux attach)
```

### 3. Clean Up S2 Artifacts

```bash
# Remove backup directories (optional, if causing issues)
find ~/sessions -type d -name "backups" -exec rm -rf {} +

# Note: This is optional. Backups are harmless and may be useful.
# Only remove if they're causing issues or taking too much space.
```

### 4. Communicate to Users

If users were using S2 features:

```bash
# Notify users
echo "S2 features (auto-recreation, backup) temporarily disabled"
echo "Revert to manual tmux session management"
echo "Check ~/.csm/logs/ for any errors during rollback"
```

### When to Rollback

**Critical issues**:
- Auto-recreation corrupts manifests
- Command injection vulnerability exploited
- Data loss in backup process
- Deadlocks in concurrent operations
- Security issues (path traversal exploited)

**When NOT to Rollback**:
- Minor UI issues (wrong message text)
- Non-critical bugs (backup missing progress message)
- Individual operation failures (one backup failed)
- Performance slightly below target (< 4s instead of < 3s)

### Partial Rollback

If only one feature has issues:

```bash
# Disable auto-recreation (keep backup working)
# Edit resume.go to skip auto-recreation logic
# Or: Add feature flag to config

# Disable backup (keep auto-recreation working)
# Remove backup command from CLI
# Or: Add feature flag to config
```

---

## Monitoring & Metrics

### Metrics to Track

**Auto-Recreation Success Rate**:
```bash
# Count successful auto-recreations
grep "Session recreated successfully" ~/.csm/logs/csm.log | wc -l

# Count failed auto-recreations
grep "Error.*failed to create tmux session" ~/.csm/logs/csm.log | wc -l

# Calculate success rate
# Success rate = successes / (successes + failures)
```

**Backup Success Rate**:
```bash
# Count successful backups
grep "Backup complete" ~/.csm/logs/csm.log | wc -l

# Count failed backups
grep "Error.*backup failed" ~/.csm/logs/csm.log | wc -l

# Calculate success rate
```

**Performance Metrics**:
```bash
# Average resume time (if logging enabled)
grep "Resume completed in" ~/.csm/logs/csm.log | awk '{sum+=$NF} END {print sum/NR}'

# Average backup time (if logging enabled)
grep "Backup completed in" ~/.csm/logs/csm.log | awk '{sum+=$NF} END {print sum/NR}'
```

**Disk Usage**:
```bash
# Total backup storage
du -sh ~/sessions/*/backups/

# Per-session backup storage
du -sh ~/sessions/*/backups/ | sort -h

# Alert if > 10GB total
TOTAL=$(du -sb ~/sessions/*/backups/ 2>/dev/null | awk '{sum+=$1} END {print sum}')
if [ $TOTAL -gt 10737418240 ]; then
    echo "WARNING: Backups using > 10GB"
fi
```

**Error Rate**:
```bash
# Count errors by type
grep "Error:" ~/.csm/logs/csm.log | cut -d: -f2 | sort | uniq -c | sort -rn

# Most common errors (top 5)
grep "Error:" ~/.csm/logs/csm.log | cut -d: -f2 | sort | uniq -c | sort -rn | head -5
```

### Monitoring Dashboards (Optional)

If using monitoring tools (Prometheus, Grafana, etc.):

**Metrics to export**:
- `csm_resume_total{status="success|failure"}`
- `csm_resume_duration_seconds`
- `csm_backup_total{status="success|failure"}`
- `csm_backup_duration_seconds`
- `csm_backup_size_bytes`
- `csm_sessions_total{status="active|stopped|archived"}`

**Alerts to configure**:
- Auto-recreation success rate < 95% (investigate)
- Backup success rate < 95% (investigate)
- Resume duration > 5s (performance issue)
- Backup storage > 10GB (cleanup needed)
- Error rate > 1% (investigate)

### Log Rotation

**Note**: Full log rotation implemented in S3 (OR-3).

For S2, manual log rotation if needed:

```bash
# Rotate CSM logs if > 100MB
LOG_SIZE=$(stat -f%z ~/.csm/logs/csm.log 2>/dev/null || stat -c%s ~/.csm/logs/csm.log 2>/dev/null)
if [ $LOG_SIZE -gt 104857600 ]; then
    mv ~/.csm/logs/csm.log ~/.csm/logs/csm.log.1
    touch ~/.csm/logs/csm.log
fi
```

---

## Definition of Done

S2 is **DONE** when:

1. ✅ All 3 deliverables implemented and tested
2. ✅ All P0 and P1 acceptance criteria checked off
3. ✅ Test coverage >80% for critical paths
4. ✅ All tests passing (unit + integration + performance)
5. ✅ Code documented (godoc + inline)
6. ✅ All user messages match Technical Specifications
7. ✅ All error messages match Technical Specifications
8. ✅ Help text implemented for resume and backup
9. ✅ Multi-persona review score ≥8.5/10
10. ✅ No known critical or high-severity bugs
11. ✅ Integration with S1 verified
12. ✅ Performance targets met
13. ✅ Security hardening complete (sanitization, validation, permissions)
14. ✅ Post-deployment verification checklist completed
15. ✅ Rollback procedure tested
16. ✅ All code committed and pushed

---

## Files to Create/Modify

### New Files
```
cmd/csm/
  ├── status.go                     # NEW - Status computation
  ├── status_test.go                # NEW - Tests
  ├── status_batch_test.go          # NEW - Batch tests
  ├── resume_integration_test.go    # NEW - Tests
  ├── resume_rollback_test.go       # NEW - Tests
  ├── resume_archive_test.go        # NEW - Tests
  ├── resume_security_test.go       # NEW - Tests (sanitization, validation)
  ├── backup.go                     # NEW - Backup command
  ├── backup_test.go                # NEW - Tests
  ├── backup_format_test.go         # NEW - Tests
  ├── backup_retention_test.go      # NEW - Tests
  ├── backup_symlink_test.go        # NEW - Tests
  ├── backup_integration_test.go    # NEW - Tests
  ├── backup_atomic_test.go         # NEW - Tests (atomic creation)
  └── backup_security_test.go       # NEW - Tests (permissions, validation)
```

### Modified Files
```
cmd/csm/
  ├── resume.go              # MODIFY - Add auto-recreation, sanitization, validation
  ├── resume_test.go         # MODIFY - Add new tests
  └── list.go                # MODIFY - Use batch status computation
```

---

## Changes from v1

1. ✅ **User Messages & Error Specifications**: All exact text specified (like S1)
2. ✅ **Backup file permissions**: All files 0600, directories 0700
3. ✅ **Tmux command sanitization**: Alphanumeric + hyphen + underscore only
4. ✅ **Path validation**: Prevent directory traversal in backup and worktree paths
5. ✅ **Concurrent operation tests**: TS-S2-11 through TS-S2-21 added
6. ✅ **Post-deployment verification**: 10-step checklist with commands
7. ✅ **Rollback procedure**: Complete guide for emergency rollback
8. ✅ **Help text drafts**: csm resume --help and csm backup --help
9. ✅ **Monitoring guidance**: Metrics, dashboards, alerts
10. ✅ **Backup timestamp microseconds**: Always included to prevent collisions
11. ✅ **History.jsonl parsing strategy**: Stream processing, skip malformed lines
12. ✅ **Backup atomic creation**: Temp dir → rename pattern
13. ✅ **--yes flag**: Non-interactive mode for scripting
14. ✅ **Security tests added**: Permissions, sanitization, path validation
15. ✅ **Markdown code block handling**: Proper escaping specified
16. ✅ **Large message handling**: No truncation, up to 10MB per message
17. ✅ **Cleanup error handling**: Best-effort, log warnings, don't fail

---

## Next Sprints Preview (Not in S2 Scope)

### S3: Health & Operations
- Doctor command (FR-7)
- Log rotation (OR-3)
- Integration tests across all features
- Performance benchmarks
- Estimated: 2-3 days

---

## Review Checklist

Before submitting for multi-persona review:

- [x] All deliverables clearly defined
- [x] All acceptance criteria listed
- [x] All risks identified and mitigated
- [x] Test strategy comprehensive
- [x] Implementation order logical
- [x] Dependencies on S1 identified
- [x] Success metrics defined
- [x] Documentation requirements clear
- [x] Files to create/modify listed
- [x] Definition of Done complete
- [x] Technical specifications added
- [x] All error messages specified
- [x] All user messages specified
- [x] Post-deployment verification added
- [x] Rollback procedure added
- [x] Security considerations addressed
- [x] Help text drafts added
- [x] Monitoring guidance added
- [x] All Round 1 feedback addressed

---

**Status**: Ready for Multi-Persona Review Round 2
**Version**: 2.0
**Last Updated**: December 7, 2025
