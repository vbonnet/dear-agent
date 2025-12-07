# S3: Sprint 3 - Health, Operations & Testing (v2)

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Round 2
**Version**: 2.0
**Sprint Goal**: Implement operational readiness features and comprehensive testing
**Prerequisites**:
- S1 Foundation ✅ Complete (schema v2, migration, locking, validation, fileutil)
- S2 User Features ✅ Approved (status, resume, backup)
- D4 Requirements ✅ Approved (9.3/10)
- S3 Round 1 ❌ Revision needed (8.0/10)

---

## Executive Summary

Sprint 3 completes Phase 3.5 by implementing operational readiness features and comprehensive testing infrastructure. This includes the doctor command for health checks, log rotation for long-term operation, and extensive integration and performance testing.

**Scope**: 3 deliverables (completes 11 total in Phase 3.5)
**Duration Estimate**: 2-3 days of focused development
**Dependencies**: S1 (all infrastructure), S2 (all user features)

**Changes from v1**:
- Doctor history.jsonl parsing strategy specified (streaming from S2)
- Doctor UUID check optimized (cache history parse, single read)
- Doctor fix mode lock age check hardened (< 60s protected)
- Test time budget clarified (fast vs slow tests)
- Rotation temp file permissions specified (0600)
- Additional integration tests added (TS-INT-11 through TS-INT-15)
- Doctor fix action logging added
- Doctor scheduling guidance added
- Doctor --dry-run flag added
- Test data preparation section added
- Rotation partial failure handling documented
- Doctor summary format for many sessions added

**Strategic Rationale**: S3 makes the system production-ready. The doctor command enables troubleshooting and automated health checks. Log rotation ensures sustainable long-term operation. Comprehensive testing validates that all components work together correctly and meet performance targets. This sprint transforms CSM from a working prototype to a production-ready tool.

---

## Sprint Goal

**Primary Goal**: Make CSM production-ready through health checks, operational sustainability, and comprehensive testing.

**Success Criteria**:
1. ✅ Doctor command detects and fixes common issues automatically
2. ✅ Log files managed sustainably (rotation prevents unbounded growth)
3. ✅ All components tested together (integration tests across S1+S2+S3)
4. ✅ Performance targets validated (benchmarks for all operations)
5. ✅ Zero known critical bugs before production deployment

---

## Technical Specifications

### Doctor Command - Implementation Details

#### History.jsonl Parsing Strategy

**Reuse S2 backup streaming approach** (efficient, handles large files):

```go
// Parse history.jsonl once, cache all UUIDs
func parseHistoryUUIDs(historyPath string) (map[string]bool, error) {
    file, err := os.Open(historyPath)
    if err != nil {
        return nil, fmt.Errorf("cannot open history file: %w", err)
    }
    defer file.Close()

    uuids := make(map[string]bool)
    var skippedCount int

    scanner := bufio.NewScanner(file)
    scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)  // 10MB line limit

    for scanner.Scan() {
        line := scanner.Bytes()

        var entry struct {
            SessionID string `json:"session_id"`
        }

        err := json.Unmarshal(line, &entry)
        if err != nil {
            // Skip malformed lines, continue processing
            skippedCount++
            continue
        }

        if entry.SessionID != "" {
            uuids[entry.SessionID] = true
        }
    }

    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("error reading history: %w", err)
    }

    if skippedCount > 0 {
        log.Printf("Skipped %d malformed entries in history.jsonl", skippedCount)
    }

    return uuids, nil
}

// Check all sessions against cached UUID set
func checkUUIDsInHistory(manifests []*Manifest, historyPath string) []CheckResult {
    // Single parse of history.jsonl
    uuids, err := parseHistoryUUIDs(historyPath)
    if err != nil {
        return []CheckResult{{
            Name: "UUID Check",
            Status: "fail",
            Message: fmt.Sprintf("Cannot read history: %v", err),
        }}
    }

    results := []CheckResult{}
    for _, m := range manifests {
        if uuids[m.SessionID] {
            results = append(results, CheckResult{
                Name: fmt.Sprintf("UUID: %s", m.Name),
                Status: "pass",
                Message: fmt.Sprintf("Found in history (%s)", m.SessionID[:8]+"...)"),
            })
        } else {
            results = append(results, CheckResult{
                Name: fmt.Sprintf("UUID: %s", m.Name),
                Status: "warn",
                Message: "Not found in history (may be new session)",
            })
        }
    }

    return results
}
```

**Key properties**:
- Read history.jsonl once (not N times for N sessions)
- Stream processing (memory bounded)
- Skip malformed lines (resilient)
- Cache UUIDs in map for O(1) lookup

#### Doctor Fix Mode Lock Age Check

**Precise lock age checking to avoid removing active locks**:

```go
func checkStaleLocks(sessionDir string, fix bool) []CheckResult {
    lockPath := filepath.Join(sessionDir, "manifest.yaml.lock")

    info, err := os.Stat(lockPath)
    if os.IsNotExist(err) {
        return []CheckResult{{
            Name: "Stale Lock",
            Status: "pass",
            Message: "No lock file",
        }}
    }

    // Read lock file to get PID and timestamp
    data, err := os.ReadFile(lockPath)
    if err != nil {
        return []CheckResult{{
            Name: "Stale Lock",
            Status: "warn",
            Message: fmt.Sprintf("Cannot read lock: %v", err),
        }}
    }

    lines := strings.Split(strings.TrimSpace(string(data)), "\n")
    if len(lines) < 2 {
        return []CheckResult{{
            Name: "Stale Lock",
            Status: "warn",
            Message: "Invalid lock format",
            Fixable: fix,
        }}
    }

    // Parse timestamp from lock file
    lockTime, err := time.Parse(time.RFC3339, lines[1])
    if err != nil {
        return []CheckResult{{
            Name: "Stale Lock",
            Status: "warn",
            Message: "Invalid lock timestamp",
            Fixable: fix,
        }}
    }

    age := time.Since(lockTime)

    // CRITICAL: Only consider locks > 60s old as stale
    if age < 60*time.Second {
        return []CheckResult{{
            Name: "Stale Lock",
            Status: "pass",
            Message: fmt.Sprintf("Active lock (age: %ds)", int(age.Seconds())),
        }}
    }

    // Stale lock detected
    result := CheckResult{
        Name: "Stale Lock",
        Status: "warn",
        Message: fmt.Sprintf("Lock age: %ds (threshold: 60s)", int(age.Seconds())),
        Fixable: true,
    }

    if fix {
        err := os.Remove(lockPath)
        if err != nil {
            result.Message += fmt.Sprintf(" - Fix failed: %v", err)
        } else {
            result.Message = fmt.Sprintf("Removed stale lock (age: %ds)", int(age.Seconds()))

            // Log fix action to migration.log
            logDoctorFix("REMOVED_STALE_LOCK", lockPath, age)
        }
    }

    return []CheckResult{result}
}

// Log doctor fix actions to migration.log
func logDoctorFix(action string, path string, age time.Duration) {
    logPath := filepath.Join(os.Getenv("HOME"), ".csm", "logs", "migration.log")

    f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if err != nil {
        log.Printf("Failed to log doctor fix: %v", err)
        return
    }
    defer f.Close()

    timestamp := time.Now().Format(time.RFC3339)
    fmt.Fprintf(f, "[%s] DOCTOR-FIX: %s: %s (age: %ds)\n",
        timestamp, action, path, int(age.Seconds()))
}
```

#### Check Execution Order

**Logical ordering for efficient and clear execution**:

1. **Sessions directory check** - Fast, prerequisite for all others
2. **Manifest validation** - Load all manifests (cache for later checks)
3. **Stale lock detection** - Check lock files
4. **UUID check** - Parse history.jsonl once, check all UUIDs
5. **Worktree check** - Resolve symlinks, check existence
6. **Migration backup check** - (If --check-migrations) Check .v1.bak files

**Rationale**:
- Directory check first (fail fast if no sessions)
- Manifest loading caches manifests for later checks
- UUID check reads history.jsonl once for all sessions
- Worktree check last (slowest, many filesystem operations)

#### Doctor Output Modes

**Summary format for many sessions**:

```
# Default verbose (all checks shown)
$ csm doctor
Running health checks...
✓ Sessions directory exists
✓ Manifest valid: session-claude-1
✓ Manifest valid: session-claude-2
... (50 more) ...
Summary: 0 warnings, 0 errors

# Summary mode (only counts, not individual checks)
$ csm doctor --summary
Running health checks...
✓ Sessions directory exists
✓ 50 manifests validated (all valid)
✓ No stale locks detected
✓ 48 UUIDs found in history (2 new sessions)
✓ 50 worktrees checked (all exist)

Summary: 0 warnings, 0 errors
```

### Doctor --dry-run Flag

**Preview fix actions without applying**:

```
$ csm doctor --dry-run --fix
Running health checks (dry-run mode)...

✓ Sessions directory exists
✓ All manifests valid
⚠ Stale lock detected: session-claude-3 (lock age: 125s)
  → Would remove: ~/sessions/session-claude-3/manifest.yaml.lock
⚠ Stale lock detected: session-claude-7 (lock age: 200s)
  → Would remove: ~/sessions/session-claude-7/manifest.yaml.lock

Summary: 2 warnings (would fix), 0 errors

Run without --dry-run to apply fixes.
```

### Log Rotation - Technical Details

#### Temp File Permissions

**All temp files created with 0600**:

```go
func RotateLog(logPath string, maxSize int64, keepCount int) error {
    // Check if rotation needed
    info, err := os.Stat(logPath)
    if err != nil || info.Size() < maxSize {
        return nil
    }

    // Create temp file with 0600
    tempPath := logPath + ".tmp"
    tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
    if err != nil {
        if os.IsExist(err) {
            // Stale .tmp from previous crash, remove and retry
            os.Remove(tempPath)
            tempFile, err = os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
            if err != nil {
                return fmt.Errorf("cannot create temp file: %w", err)
            }
        } else {
            return fmt.Errorf("cannot create temp file: %w", err)
        }
    }
    tempFile.Close()

    // Rotate existing files: .log.4 → .log.5, .log.3 → .log.4, etc.
    for i := keepCount - 1; i >= 1; i-- {
        oldPath := fmt.Sprintf("%s.%d", logPath, i)
        newPath := fmt.Sprintf("%s.%d", logPath, i+1)

        if i == keepCount-1 {
            // Delete oldest file (.log.5 → .log.6, then delete .log.6)
            os.Remove(newPath)
        }

        // Rename (permissions preserved automatically)
        os.Rename(oldPath, newPath)
    }

    // Move current log to .log.1 (atomic)
    err = os.Rename(logPath, logPath+".1")
    if err != nil {
        // Cleanup temp file on failure
        os.Remove(tempPath)
        return fmt.Errorf("cannot rotate log: %w", err)
    }

    // Create new empty log file
    newLog, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0600)
    if err != nil {
        os.Remove(tempPath)
        return fmt.Errorf("cannot create new log: %w", err)
    }
    newLog.Close()

    // Remove temp file (succeeded)
    os.Remove(tempPath)

    return nil
}
```

**Key properties**:
- Temp file created with 0600
- Permissions preserved during rename
- Stale .tmp cleanup on retry
- Atomic operations (rename)

#### Partial Failure Handling

**Best-effort rotation with fallback**:

```go
func logMigration(status string, path string, err error) {
    logPath := filepath.Join(os.Getenv("HOME"), ".csm", "logs", "migration.log")

    // Attempt rotation before writing
    if needsRotation(logPath) {
        rotateErr := rotate.RotateLog(logPath, 10*1024*1024, 5)
        if rotateErr != nil {
            // Log rotation failure to stderr, but continue
            fmt.Fprintf(os.Stderr, "Warning: log rotation failed: %v\n", rotateErr)
            // Try to write to current log anyway
        }
    }

    // Append log entry (best-effort)
    f, openErr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if openErr != nil {
        // Current log can't be opened, try fallback
        fallbackPath := "/tmp/csm-migration-fallback.log"
        f, openErr = os.OpenFile(fallbackPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
        if openErr != nil {
            // Even fallback failed, log to stderr only
            fmt.Fprintf(os.Stderr, "Error: cannot write migration log: %v\n", openErr)
            return
        }
        defer f.Close()
        fmt.Fprintf(os.Stderr, "Warning: using fallback log: %s\n", fallbackPath)
    } else {
        defer f.Close()
    }

    // Write log entry
    timestamp := time.Now().Format(time.RFC3339)
    if err != nil {
        fmt.Fprintf(f, "[%s] %s: %s - %v\n", timestamp, status, path, err)
    } else {
        fmt.Fprintf(f, "[%s] %s: %s\n", timestamp, status, path)
    }
}
```

**Fallback strategy**:
1. Rotation failure → Log to stderr, continue with current log
2. Current log write failure → Try /tmp/csm-migration-fallback.log
3. Fallback failure → Log to stderr only
4. Never fail operation due to logging failure

---

## Deliverables

### D3.1: Doctor Command (FR-7)
**Priority**: P1 (Should Have)
**Estimated Effort**: 10 hours (increased from 8h for optimizations)
**Dependencies**: S1 (manifest, locking), S2 (status computation)

**Tasks**:
1. Create `cmd/csm/doctor.go`:
   - `func runDoctor(identifier string, fix bool, quiet bool, dryRun bool, summary bool, checkMigrations bool) error`
   - Implement health check system
   - Support all/specific session checks
   - Auto-fix capability for stale locks
   - **NEW**: --dry-run flag for preview
   - **NEW**: --summary flag for many sessions
   - **NEW**: Fix action logging

2. Health check functions:
   ```go
   type CheckResult struct {
       Name    string
       Status  string  // "pass", "warn", "fail"
       Message string
       Fixable bool
   }

   func checkSessionsDirectory() CheckResult
   func checkManifestValid(path string) CheckResult
   func checkStaleLocks(sessionDir string, fix bool, dryRun bool) []CheckResult
   func checkUUIDsInHistory(manifests []*Manifest, historyPath string) []CheckResult
   func checkWorktreeExists(path string) CheckResult
   func checkMigrationBackups(sessionDir string) CheckResult
   ```

3. Health checks to implement (FR-7.1):
   - **Sessions directory exists**: Check `~/sessions/` (or configured path) exists
   - **All manifests valid**: Load and validate each manifest.yaml (cache for later)
   - **Stale lock detection**: Find lock files > 60s old (protect < 60s)
   - **Claude UUIDs in history**: Parse history.jsonl once, check all UUIDs (optimized)
   - **Worktrees exist**: Check worktree directories exist (resolve symlinks)
   - **Migration backups**: (With --check-migrations) Verify .v1.bak files present

4. Stale lock cleanup (FR-7.2):
   - `csm doctor` detects stale locks as warnings
   - `csm doctor --fix` removes stale locks (> 60s old)
   - **NEW**: `csm doctor --fix --dry-run` shows what would be fixed
   - Confirmation message for each fixed lock
   - Respects active locks (< 60s old)
   - **NEW**: Logs fix actions to migration.log

5. Output modes (FR-7.3):
   - **Verbose mode** (default): Show all checks with ✓ or ✗
   - **Summary mode** (`--summary`): Show aggregated results (for many sessions)
   - **Quiet mode** (`--quiet`): Show only warnings and errors
   - **Dry-run mode** (`--dry-run`): Preview fixes without applying
   - **Exit codes**: 0 = healthy, 1 = warnings, 2 = errors
   - Scriptable output for automation

6. Specific session check (FR-7.4):
   - `csm doctor <identifier>` checks only that session
   - Validates manifest
   - Checks worktree
   - Checks UUID in history
   - Focused output (not all sessions)

7. Implementation details:
   - **History.jsonl parsing**: Stream processing, single read, cached UUIDs
   - **UUID check optimization**: O(1) lookup after single parse
   - **Lock age check**: Precise timestamp check, protect < 60s locks
   - **Check execution order**: Directory → Manifests → Locks → UUIDs → Worktrees
   - **Fix action logging**: Log to migration.log with timestamp and details

8. Output format:
   ```
   Running health checks...

   ✓ Sessions directory exists: ~/sessions
   ✓ Manifest valid: session-claude-1
   ✓ Manifest valid: session-claude-2
   ⚠ Stale lock detected: session-claude-3 (lock age: 125s)
   ✓ Worktree exists: session-claude-1 → /home/user/projects/app
   ✗ Worktree missing: session-claude-2 → /home/user/old-project

   Summary: 2 warnings, 1 error

   Suggestions:
     • Fix stale locks: csm doctor --fix
     • Update worktree: csm set claude-2 --worktree <new-path>
     • Or archive: csm archive claude-2
   ```

9. Fix mode output:
   ```
   Running health checks with auto-fix...

   ✓ Sessions directory exists
   ✓ All manifests valid
   ⚠ Stale lock detected: session-claude-3
     → Removed stale lock: ~/sessions/session-claude-3/manifest.yaml.lock
     → Lock age: 125s (threshold: 60s)
     → Logged fix to migration.log
   ✓ UUIDs checked
   ✓ Worktrees checked

   Summary: 1 warning fixed, 0 errors

   ✅ System healthy
   ```

10. User messaging:
    - All messages specified (see Technical Specifications section above)
    - Clear pass/warn/fail indicators
    - Actionable suggestions for errors
    - Summary at end
    - **NEW**: Dry-run preview messages
    - **NEW**: Summary mode aggregated messages

**Acceptance Criteria**:
- [ ] `csm doctor` runs all health checks
- [ ] Sessions directory check: pass if exists, fail if missing
- [ ] Manifest validation: load and validate each manifest.yaml
- [ ] Manifests cached after loading (used for UUID check)
- [ ] Invalid manifest: show error with path and reason
- [ ] Stale lock detection: warn if lock > 60s old
- [ ] Active lock (< 60s): marked as pass, not stale
- [ ] Lock age check uses timestamp from lock file (not file mtime)
- [ ] Shows lock age in warning message
- [ ] History.jsonl parsed once via streaming (S2 approach)
- [ ] All UUIDs cached in map for O(1) lookup
- [ ] Malformed JSON lines skipped, count reported
- [ ] Claude UUID check: checks all sessions against cached UUIDs
- [ ] Missing UUID: warn (not error, session may be new)
- [ ] Worktree check: resolve symlinks, verify exists
- [ ] Missing worktree: show error with suggestions
- [ ] Migration backup check: (--check-migrations) verify .v1.bak files
- [ ] Missing backup: warn (not error, may be v2 native)
- [ ] Each check shows ✓ (pass), ⚠ (warn), or ✗ (fail)
- [ ] Failed checks show error details
- [ ] Summary shows count: "2 warnings, 1 error"
- [ ] Checks execute in order: directory → manifests → locks → UUIDs → worktrees
- [ ] `csm doctor --fix` removes stale locks (> 60s)
- [ ] Fix mode does NOT remove active locks (< 60s)
- [ ] Confirmation message for each removed lock
- [ ] Fix actions logged to migration.log with timestamp
- [ ] Log format: `[timestamp] DOCTOR-FIX: <action>: <path> (age: <n>s)`
- [ ] `csm doctor --fix --dry-run` previews fixes without applying
- [ ] Dry-run shows "Would remove: <path>" for each stale lock
- [ ] Dry-run exit message: "Run without --dry-run to apply fixes"
- [ ] `csm doctor` verbose mode shows all checks
- [ ] `csm doctor --summary` shows aggregated results
- [ ] Summary format: "50 manifests validated (all valid)"
- [ ] `csm doctor --quiet` shows only warnings/errors
- [ ] Exit code 0: all healthy
- [ ] Exit code 1: warnings present
- [ ] Exit code 2: errors present
- [ ] `csm doctor claude-1` checks only that session
- [ ] Specific check shows focused output
- [ ] All user messages match Technical Specifications
- [ ] Doctor completes in < 2 seconds for 10 sessions
- [ ] Doctor with 50 sessions uses single history.jsonl read

**Tests**:
- `doctor_test.go`: Individual check functions
- `doctor_integration_test.go`: Full health check workflow
- `doctor_fix_test.go`: Stale lock cleanup, fix action logging
- `doctor_quiet_test.go`: Output modes and exit codes
- `doctor_specific_test.go`: Single session check
- `doctor_optimization_test.go`: UUID check performance (single history read)
- `doctor_dryrun_test.go`: Dry-run mode validation

---

### D3.2: Log Rotation (OR-3)
**Priority**: P1 (Should Have)
**Estimated Effort**: 5 hours (increased from 4h for hardening)
**Dependencies**: S1 (migration logging)

**Tasks**:
1. Create `internal/logging/rotate.go`:
   - `func RotateLog(logPath string, maxSize int64, keepCount int) error`
   - Implement rotation logic
   - Non-blocking rotation
   - Atomic file operations
   - **NEW**: Temp file with 0600 permissions
   - **NEW**: Stale .tmp cleanup
   - **NEW**: Fallback strategy for failures

2. Rotation policy (OR-3):
   - **Trigger**: Log file exceeds 10MB
   - **Retention**: Keep last 5 rotated files (.log.1 through .log.5)
   - **Naming**: migration.log → migration.log.1 (newest) to migration.log.5 (oldest)
   - **Deletion**: migration.log.6 deleted when .log.5 becomes .log.6

3. Rotation workflow:
   ```
   1. Check log file size
   2. If < 10MB: Continue writing
   3. If ≥ 10MB:
      a. Create .log.tmp with 0600 (or cleanup stale .tmp and retry)
      b. Rotate existing files: .log.4 → .log.5, .log.3 → .log.4, etc.
      c. Delete .log.5 if it would become .log.6
      d. Move current log: migration.log → migration.log.1 (atomic)
      e. Create new migration.log (empty) with 0600
      f. Remove .log.tmp (succeeded)
      g. Continue writing to new file
   ```

4. Rotation triggers:
   - **Before write**: Check size before appending log entry
   - **Lazy rotation**: Only rotate when actually writing
   - **No background process**: Rotation on-demand only

5. Atomic operations:
   - Use temp files for rotation: `.log.tmp` (marker file)
   - Atomic renames (POSIX guarantees)
   - Cleanup temp files on failure
   - **NEW**: Stale .tmp cleanup (previous crash recovery)

6. Integration with migration logger:
   ```go
   // Update internal/manifest/migrate.go
   func logMigration(status string, path string, err error) {
       logPath := filepath.Join(os.Getenv("HOME"), ".csm", "logs", "migration.log")

       // Check size before writing
       if needsRotation(logPath) {
           rotateErr := rotate.RotateLog(logPath, 10*1024*1024, 5)
           if rotateErr != nil {
               fmt.Fprintf(os.Stderr, "Warning: log rotation failed: %v\n", rotateErr)
               // Continue with current log
           }
       }

       // Append log entry (with fallback)
       f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
       if err != nil {
           // Fallback to /tmp
           f, err = os.OpenFile("/tmp/csm-migration-fallback.log",
               os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
           if err != nil {
               fmt.Fprintf(os.Stderr, "Error: cannot write migration log: %v\n", err)
               return
           }
       }
       defer f.Close()

       timestamp := time.Now().Format(time.RFC3339)
       fmt.Fprintf(f, "[%s] %s: %s\n", timestamp, status, path)
   }
   ```

7. Error handling:
   - Rotation failure: Log to stderr, continue with current file
   - Disk full: Try fallback log in /tmp
   - Permissions: Log error, don't crash
   - Best-effort: Never fail operation due to log rotation failure
   - **NEW**: Fallback to /tmp/csm-migration-fallback.log

8. Performance:
   - Check file size: O(1) via os.Stat()
   - Rotation: O(n) where n = keepCount (max 5)
   - Non-blocking: No locks held during rotation

9. File permissions:
   - **NEW**: .log.tmp created with 0600
   - Rotated files preserve original permissions (0600)
   - New migration.log created with 0600

**Acceptance Criteria**:
- [ ] Log file < 10MB: no rotation
- [ ] Log file ≥ 10MB: rotation triggered
- [ ] .log.tmp created with 0600 before rotation
- [ ] Stale .tmp (from previous crash) cleaned up and retried
- [ ] Current log moved to .log.1 (atomic rename)
- [ ] Existing .log.1 → .log.2, .log.2 → .log.3, etc.
- [ ] .log.5 deleted before rotation (if exists)
- [ ] .log.6+ cleaned up if exist (manual creation)
- [ ] New migration.log created (empty) with 0600
- [ ] .log.tmp removed after successful rotation
- [ ] Only 5 rotated files retained after rotation
- [ ] Rotation uses atomic operations (rename)
- [ ] Permissions preserved during rotation (0600)
- [ ] Rotation failure logged to stderr
- [ ] Migration continues even if rotation fails
- [ ] Disk full during rotation: fallback to /tmp/csm-migration-fallback.log
- [ ] Fallback log also created with 0600
- [ ] Fallback failure: log to stderr only, don't crash
- [ ] File permissions preserved (0600)
- [ ] Active writes not interrupted during rotation
- [ ] Rotation completes in < 100ms

**Tests**:
- `rotate_test.go`: Rotation logic, file operations, permissions
- `rotate_integration_test.go`: Integration with migration logger
- `rotate_edge_test.go`: Disk full, permissions, concurrent writes, stale .tmp
- `rotate_fallback_test.go`: Fallback to /tmp on failure

---

### D3.3: Integration & Performance Testing
**Priority**: P0 (Must Have)
**Estimated Effort**: 10 hours (increased from 8h for additional tests)
**Dependencies**: S1, S2, D3.1, D3.2

**Tasks**:
1. Create integration test suite:
   - `cmd/csm/integration_test.go`: End-to-end workflows
   - Tests across all sprints (S1 + S2 + S3)
   - Real file system operations
   - Real tmux interactions (with mocking for CI)
   - **NEW**: Test data preparation utilities
   - **NEW**: Test isolation (isolated directories)

2. Integration test scenarios (15 total, 5 new):
   - **TS-INT-1**: Full lifecycle - create, resume, backup, doctor, archive
   - **TS-INT-2**: Reboot simulation - kill tmux, resume all, verify recreation
   - **TS-INT-3**: Migration + resume - v1 manifest → migrate → resume
   - **TS-INT-4**: Concurrent operations - resume + backup + doctor simultaneously
   - **TS-INT-5**: Doctor fixes + resume - doctor --fix stale locks, then resume
   - **TS-INT-6**: Backup + rotation - create large history, backup, rotate logs
   - **TS-INT-7**: Error recovery - disk full during backup, verify cleanup
   - **TS-INT-8**: Security validation - injection attempts, path traversal
   - **TS-INT-9**: Performance under load - 50 sessions, all operations
   - **TS-INT-10**: Long-running stability - repeated operations, no memory leaks
   - **TS-INT-11**: Doctor fix + concurrent resume (NEW)
   - **TS-INT-12**: Rotation disk full (NEW)
   - **TS-INT-13**: Doctor empty system (NEW)
   - **TS-INT-14**: Doctor new session (UUID not in history) (NEW)
   - **TS-INT-15**: Rotation .log.tmp exists (stale from crash) (NEW)

3. Performance benchmark suite:
   - `cmd/csm/benchmark_test.go`: Go benchmarks
   - Measure all critical operations
   - Validate against NFR targets
   - **NEW**: BM-9 for rotation

4. Performance benchmarks (9 total, 1 new):
   - **BM-1**: Resume auto-recreation (target: < 3s)
   - **BM-2**: List 50 sessions (target: < 1s)
   - **BM-3**: Backup 200 messages (target: < 5s)
   - **BM-4**: Doctor 50 sessions (target: < 2s)
   - **BM-5**: Migration v1 → v2 (target: < 100ms)
   - **BM-6**: Lock acquire/release (target: < 10ms)
   - **BM-7**: Manifest validation (target: < 1ms)
   - **BM-8**: Status computation (target: < 50ms for 50 sessions)
   - **BM-9**: Log rotation (target: < 100ms) (NEW)

5. Load testing:
   - Create 100 test sessions
   - Run all operations concurrently
   - Measure resource usage (CPU, memory, disk I/O)
   - Verify no resource leaks

6. Stress testing:
   - Large backups (1000+ messages)
   - Deeply nested worktrees
   - Long session names
   - Unicode in all fields
   - Concurrent operations (10+ processes)

7. Regression testing:
   - All test scenarios from S1, S2, S3
   - Verify no feature degradation
   - Cross-sprint integration

8. Test infrastructure:
   - **NEW**: Test harness for tmux operations (mock in CI)
   - **NEW**: Mock history.jsonl generation
   - **NEW**: Test data fixtures (v1 manifests, sessions)
   - **NEW**: Cleanup between tests (isolated directories)
   - **NEW**: Memory leak detection (pprof integration)

9. Test isolation:
   - **NEW**: Each test uses isolated temp directory (/tmp/csm-test-XXXXX)
   - Cleanup after each test
   - No cross-test contamination
   - Parallel-safe (can run with -parallel)

10. CI/CD readiness:
    - All tests runnable via `go test ./...`
    - No manual setup required
    - Isolated test environments
    - Deterministic results
    - **NEW**: Fast tests (< 2 min) for CI/PR checks
    - **NEW**: Slow tests (nightly) for long-running stability

**Acceptance Criteria**:
- [ ] TS-INT-1 through TS-INT-15 all passing
- [ ] BM-1 through BM-9 all meet targets
- [ ] All S1 tests passing
- [ ] All S2 tests passing
- [ ] All S3 tests passing
- [ ] Tests isolated (each uses /tmp/csm-test-XXX)
- [ ] Cleanup after each test (no leftover files)
- [ ] Tests parallel-safe (can run with -parallel=4)
- [ ] No flaky tests (100% pass rate on 10 runs)
- [ ] Test coverage >80% for critical paths
- [ ] Test coverage >60% overall
- [ ] Fast tests complete in < 2 minutes total (for CI)
- [ ] Slow tests have no time limit (nightly only)
- [ ] No resource leaks detected (pprof)
- [ ] No memory growth over 100 iterations (TS-INT-10)
- [ ] CI/CD ready (no manual steps)
- [ ] Test data fixtures in testdata/ directory
- [ ] Mock tmux for CI (real tmux optional)

**Tests**:
- `integration_test.go`: End-to-end scenarios (TS-INT-1 through TS-INT-15)
- `benchmark_test.go`: Performance benchmarks (BM-1 through BM-9)
- `load_test.go`: Resource usage under load
- `stress_test.go`: Edge cases and limits
- `test_helpers.go`: Test utilities, fixtures, mocking

---

## Test Data Preparation (NEW Section)

### Test Fixtures

**Location**: `cmd/csm/testdata/`

**Structure**:
```
cmd/csm/testdata/
├── manifests/
│   ├── v1-simple.yaml          # Basic v1 manifest
│   ├── v1-complete.yaml        # All fields populated
│   ├── v2-simple.yaml          # Basic v2 manifest
│   ├── v2-complete.yaml        # All fields populated
│   └── v2-archived.yaml        # Archived session
├── history/
│   ├── simple.jsonl            # 10 messages
│   ├── medium.jsonl            # 200 messages
│   └── large.jsonl             # 1000+ messages
└── worktrees/
    └── sample-project/         # Sample worktree directory
        └── README.md
```

### Mock Tmux Strategy

**For CI/CD (no tmux available)**:

```go
// Test helper to mock tmux
type TmuxMock struct {
    sessions map[string]bool
}

func (m *TmuxMock) HasSession(name string) bool {
    return m.sessions[name]
}

func (m *TmuxMock) CreateSession(name, dir string) error {
    m.sessions[name] = true
    return nil
}

func (m *TmuxMock) KillSession(name string) error {
    delete(m.sessions, name)
    return nil
}

// Use in tests
func TestResumeWithMock(t *testing.T) {
    mock := &TmuxMock{sessions: make(map[string]bool)}
    // Inject mock into resume logic
    // ...
}
```

**For local testing (tmux available)**:

```go
func TestResumeWithRealTmux(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping tmux integration test in short mode")
    }

    // Use real tmux commands
    cmd := exec.Command("tmux", "new-session", "-d", "-s", "test-session")
    err := cmd.Run()
    // ...
}
```

### Benchmark Session Creation

**Setup helper for performance tests**:

```go
func setupBenchmarkSessions(b *testing.B, count int) {
    b.Helper()

    // Create test directory
    testDir, _ := os.MkdirTemp("/tmp", "csm-bench-")
    b.Cleanup(func() { os.RemoveAll(testDir) })

    // Create sessions (without real tmux for speed)
    for i := 0; i < count; i++ {
        sessionDir := filepath.Join(testDir, fmt.Sprintf("session-%d", i))
        os.MkdirAll(sessionDir, 0700)

        // Create manifest
        manifest := &Manifest{
            SchemaVersion: "2.0",
            SessionID: uuid.New().String(),
            Name: fmt.Sprintf("bench-%d", i),
            CreatedAt: time.Now(),
            // ... other fields ...
        }

        data, _ := yaml.Marshal(manifest)
        os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), data, 0600)
    }

    return testDir
}
```

---

## Fast vs Slow Tests (NEW Section)

### Fast Tests (< 2 minutes total, for CI/PR)

**Included**:
- All unit tests (S1, S2, S3)
- Quick integration tests:
  - TS-INT-1: Full lifecycle (single session)
  - TS-INT-3: Migration + resume
  - TS-INT-5: Doctor fixes + resume
  - TS-INT-11: Doctor fix + concurrent resume
  - TS-INT-13: Doctor empty system
  - TS-INT-14: Doctor new session
- Quick benchmarks (1 iteration each for validation)

**Run with**:
```bash
go test ./... -short -timeout=2m
```

**Tagged in tests**:
```go
func TestQuickIntegration(t *testing.T) {
    // Fast test, always runs
}

func TestLongRunningStability(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping long-running test in short mode")
    }
    // Slow test, runs only without -short
}
```

### Slow Tests (No time limit, for nightly builds)

**Included**:
- Load testing (100 sessions)
- Stress testing (concurrent operations)
- Long-running stability (TS-INT-10: 100 iterations)
- Full benchmarks (10+ iterations for statistical significance)
- Integration tests with real tmux (TS-INT-2, TS-INT-4, TS-INT-9)

**Run with**:
```bash
go test ./... -timeout=30m
```

**CI/CD configuration**:
- **PR checks**: `go test -short -timeout=2m` (fast only)
- **Nightly builds**: `go test -timeout=30m` (all tests)
- **Release validation**: `go test -timeout=1h -count=10` (stability)

---

## Integration with S1 & S2 Components

### Using S1 Components

**Manifest Loading**:
```go
// Doctor uses manifest loading from S1
manifest, err := manifest.Load(manifestPath)
// Triggers automatic migration if v1
```

**Lock Checking**:
```go
// Doctor checks for stale locks
lockPath := manifestPath + ".lock"
if isLockStale(lockPath, 60*time.Second) {
    // Warn or fix
}
```

**Validation**:
```go
// Doctor validates manifests
if err := manifest.Validate(); err != nil {
    return CheckResult{Status: "fail", Message: err.Error()}
}
```

### Using S2 Components

**Status Computation**:
```go
// Doctor uses status computation from S2
status := ComputeStatus(manifest)
// Verifies status is "active", "stopped", or "archived"
```

**History.jsonl Parsing**:
```go
// Doctor verifies UUIDs in history (reuses S2 streaming approach)
uuids, err := parseHistoryUUIDs(historyPath)
// Single parse, cached for all sessions
```

---

## Out of Scope (Phase 4)

The following are **NOT** included in S3 (deferred to Phase 4):

- Archive/unarchive commands (Phase 4)
- Context tracking enhancements (Phase 4)
- Lifecycle management UI (Phase 4)
- Advanced search/filtering (Phase 4)
- Export/import features (Phase 4)
- Multi-session operations (Phase 4)
- Doctor --watch mode (continuous monitoring)

---

## Testing Strategy

### Unit Tests (per-deliverable)

**D3.1 Doctor Command**:
- `doctor_test.go`: Individual check functions
- `doctor_integration_test.go`: Full health check workflow
- `doctor_fix_test.go`: Stale lock cleanup, fix action logging
- `doctor_quiet_test.go`: Output modes and exit codes
- `doctor_specific_test.go`: Single session check
- `doctor_optimization_test.go`: UUID check performance (single history read) (NEW)
- `doctor_dryrun_test.go`: Dry-run mode validation (NEW)

**D3.2 Log Rotation**:
- `rotate_test.go`: Rotation logic, file operations, permissions
- `rotate_integration_test.go`: Integration with migration logger
- `rotate_edge_test.go`: Disk full, permissions, concurrent writes, stale .tmp
- `rotate_fallback_test.go`: Fallback to /tmp on failure (NEW)

**D3.3 Integration Testing**:
- `integration_test.go`: End-to-end scenarios (TS-INT-1 through TS-INT-15)
- `benchmark_test.go`: Performance benchmarks (BM-1 through BM-9)
- `load_test.go`: Resource usage under load
- `stress_test.go`: Edge cases and limits
- `test_helpers.go`: Test utilities, fixtures, mocking (NEW)

### Integration Test Scenarios

**TS-INT-1: Full Lifecycle**
- Create → Resume → Kill → Auto-recreate → Backup → Doctor → Archive
- Verify each step successful

**TS-INT-2: Reboot Simulation**
- Create 10 sessions
- Kill all tmux sessions
- Resume all
- Verify all recreated correctly

**TS-INT-3: Migration + Resume**
- Start with v1 manifest
- Resume triggers migration
- Verify .v1.bak created
- Verify session recreated

**TS-INT-4: Concurrent Operations**
- Run resume, backup, doctor simultaneously on same session
- Verify lock prevents conflicts
- Verify all operations succeed

**TS-INT-5: Doctor Fixes + Resume**
- Create stale lock (> 60s)
- Run `csm doctor --fix`
- Verify lock removed
- Run `csm resume`
- Verify resume successful

**TS-INT-6: Backup + Rotation**
- Create large history (> 10MB of logs)
- Run backup
- Verify log rotation triggered
- Verify backup successful

**TS-INT-7: Error Recovery**
- Simulate disk full during backup
- Verify cleanup of partial backup
- Verify system still functional

**TS-INT-8: Security Validation**
- Attempt command injection via session name
- Attempt path traversal in backup
- Verify all blocked with errors

**TS-INT-9: Performance Under Load**
- Create 50 sessions
- Run all operations (list, resume, backup, doctor)
- Verify all meet performance targets

**TS-INT-10: Long-Running Stability**
- Run repeated operations (100 iterations)
- Create, resume, backup, archive cycle
- Verify no memory leaks
- Verify no file descriptor leaks

**TS-INT-11: Doctor Fix + Concurrent Resume** (NEW)
- Doctor --fix starts (finds stale lock)
- Resume starts concurrently (tries to acquire lock)
- Verify: Doctor checks lock age (if < 60s, doesn't remove)
- Verify: Resume succeeds or waits for doctor to finish

**TS-INT-12: Rotation Disk Full** (NEW)
- Start rotation (.log.tmp created)
- Simulate disk full during rename
- Verify: .log.tmp cleaned up
- Verify: Fallback to /tmp/csm-migration-fallback.log
- Verify: Migration continues

**TS-INT-13: Doctor Empty System** (NEW)
- Fresh install, no sessions directory
- Run `csm doctor`
- Verify: Creates sessions directory or warns
- Verify: Exit code 0 (healthy, no errors)

**TS-INT-14: Doctor New Session (UUID Not in History)** (NEW)
- Create session manually (not via csm create)
- Session UUID not in history.jsonl
- Run `csm doctor`
- Verify: Warns (not error) "UUID not found (may be new)"
- Verify: Exit code 1 (warning, not error)

**TS-INT-15: Rotation .log.tmp Already Exists** (NEW)
- Create stale .log.tmp (simulate previous crash)
- Trigger rotation
- Verify: Stale .tmp detected and removed
- Verify: New .tmp created
- Verify: Rotation proceeds successfully

### Performance Tests

**BM-1: Resume Auto-Recreation** (NFR-1.1):
- Target: < 3 seconds average
- No run > 5 seconds

**BM-2: List 50 Sessions** (NFR-1.2):
- Target: < 1 second average

**BM-3: Backup 200 Messages** (NFR-1.4):
- Target: < 5 seconds

**BM-4: Doctor 50 Sessions** (NEW):
- Target: < 2 seconds
- Verify: Single history.jsonl read

**BM-5: Migration v1 → v2**:
- Target: < 100ms

**BM-6: Lock Acquire/Release**:
- Target: < 10ms

**BM-7: Manifest Validation**:
- Target: < 1ms

**BM-8: Status Computation Batch**:
- Target: < 50ms for 50 sessions

**BM-9: Log Rotation** (NEW):
- Target: < 100ms

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All P0 requirements: 100%

---

## Implementation Order

### Day 1 (Doctor Command)
1. Morning: Doctor infrastructure + health checks + UUID optimization (5h)
2. Afternoon: Fix mode + output modes + logging (5h)

### Day 2 (Log Rotation + Integration Tests Setup)
3. Morning: Log rotation implementation + fallback (5h)
4. Afternoon: Integration test infrastructure + fixtures + mocks (5h)

### Day 3 (Testing & Benchmarks)
5. Morning: Integration tests (TS-INT-1 through TS-INT-15) (5h)
6. Afternoon: Performance benchmarks (BM-1 through BM-9) + validation (5h)

---

## Risk Management

### Risk 1: Doctor Removes Active Locks
**Probability**: LOW
**Impact**: HIGH
**Mitigation**:
- ✅ Respect lock timeout (60s, not shorter)
- ✅ Check lock timestamp (not file mtime)
- ✅ Test concurrent doctor + resume (TS-INT-11)
- ✅ Never remove locks < 60s old

### Risk 2: Log Rotation Interrupts Active Writes
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Rotation only on write (not background process)
- ✅ Atomic operations (temp file + rename)
- ✅ Best-effort (don't fail on rotation error)
- ✅ Fallback to /tmp on failure
- ✅ Test concurrent writes during rotation

### Risk 3: Integration Tests Flaky
**Probability**: MEDIUM
**Impact**: MEDIUM
**Mitigation**:
- ✅ Isolated test environments (/tmp/csm-test-XXX)
- ✅ Cleanup between tests
- ✅ Deterministic test data (fixtures)
- ✅ Mock tmux for CI (real tmux optional)
- ✅ Run 10 times to verify stability

### Risk 4: Performance Tests Don't Meet Targets
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Profile before finalizing
- ✅ Optimize hot paths (UUID check cached)
- ✅ Batch operations where possible (status, UUID check)
- ✅ Document if targets need adjustment

### Risk 5: Doctor False Positives
**Probability**: MEDIUM
**Impact**: LOW
**Mitigation**:
- ✅ UUID not found = warning (not error, may be new session)
- ✅ Migration backup missing = warning (may be v2 native)
- ✅ Test false positive scenarios (TS-INT-14)
- ✅ Clear messaging on what's expected vs actual

### Risk 6: Test Time Budget Exceeded
**Probability**: MEDIUM
**Impact**: MEDIUM
**Mitigation**:
- ✅ Separate fast (< 2 min) and slow (nightly) tests
- ✅ Fast tests use mocks, minimal fixtures
- ✅ Slow tests use real tmux, large fixtures
- ✅ CI runs fast tests only (PR checks)
- ✅ Nightly runs all tests (no time limit)

---

## Success Metrics

### Functional
- [ ] All 3 deliverables implemented
- [ ] All P0 and P1 acceptance criteria met
- [ ] Doctor detects all common issues
- [ ] Log rotation prevents unbounded growth
- [ ] All integration tests passing
- [ ] All performance benchmarks meeting targets

### Quality
- [ ] >80% test coverage for critical paths
- [ ] >60% test coverage overall
- [ ] All unit tests passing
- [ ] All integration tests passing
- [ ] All performance tests passing
- [ ] Zero known critical bugs
- [ ] No flaky tests (100% pass rate on 10 runs)

### Performance
- [ ] Resume < 3s
- [ ] List 50 sessions < 1s
- [ ] Backup 200 messages < 5s
- [ ] Doctor 50 sessions < 2s
- [ ] All benchmarks meet targets

---

## Documentation Requirements

### Code Documentation
- [ ] Godoc comments on all exported functions
- [ ] Inline comments for complex logic
- [ ] Doctor check descriptions
- [ ] Rotation algorithm documented
- [ ] UUID optimization documented
- [ ] Fallback strategy documented

### User Documentation (DR-1)
- [ ] Help text for `csm doctor`
- [ ] Examples for doctor modes
- [ ] Troubleshooting guide
- [ ] Log rotation behavior documented
- [ ] Doctor scheduling recommendations

### Developer Documentation
- [ ] README updated with doctor command
- [ ] CHANGELOG entry for S3 features
- [ ] Testing guide for contributors
- [ ] Performance benchmarking guide
- [ ] Mock strategy documentation

---

## Help Text Drafts

### csm doctor --help

```
Usage: csm doctor [session-name|session-id] [flags]

Run health checks on CSM sessions. Detects common issues like stale locks,
invalid manifests, missing worktrees, and more.

Arguments:
  [session-name|session-id]  Optional: Check specific session only

Flags:
  --fix                 Automatically fix issues (e.g., remove stale locks)
  --dry-run             Preview fixes without applying (use with --fix)
  --quiet               Show only warnings and errors (for scripting)
  --summary             Show aggregated results (for many sessions)
  --check-migrations    Include migration backup validation
  -h, --help            Show this help message

Health Checks:
  • Sessions directory exists
  • All manifests are valid
  • Stale lock detection (> 60s old)
  • Claude UUIDs exist in history
  • Worktrees exist
  • Migration backups present (with --check-migrations)

Exit Codes:
  0  All checks passed (healthy)
  1  Warnings present (e.g., stale locks, missing UUIDs)
  2  Errors present (e.g., invalid manifests, missing worktrees)

Examples:
  # Check all sessions (verbose)
  csm doctor

  # Check with aggregated results (for many sessions)
  csm doctor --summary

  # Check specific session
  csm doctor claude-myapp

  # Auto-fix stale locks
  csm doctor --fix

  # Preview fixes without applying
  csm doctor --fix --dry-run

  # Quiet mode for scripting
  csm doctor --quiet
  if [ $? -ne 0 ]; then
      echo "Issues detected"
  fi

  # Include migration backup checks
  csm doctor --check-migrations

Common Issues Detected:
  • Stale locks: Locks from crashed processes (> 60s old)
  • Invalid manifests: YAML syntax errors, validation failures
  • Missing worktrees: Project directories moved or deleted
  • Missing UUIDs: Session not found in Claude history (may be new)

Troubleshooting:
  If doctor reports issues:
  • Stale locks: Run with --fix to remove automatically
  • Invalid manifest: Edit manually or restore from .v1.bak backup
  • Missing worktree: Update path with 'csm set <id> --worktree <path>'
  • Missing UUID: Normal for new sessions, ignore warning

Scheduling:
  Recommended: Run doctor daily via cron for automated monitoring:
    0 2 * * * csm doctor --quiet && echo "CSM healthy" || echo "CSM issues detected"

See also: csm list, csm resume, csm backup
```

---

## Post-Deployment Verification

After deploying S3 to any environment, verify:

### 1. Doctor Works on Healthy System

```bash
# All sessions healthy
csm doctor

# Verify:
# - Shows "✓" for all checks
# - Summary: "0 warnings, 0 errors"
# - Exit code 0
echo $?
```

### 2. Doctor Detects Stale Locks

```bash
# Create artificial stale lock
echo "99999" > ~/sessions/session-claude-1/manifest.yaml.lock
echo "2025-12-07T00:00:00-08:00" >> ~/sessions/session-claude-1/manifest.yaml.lock

# Make it old (> 60s)
touch -t 202512070000 ~/sessions/session-claude-1/manifest.yaml.lock

# Run doctor
csm doctor

# Verify:
# - Shows "⚠ Stale lock detected: session-claude-1 (lock age: ...)"
# - Exit code 1
```

### 3. Doctor Fix Mode Works

```bash
# Fix the stale lock
csm doctor --fix

# Verify:
# - Shows "→ Removed stale lock"
# - Shows "→ Logged fix to migration.log"
# - Lock file deleted
# - Exit code 0

# Verify lock removed
ls ~/sessions/session-claude-1/manifest.yaml.lock
# Should show "No such file"

# Verify fix logged
tail -1 ~/.csm/logs/migration.log
# Should show: [timestamp] DOCTOR-FIX: REMOVED_STALE_LOCK: ...
```

### 4. Doctor Dry-Run Works

```bash
# Create stale lock again
echo "99999" > ~/sessions/session-claude-1/manifest.yaml.lock
echo "2025-12-07T00:00:00-08:00" >> ~/sessions/session-claude-1/manifest.yaml.lock
touch -t 202512070000 ~/sessions/session-claude-1/manifest.yaml.lock

# Dry-run mode
csm doctor --fix --dry-run

# Verify:
# - Shows "Would remove: ~/sessions/session-claude-1/manifest.yaml.lock"
# - Lock file NOT removed (still exists)
# - Exit message: "Run without --dry-run to apply fixes"

# Verify lock still exists
ls ~/sessions/session-claude-1/manifest.yaml.lock
# Should still exist
```

### 5. Doctor Quiet Mode Works

```bash
# Create issue (missing worktree)
rm -rf /tmp/test-worktree
# (Assume session uses this worktree)

# Quiet mode
csm doctor --quiet

# Verify:
# - Shows only "✗ Worktree missing: ..."
# - No "✓" lines shown
# - Exit code 2
```

### 6. Doctor Summary Mode Works

```bash
# Create 50 sessions
for i in {1..50}; do
    csm create test-$i
done

# Summary mode
csm doctor --summary

# Verify:
# - Shows "✓ 50 manifests validated (all valid)"
# - NOT 50 individual "✓ Manifest valid: ..." lines
# - Aggregated results
```

### 7. Doctor Specific Session Works

```bash
csm doctor claude-1

# Verify:
# - Shows "Checking session 'claude-1'..."
# - Only checks for that session
# - Focused output
```

### 8. Doctor UUID Check Optimized

```bash
# Create 50 sessions
# (Already created from test 6)

# Run doctor with verbose logging
time csm doctor 2>&1 | grep -c "Parsing history.jsonl"

# Verify:
# - Only 1 occurrence (single parse)
# - Completes in < 2s for 50 sessions
```

### 9. Log Rotation Works

```bash
# Check current log size
ls -lh ~/.csm/logs/migration.log

# If < 10MB, create many migrations to trigger rotation
# (Or manually append to exceed 10MB)

# After exceeding 10MB, next migration should rotate
# Verify:
ls -lh ~/.csm/logs/
# - migration.log (small, newly created)
# - migration.log.1 (large, old file)
# - All files have 0600 permissions

# Verify permissions
stat -c "%a" ~/.csm/logs/migration.log*
# All should show "600"
```

### 10. Rotation Fallback Works

```bash
# Make ~/.csm/logs read-only
chmod 000 ~/.csm/logs/

# Trigger migration (should fallback)
# ... (trigger migration somehow) ...

# Verify:
# - Warning on stderr: "using fallback log: /tmp/csm-migration-fallback.log"
# - Migration succeeds anyway
# - Fallback log created with 0600

# Check fallback log
ls -lh /tmp/csm-migration-fallback.log
stat -c "%a" /tmp/csm-migration-fallback.log
# Should show "600"

# Restore permissions
chmod 700 ~/.csm/logs/
```

### 11. Integration Tests Pass

```bash
# Run all tests
cd ~/src/repos/ai-tools/base/claude-session-manager
go test ./cmd/csm/... -v

# Verify:
# - All tests pass
# - TS-INT-1 through TS-INT-15 complete
```

### 12. Fast Tests Complete Quickly

```bash
# Run fast tests only
go test ./cmd/csm/... -short -timeout=2m

# Verify:
# - Completes in < 2 minutes
# - All fast tests pass
# - Slow tests skipped (shows "Skipping long-running test")
```

### 13. Performance Meets Targets

```bash
# Run benchmarks
go test ./cmd/csm/... -bench=. -benchtime=10s

# Verify:
# - BenchmarkResumeAutoRecreation: < 3s per op
# - BenchmarkList50Sessions: < 1s per op
# - BenchmarkBackup200Messages: < 5s per op
# - BenchmarkDoctor50Sessions: < 2s per op
# - BenchmarkLogRotation: < 100ms per op
```

---

## Rollback Procedure

If S3 deployment has critical bugs and needs to be rolled back:

### 1. Immediate Rollback (Git)

```bash
# Identify S3 commit
git log --oneline | grep "S3"

# Revert to previous commit
git revert <s3-commit-hash>

# Rebuild and redeploy
go build -o csm ./cmd/csm
```

### 2. Verify S1+S2 Still Works

```bash
# Test S1+S2 features with rolled-back code
csm list
csm resume claude-test
csm backup claude-test

# Verify:
# - All S1 features work (locking, migration, validation)
# - All S2 features work (status, resume, backup)
```

### 3. Clean Up S3 Artifacts (Optional)

```bash
# Remove rotated log files (if causing issues)
rm ~/.csm/logs/migration.log.[1-5]

# Note: Usually safe to leave, only remove if problematic

# Remove fallback log (if exists)
rm /tmp/csm-migration-fallback.log
```

### 4. Impact on Logs

**Rollback S3 → rotation stops**:
- Existing rotated logs (.log.1 through .log.5) remain
- No new rotation will occur
- migration.log will grow unbounded again
- **Action**: Manual log rotation may be needed

### When to Rollback

**Critical issues**:
- Doctor removes active locks (data corruption risk)
- Log rotation corrupts logs (data loss)
- Integration tests reveal cross-sprint bugs (S1/S2 regression)
- Performance regression in S1/S2 features (doctor slows down system)
- Fix actions not logged (audit trail loss)

**When NOT to Rollback**:
- Doctor false positives (can be fixed in patch)
- Minor UI issues in doctor output
- Single test flakiness
- Performance slightly below target (2.5s instead of 2s for doctor)

---

## Monitoring & Metrics

### Metrics to Track

**Doctor Usage**:
```bash
# How often doctor is run
grep "Running health checks" ~/.csm/logs/csm.log | wc -l

# Issues detected
grep "Summary:" ~/.csm/logs/csm.log | grep -v "0 warnings, 0 errors"

# Auto-fixes applied
grep "DOCTOR-FIX" ~/.csm/logs/migration.log | wc -l
```

**Log Rotation**:
```bash
# Current log sizes
du -sh ~/.csm/logs/migration.log*

# Rotation count (number of .log.N files)
ls ~/.csm/logs/migration.log.* 2>/dev/null | wc -l

# Total log storage
du -sh ~/.csm/logs/
```

**System Health**:
```bash
# Run doctor in quiet mode, check exit code
csm doctor --quiet
HEALTH=$?

# Alert if unhealthy
if [ $HEALTH -ne 0 ]; then
    echo "ALERT: CSM health check failed (exit code: $HEALTH)"
fi
```

### Doctor Scheduling Recommendations

**Daily automated check (cron)**:

```bash
# Add to crontab
crontab -e

# Run doctor nightly at 2 AM, alert on issues
0 2 * * * csm doctor --quiet --summary && echo "CSM healthy" || echo "ALERT: CSM issues detected" | mail -s "CSM Health Check Failed" admin@example.com
```

**Continuous monitoring (systemd timer)**:

```bash
# /etc/systemd/system/csm-doctor.timer
[Unit]
Description=Run CSM doctor health checks daily

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target

# /etc/systemd/system/csm-doctor.service
[Unit]
Description=CSM doctor health check

[Service]
Type=oneshot
ExecStart=/usr/local/bin/csm doctor --quiet --summary
User=youruser
```

**Alert integration examples**:

**Email alerts**:
```bash
csm doctor --quiet || echo "CSM health check failed" | mail -s "CSM Alert" admin@example.com
```

**Slack webhook**:
```bash
csm doctor --quiet || curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK/URL \
  -d '{"text":"CSM health check failed: Issues detected"}'
```

**PagerDuty**:
```bash
csm doctor --quiet || curl -X POST https://events.pagerduty.com/v2/enqueue \
  -H 'Content-Type: application/json' \
  -d '{
    "routing_key": "YOUR_KEY",
    "event_action": "trigger",
    "payload": {
      "summary": "CSM health check failed",
      "severity": "warning",
      "source": "csm-doctor"
    }
  }'
```

### Log Analysis Guidance

**Analyzing rotated logs**:

```bash
# View all migration logs combined
cat ~/.csm/logs/migration.log.{5..1} ~/.csm/logs/migration.log | less

# Search across all rotated logs
grep "FAILED" ~/.csm/logs/migration.log*

# Count successes vs failures
grep "SUCCESS" ~/.csm/logs/migration.log* | wc -l
grep "FAILED" ~/.csm/logs/migration.log* | wc -l

# View doctor fix actions
grep "DOCTOR-FIX" ~/.csm/logs/migration.log*
```

**Log rotation disk usage monitoring**:

```bash
# Alert if logs exceed 100MB
TOTAL=$(du -sb ~/.csm/logs/ | awk '{print $1}')
if [ $TOTAL -gt 104857600 ]; then
    echo "WARNING: Log storage exceeds 100MB"
fi
```

### Alert Thresholds

- Doctor detects errors (exit code 2): Alert immediately
- Stale locks > 5: Investigate (may indicate crashes)
- Log storage > 100MB: Cleanup needed (rotation may be failing)
- Performance degradation > 20%: Investigate
- Fix actions > 10/day: Investigate root cause

---

## Definition of Done

S3 is **DONE** when:

1. ✅ All 3 deliverables implemented and tested
2. ✅ All P0 and P1 acceptance criteria met
3. ✅ Doctor command fully functional with all modes
4. ✅ Doctor UUID check optimized (single history read)
5. ✅ Doctor fix mode protects active locks (< 60s)
6. ✅ Doctor fix actions logged to migration.log
7. ✅ Log rotation implemented with fallback
8. ✅ Rotation temp files created with 0600
9. ✅ All integration tests passing (TS-INT-1 through TS-INT-15)
10. ✅ All performance benchmarks meeting targets (BM-1 through BM-9)
11. ✅ Test coverage >80% critical, >60% overall
12. ✅ Code documented (godoc + inline)
13. ✅ Help text implemented for doctor
14. ✅ Test data fixtures created
15. ✅ Fast vs slow tests separated
16. ✅ Mock strategy documented
17. ✅ Multi-persona review ≥8.5/10
18. ✅ No critical bugs
19. ✅ Post-deployment verification passed
20. ✅ Rollback procedure tested
21. ✅ All code committed

---

## Files to Create/Modify

### New Files
```
cmd/csm/
  ├── doctor.go                      # NEW - Doctor command
  ├── doctor_test.go                 # NEW - Tests
  ├── doctor_integration_test.go     # NEW - Tests
  ├── doctor_fix_test.go             # NEW - Tests
  ├── doctor_quiet_test.go           # NEW - Tests
  ├── doctor_specific_test.go        # NEW - Tests
  ├── doctor_optimization_test.go    # NEW - UUID check perf (NEW v2)
  ├── doctor_dryrun_test.go          # NEW - Dry-run mode (NEW v2)
  ├── integration_test.go            # NEW - End-to-end tests
  ├── benchmark_test.go              # NEW - Performance benchmarks
  ├── load_test.go                   # NEW - Load testing
  ├── stress_test.go                 # NEW - Stress testing
  ├── test_helpers.go                # NEW - Test utilities (NEW v2)
  └── testdata/                      # NEW - Test fixtures (NEW v2)
      ├── manifests/
      ├── history/
      └── worktrees/

internal/logging/
  ├── rotate.go                      # NEW - Log rotation
  ├── rotate_test.go                 # NEW - Tests
  ├── rotate_integration_test.go     # NEW - Tests
  ├── rotate_edge_test.go            # NEW - Tests
  └── rotate_fallback_test.go        # NEW - Fallback tests (NEW v2)
```

### Modified Files
```
internal/manifest/
  └── migrate.go                     # MODIFY - Add rotation + fallback

cmd/csm/
  └── main.go                        # MODIFY - Add doctor command
```

---

## Changes from v1

1. ✅ **Doctor history.jsonl parsing strategy**: Stream processing from S2, single read
2. ✅ **Doctor UUID check optimization**: Cache all UUIDs in map, O(1) lookup
3. ✅ **Doctor fix mode lock age check**: Check timestamp, protect < 60s locks
4. ✅ **Test time budget clarification**: Fast (< 2 min) vs slow (nightly) tests
5. ✅ **Rotation temp file permissions**: .log.tmp created with 0600
6. ✅ **Additional integration tests**: TS-INT-11 through TS-INT-15
7. ✅ **Doctor fix action logging**: Log to migration.log with timestamp
8. ✅ **Doctor scheduling guidance**: Cron examples, alert integration
9. ✅ **Doctor --dry-run flag**: Preview fixes without applying
10. ✅ **Test data preparation section**: Fixtures, mocks, helpers
11. ✅ **Rotation partial failure handling**: Fallback to /tmp, best-effort
12. ✅ **Doctor summary format**: --summary flag for many sessions
13. ✅ **Check execution order**: Directory → Manifests → Locks → UUIDs → Worktrees
14. ✅ **Doctor new session handling**: UUID not found = warning (not error)
15. ✅ **Rotation stale .tmp cleanup**: Recovery from previous crash
16. ✅ **Test isolation**: Each test uses /tmp/csm-test-XXX
17. ✅ **Mock tmux strategy**: CI-friendly mocking, real tmux optional
18. ✅ **Rotation benchmark**: BM-9 added (< 100ms target)
19. ✅ **Log analysis guidance**: Commands for analyzing rotated logs
20. ✅ **Doctor empty system test**: TS-INT-13 for fresh install

---

## Phase 3.5 Completion

With S3 complete, **all 11 deliverables of Phase 3.5** will be done:

**S1 Foundation (5)**:
1. ✅ Manifest schema v2
2. ✅ Migration v1 → v2
3. ✅ Context validation
4. ✅ File locking
5. ✅ Fileutil package

**S2 User Features (3)**:
6. ✅ Status computation
7. ✅ Enhanced resume (auto-recreation)
8. ✅ Backup command

**S3 Operations (3)**:
9. ✅ Doctor command
10. ✅ Log rotation
11. ✅ Integration & performance testing

**Phase 3.5: COMPLETE** → Session persistence production-ready

---

## Review Checklist

Before submitting for multi-persona review:

- [x] All deliverables clearly defined
- [x] All acceptance criteria listed
- [x] All risks identified and mitigated
- [x] Test strategy comprehensive
- [x] Implementation order logical
- [x] Dependencies on S1/S2 identified
- [x] Success metrics defined
- [x] Documentation requirements clear
- [x] Files to create/modify listed
- [x] Definition of Done complete
- [x] Technical specifications added
- [x] Help text drafts added
- [x] Post-deployment verification added
- [x] Rollback procedure added
- [x] Monitoring guidance added
- [x] All Round 1 feedback addressed
- [x] Test data preparation added
- [x] Fast vs slow tests clarified
- [x] Mock strategy documented
- [x] Optimization strategies specified

---

**Status**: Ready for Multi-Persona Review Round 2
**Version**: 2.0
**Last Updated**: December 7, 2025
