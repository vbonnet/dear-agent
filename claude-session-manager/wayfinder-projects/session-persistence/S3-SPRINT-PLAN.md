# S3: Sprint 3 - Health, Operations & Testing

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Awaiting Multi-Persona Approval
**Sprint Goal**: Implement operational readiness features and comprehensive testing
**Prerequisites**:
- S1 Foundation ✅ Complete (schema v2, migration, locking, validation, fileutil)
- S2 User Features ✅ Approved (status, resume, backup)
- D4 Requirements ✅ Approved (9.3/10)

---

## Executive Summary

Sprint 3 completes Phase 3.5 by implementing operational readiness features and comprehensive testing infrastructure. This includes the doctor command for health checks, log rotation for long-term operation, and extensive integration and performance testing.

**Scope**: 3 deliverables (completes 11 total in Phase 3.5)
**Duration Estimate**: 2-3 days of focused development
**Dependencies**: S1 (all infrastructure), S2 (all user features)

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

## Deliverables

### D3.1: Doctor Command (FR-7)
**Priority**: P1 (Should Have)
**Estimated Effort**: 8 hours
**Dependencies**: S1 (manifest, locking), S2 (status computation)

**Tasks**:
1. Create `cmd/csm/doctor.go`:
   - `func runDoctor(identifier string, fix bool, quiet bool, checkMigrations bool) error`
   - Implement health check system
   - Support all/specific session checks
   - Auto-fix capability for stale locks
   - Quiet mode for automation

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
   func checkStaleLocks(sessionDir string) CheckResult
   func checkUUIDInHistory(uuid string) CheckResult
   func checkWorktreeExists(path string) CheckResult
   func checkMigrationBackups(sessionDir string) CheckResult
   ```

3. Health checks to implement (FR-7.1):
   - **Sessions directory exists**: Check `~/sessions/` (or configured path) exists
   - **All manifests valid**: Load and validate each manifest.yaml
   - **Stale lock detection**: Find lock files > 60s old
   - **Claude UUIDs in history**: Verify session UUIDs exist in ~/.claude/history.jsonl
   - **Worktrees exist**: Check worktree directories exist (resolve symlinks)
   - **Migration backups**: (With --check-migrations) Verify .v1.bak files present

4. Stale lock cleanup (FR-7.2):
   - `csm doctor` detects stale locks as warnings
   - `csm doctor --fix` removes stale locks (> 60s old)
   - Confirmation message for each fixed lock
   - Respects active locks (< 60s old)

5. Output modes (FR-7.3):
   - **Verbose mode** (default): Show all checks with ✓ or ✗
   - **Quiet mode** (`--quiet`): Show only warnings and errors
   - **Exit codes**: 0 = healthy, 1 = warnings, 2 = errors
   - Scriptable output for automation

6. Specific session check (FR-7.4):
   - `csm doctor <identifier>` checks only that session
   - Validates manifest
   - Checks worktree
   - Checks UUID in history
   - Focused output (not all sessions)

7. Output format:
   ```
   Running health checks...

   ✓ Sessions directory exists: ~/sessions
   ✓ Manifest valid: session-claude-1
   ✓ Manifest valid: session-claude-2
   ⚠ Stale lock detected: session-claude-3 (lock age: 125s)
   ✓ Worktree exists: session-claude-1 → /home/user/projects/app
   ✗ Worktree missing: session-claude-2 → /home/user/old-project

   Summary: 2 warnings, 1 error

   Run with --fix to automatically fix stale locks.
   ```

8. Fix mode output:
   ```
   Running health checks with auto-fix...

   ✓ Sessions directory exists
   ✓ All manifests valid
   ⚠ Stale lock detected: session-claude-3
     → Removed stale lock (lock age: 125s)
   ✓ Worktrees checked

   Summary: 1 warning fixed, 0 errors

   ✓ System healthy
   ```

9. Quiet mode output:
   ```bash
   $ csm doctor --quiet
   ⚠ Stale lock: session-claude-3 (125s old)
   ✗ Worktree missing: session-claude-2

   $ echo $?
   2
   ```

10. User messaging:
    - All messages specified (see Technical Specifications section below)
    - Clear pass/warn/fail indicators
    - Actionable suggestions for errors
    - Summary at end

**Acceptance Criteria**:
- [ ] `csm doctor` runs all health checks
- [ ] Sessions directory check: pass if exists, fail if missing
- [ ] Manifest validation: load and validate each manifest.yaml
- [ ] Invalid manifest: show error with path and reason
- [ ] Stale lock detection: warn if lock > 60s old
- [ ] Shows lock age in warning message
- [ ] Claude UUID check: parse history.jsonl, verify session UUIDs
- [ ] Missing UUID: warn (not error, session may be new)
- [ ] Worktree check: resolve symlinks, verify exists
- [ ] Missing worktree: show error with suggestions
- [ ] Migration backup check: (--check-migrations) verify .v1.bak files
- [ ] Missing backup: warn (not error, may be v2 native)
- [ ] Each check shows ✓ (pass), ⚠ (warn), or ✗ (fail)
- [ ] Failed checks show error details
- [ ] Summary shows count: "2 warnings, 1 error"
- [ ] `csm doctor --fix` removes stale locks
- [ ] Confirmation message for each removed lock
- [ ] Does not remove active locks (< 60s old)
- [ ] `csm doctor` verbose mode shows all checks
- [ ] `csm doctor --quiet` shows only warnings/errors
- [ ] Exit code 0: all healthy
- [ ] Exit code 1: warnings present
- [ ] Exit code 2: errors present
- [ ] `csm doctor claude-1` checks only that session
- [ ] Specific check shows focused output
- [ ] All user messages match Technical Specifications
- [ ] Doctor completes in < 2 seconds for 10 sessions

**Tests**:
- `doctor_test.go`: Individual check functions
- `doctor_integration_test.go`: Full health check workflow
- `doctor_fix_test.go`: Stale lock cleanup
- `doctor_quiet_test.go`: Output modes and exit codes
- `doctor_specific_test.go`: Single session check

---

### D3.2: Log Rotation (OR-3)
**Priority**: P1 (Should Have)
**Estimated Effort**: 4 hours
**Dependencies**: S1 (migration logging)

**Tasks**:
1. Create `internal/logging/rotate.go`:
   - `func RotateLog(logPath string, maxSize int64, keepCount int) error`
   - Implement rotation logic
   - Non-blocking rotation
   - Atomic file operations

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
      a. Rotate existing files: .log.4 → .log.5, .log.3 → .log.4, etc.
      b. Delete .log.5 if it would become .log.6
      c. Move current log: migration.log → migration.log.1
      d. Create new migration.log (empty)
      e. Continue writing to new file
   ```

4. Rotation triggers:
   - **Before write**: Check size before appending log entry
   - **Lazy rotation**: Only rotate when actually writing
   - **No background process**: Rotation on-demand only

5. Atomic operations:
   - Use temp files for rotation: `.log.tmp` → `.log.1`
   - Atomic renames (POSIX guarantees)
   - Cleanup temp files on failure

6. Integration with migration logger:
   ```go
   // Update internal/manifest/migrate.go
   func logMigration(status string, path string, err error) {
       // Check size before writing
       if needsRotation(migrationLogPath) {
           rotate.RotateLog(migrationLogPath, 10*1024*1024, 5)
       }

       // Append log entry
       f, _ := os.OpenFile(migrationLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
       defer f.Close()
       fmt.Fprintf(f, "[%s] %s: %s\n", time.Now().Format(time.RFC3339), status, path)
   }
   ```

7. Error handling:
   - Rotation failure: Log to stderr, continue with current file
   - Disk full: Log error, don't crash
   - Permissions: Log error, don't crash
   - Best-effort: Never fail operation due to log rotation failure

8. Performance:
   - Check file size: O(1) via os.Stat()
   - Rotation: O(n) where n = keepCount (max 5)
   - Non-blocking: No locks held during rotation

**Acceptance Criteria**:
- [ ] Log file < 10MB: no rotation
- [ ] Log file ≥ 10MB: rotation triggered
- [ ] Current log moved to .log.1
- [ ] Existing .log.1 → .log.2, .log.2 → .log.3, etc.
- [ ] .log.5 deleted before rotation (if exists)
- [ ] New migration.log created (empty)
- [ ] Only 5 rotated files retained after rotation
- [ ] Rotation uses atomic operations (temp file + rename)
- [ ] Rotation failure logged to stderr
- [ ] Migration continues even if rotation fails
- [ ] File permissions preserved (0600)
- [ ] Active writes not interrupted during rotation
- [ ] Rotation completes in < 100ms

**Tests**:
- `rotate_test.go`: Rotation logic, file operations
- `rotate_integration_test.go`: Integration with migration logger
- `rotate_edge_test.go`: Disk full, permissions, concurrent writes

---

### D3.3: Integration & Performance Testing
**Priority**: P0 (Must Have)
**Estimated Effort**: 8 hours
**Dependencies**: S1, S2, D3.1, D3.2

**Tasks**:
1. Create integration test suite:
   - `cmd/csm/integration_test.go`: End-to-end workflows
   - Tests across all sprints (S1 + S2 + S3)
   - Real file system operations
   - Real tmux interactions

2. Integration test scenarios:
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

3. Performance benchmark suite:
   - `cmd/csm/benchmark_test.go`: Go benchmarks
   - Measure all critical operations
   - Validate against NFR targets

4. Performance benchmarks:
   - **BM-1**: Resume auto-recreation (target: < 3s)
   - **BM-2**: List 50 sessions (target: < 1s)
   - **BM-3**: Backup 200 messages (target: < 5s)
   - **BM-4**: Doctor 50 sessions (target: < 2s)
   - **BM-5**: Migration v1 → v2 (target: < 100ms)
   - **BM-6**: Lock acquire/release (target: < 10ms)
   - **BM-7**: Manifest validation (target: < 1ms)
   - **BM-8**: Status computation (target: < 50ms for 50 sessions)

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
   - Test harness for tmux operations
   - Mock history.jsonl generation
   - Test data fixtures
   - Cleanup between tests

9. CI/CD readiness:
   - All tests runnable via `go test ./...`
   - No manual setup required
   - Isolated test environments
   - Deterministic results

**Acceptance Criteria**:
- [ ] TS-INT-1 through TS-INT-10 all passing
- [ ] BM-1 through BM-8 all meet targets
- [ ] All S1 tests passing
- [ ] All S2 tests passing
- [ ] All S3 tests passing
- [ ] No flaky tests (100% pass rate on 10 runs)
- [ ] Test coverage >80% for critical paths
- [ ] Test coverage >60% overall
- [ ] All tests complete in < 2 minutes total
- [ ] No resource leaks detected
- [ ] CI/CD ready (no manual steps)

**Tests**:
- `integration_test.go`: End-to-end scenarios
- `benchmark_test.go`: Performance benchmarks
- `load_test.go`: Resource usage under load
- `stress_test.go`: Edge cases and limits

---

## Technical Specifications

### Doctor Command - User Messages

**Verbose Mode (Default)**:
```
Running health checks...

✓ Sessions directory exists: ~/sessions
✓ Manifest valid: session-claude-1
✓ Manifest valid: session-claude-2
✗ Manifest invalid: session-claude-3 (invalid YAML: line 5)
⚠ Stale lock detected: session-claude-4 (lock age: 125s)
✓ Claude UUID found: session-claude-1 (e6121188-...)
⚠ Claude UUID not found: session-claude-5 (may be new session)
✓ Worktree exists: session-claude-1 → /home/user/projects/app
✗ Worktree missing: session-claude-2 → /home/user/old-project

Summary: 2 warnings, 2 errors

Suggestions:
  • Fix invalid manifest: edit ~/sessions/session-claude-3/manifest.yaml
  • Remove stale locks: csm doctor --fix
  • Update worktree: csm set claude-2 --worktree <new-path>
  • Or archive: csm archive claude-2
```

**Fix Mode**:
```
Running health checks with auto-fix...

✓ Sessions directory exists
✓ All manifests valid
⚠ Stale lock detected: session-claude-4
  → Removed stale lock: ~/sessions/session-claude-4/manifest.yaml.lock
  → Lock age: 125s (threshold: 60s)
✓ Claude UUIDs checked
✓ Worktrees checked

Summary: 1 warning fixed, 0 errors

✅ System healthy
```

**Quiet Mode**:
```
⚠ Stale lock: session-claude-4 (125s old)
✗ Manifest invalid: session-claude-3 (invalid YAML)
✗ Worktree missing: session-claude-2
```

**Specific Session**:
```
$ csm doctor claude-1

Checking session 'claude-1'...

✓ Manifest valid
✓ Schema version: 2.0
✓ Lifecycle: active
✓ Claude UUID: e6121188-... (found in history)
✓ Worktree: /home/user/projects/app (exists)
✓ No stale locks

✅ Session healthy
```

**All Healthy**:
```
Running health checks...

✓ Sessions directory exists
✓ All manifests valid (5 sessions)
✓ No stale locks
✓ All Claude UUIDs found
✓ All worktrees exist

✅ System healthy (0 warnings, 0 errors)
```

**Exit Codes**:
- `0`: All checks passed (healthy)
- `1`: Warnings present (stale locks, missing UUIDs)
- `2`: Errors present (invalid manifests, missing worktrees)

### Log Rotation - File Naming

**Before rotation** (log file ≥ 10MB):
```
~/.csm/logs/
├── migration.log (10.5 MB)
├── migration.log.1 (10.2 MB)
├── migration.log.2 (10.1 MB)
├── migration.log.3 (9.8 MB)
└── migration.log.4 (9.5 MB)
```

**After rotation**:
```
~/.csm/logs/
├── migration.log (0 bytes, newly created)
├── migration.log.1 (10.5 MB, was migration.log)
├── migration.log.2 (10.2 MB, was migration.log.1)
├── migration.log.3 (10.1 MB, was migration.log.2)
├── migration.log.4 (9.8 MB, was migration.log.3)
└── migration.log.5 (9.5 MB, was migration.log.4)
```

**Next rotation** (when migration.log ≥ 10MB again):
```
~/.csm/logs/
├── migration.log (0 bytes)
├── migration.log.1 (10.3 MB, was migration.log)
├── migration.log.2 (10.5 MB, was migration.log.1)
├── migration.log.3 (10.2 MB, was migration.log.2)
├── migration.log.4 (10.1 MB, was migration.log.3)
└── migration.log.5 (9.8 MB, was migration.log.4)
# migration.log.5 (was migration.log.4, 9.5 MB) - deleted
```

### Integration Test Scenarios

**TS-INT-1: Full Lifecycle**:
```go
func TestFullLifecycle(t *testing.T) {
    // 1. Create session
    runCmd("csm", "create", "test-session")

    // 2. Resume (should attach to existing)
    runCmd("csm", "resume", "test-session")

    // 3. Kill tmux
    killTmux("test-session")

    // 4. Resume (should auto-recreate)
    runCmd("csm", "resume", "test-session")
    verifyTmuxExists("test-session")

    // 5. Backup
    runCmd("csm", "backup", "test-session")
    verifyBackupExists("test-session")

    // 6. Doctor
    output := runCmd("csm", "doctor")
    assert.Contains(t, output, "✓ Manifest valid: session-test-session")

    // 7. Archive
    runCmd("csm", "archive", "test-session")
    status := getStatus("test-session")
    assert.Equal(t, "archived", status)
}
```

**TS-INT-4: Concurrent Operations**:
```go
func TestConcurrentOperations(t *testing.T) {
    // Create session
    runCmd("csm", "create", "test-concurrent")
    killTmux("test-concurrent")

    // Run concurrently: resume, backup, doctor
    var wg sync.WaitGroup
    wg.Add(3)

    go func() {
        defer wg.Done()
        runCmd("csm", "resume", "test-concurrent")
    }()

    go func() {
        defer wg.Done()
        runCmd("csm", "backup", "test-concurrent")
    }()

    go func() {
        defer wg.Done()
        runCmd("csm", "doctor", "test-concurrent")
    }()

    wg.Wait()

    // Verify: All operations succeeded, no corruption
    verifyManifestValid("test-concurrent")
    verifyTmuxExists("test-concurrent")
    verifyBackupExists("test-concurrent")
}
```

### Performance Benchmarks

**BM-1: Resume Auto-Recreation**:
```go
func BenchmarkResumeAutoRecreation(b *testing.B) {
    // Setup: Session with stopped tmux
    setup("bench-resume")
    killTmux("bench-resume")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        runCmd("csm", "resume", "bench-resume")
        killTmux("bench-resume")
    }

    // Target: < 3 seconds per iteration
}
```

**BM-2: List Performance**:
```go
func BenchmarkList50Sessions(b *testing.B) {
    // Setup: Create 50 sessions
    for i := 0; i < 50; i++ {
        createSession(fmt.Sprintf("bench-%d", i))
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        runCmd("csm", "list")
    }

    // Target: < 1 second per iteration
}
```

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
// Doctor verifies UUIDs in history (similar to backup)
entries := extractConversation(historyPath, manifest.SessionID)
if len(entries) == 0 {
    return CheckResult{Status: "warn", Message: "UUID not found (may be new)"}
}
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

---

## Testing Strategy

### Unit Tests (per-deliverable)

**D3.1 Doctor Command**:
- `doctor_test.go`: Individual check functions
- `doctor_integration_test.go`: Full health check workflow
- `doctor_fix_test.go`: Stale lock cleanup
- `doctor_quiet_test.go`: Output modes and exit codes
- `doctor_specific_test.go`: Single session check

**D3.2 Log Rotation**:
- `rotate_test.go`: Rotation logic, file operations
- `rotate_integration_test.go`: Integration with migration logger
- `rotate_edge_test.go`: Disk full, permissions, concurrent writes

**D3.3 Integration Testing**:
- `integration_test.go`: End-to-end scenarios (TS-INT-1 through TS-INT-10)
- `benchmark_test.go`: Performance benchmarks (BM-1 through BM-8)
- `load_test.go`: Resource usage under load
- `stress_test.go`: Edge cases and limits

### Integration Tests (S3 scope)

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

**BM-5: Migration v1 → v2**:
- Target: < 100ms

**BM-6: Lock Acquire/Release**:
- Target: < 10ms

**BM-7: Manifest Validation**:
- Target: < 1ms

**BM-8: Status Computation Batch**:
- Target: < 50ms for 50 sessions

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All P0 requirements: 100%

---

## Implementation Order

### Day 1 (Doctor Command)
1. Morning: Doctor infrastructure + health checks (4h)
2. Afternoon: Fix mode + output modes (4h)

### Day 2 (Log Rotation + Integration Tests)
3. Morning: Log rotation implementation (4h)
4. Afternoon: Integration test infrastructure (4h)

### Day 3 (Testing & Benchmarks)
5. Morning: Integration tests (TS-INT-1 through TS-INT-10) (4h)
6. Afternoon: Performance benchmarks (BM-1 through BM-8) + validation (4h)

---

## Risk Management

### Risk 1: Doctor Removes Active Locks
**Probability**: LOW
**Impact**: HIGH
**Mitigation**:
- ✅ Respect lock timeout (60s, not shorter)
- ✅ Check lock timestamp before removal
- ✅ Test concurrent doctor + resume
- ✅ Never remove locks < 60s old

### Risk 2: Log Rotation Interrupts Active Writes
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Rotation only on write (not background process)
- ✅ Atomic operations (temp file + rename)
- ✅ Best-effort (don't fail on rotation error)
- ✅ Test concurrent writes during rotation

### Risk 3: Integration Tests Flaky
**Probability**: MEDIUM
**Impact**: MEDIUM
**Mitigation**:
- ✅ Isolated test environments
- ✅ Cleanup between tests
- ✅ Deterministic test data
- ✅ Retry logic for tmux operations
- ✅ Run 10 times to verify stability

### Risk 4: Performance Tests Don't Meet Targets
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Profile before finalizing
- ✅ Optimize hot paths
- ✅ Batch operations where possible
- ✅ Document if targets need adjustment

### Risk 5: Doctor False Positives
**Probability**: MEDIUM
**Impact**: LOW
**Mitigation**:
- ✅ UUID not found = warning (not error, may be new session)
- ✅ Migration backup missing = warning (may be v2 native)
- ✅ Clear messaging on what's expected vs actual

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
- [ ] No flaky tests

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

### User Documentation (DR-1)
- [ ] Help text for `csm doctor`
- [ ] Examples for doctor modes
- [ ] Troubleshooting guide
- [ ] Log rotation behavior documented

### Developer Documentation
- [ ] README updated with doctor command
- [ ] CHANGELOG entry for S3 features
- [ ] Testing guide for contributors
- [ ] Performance benchmarking guide

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
  --quiet               Show only warnings and errors (for scripting)
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

  # Check specific session
  csm doctor claude-myapp

  # Auto-fix stale locks
  csm doctor --fix

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

# Make it old (or wait 61 seconds)
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
# - Lock file deleted
# - Exit code 0

# Verify lock removed
ls ~/sessions/session-claude-1/manifest.yaml.lock
# Should show "No such file"
```

### 4. Doctor Quiet Mode Works

```bash
# Create issue
rm -rf /tmp/test-worktree
# (Assume session uses this worktree)

# Quiet mode
csm doctor --quiet

# Verify:
# - Shows only "✗ Worktree missing: ..."
# - No "✓" lines shown
# - Exit code 2
```

### 5. Doctor Specific Session Works

```bash
csm doctor claude-1

# Verify:
# - Shows "Checking session 'claude-1'..."
# - Only checks for that session
# - Focused output
```

### 6. Log Rotation Works

```bash
# Check current log size
ls -lh ~/.csm/logs/migration.log

# If < 10MB, create many migrations to trigger rotation
for i in {1..1000}; do
    # Trigger migrations (if you have test v1 manifests)
    # Or manually append to log to exceed 10MB
done

# After exceeding 10MB, next migration should rotate
# Verify:
ls -lh ~/.csm/logs/
# - migration.log (small, newly created)
# - migration.log.1 (large, old file)
```

### 7. Integration Tests Pass

```bash
# Run all tests
cd ~/src/repos/ai-tools/base/claude-session-manager
go test ./cmd/csm/... -v

# Verify:
# - All tests pass
# - Integration tests complete
# - Performance benchmarks run
```

### 8. Performance Meets Targets

```bash
# Run benchmarks
go test ./cmd/csm/... -bench=. -benchtime=10s

# Verify:
# - BenchmarkResumeAutoRecreation: < 3s per op
# - BenchmarkList50Sessions: < 1s per op
# - BenchmarkBackup200Messages: < 5s per op
# - BenchmarkDoctor50Sessions: < 2s per op
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
```

### When to Rollback

**Critical issues**:
- Doctor removes active locks (corruption risk)
- Log rotation corrupts logs
- Integration tests reveal cross-sprint bugs
- Performance regression in S1/S2 features

**When NOT to Rollback**:
- Doctor false positives (can be fixed in patch)
- Minor UI issues in doctor output
- Single test flakiness
- Performance slightly below target

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
grep "Removed stale lock" ~/.csm/logs/csm.log | wc -l
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

### Alert Thresholds

- Doctor detects errors: Alert immediately
- Stale locks > 5: Investigate (may indicate crashes)
- Log storage > 100MB: Cleanup needed
- Performance degradation > 20%: Investigate

---

## Definition of Done

S3 is **DONE** when:

1. ✅ All 3 deliverables implemented and tested
2. ✅ All P0 and P1 acceptance criteria met
3. ✅ Doctor command fully functional
4. ✅ Log rotation implemented
5. ✅ All integration tests passing
6. ✅ All performance benchmarks meeting targets
7. ✅ Test coverage >80% critical, >60% overall
8. ✅ Code documented (godoc + inline)
9. ✅ Help text implemented for doctor
10. ✅ Multi-persona review ≥8.5/10
11. ✅ No critical bugs
12. ✅ Post-deployment verification passed
13. ✅ Rollback procedure tested
14. ✅ All code committed

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
  ├── integration_test.go            # NEW - End-to-end tests
  ├── benchmark_test.go              # NEW - Performance benchmarks
  ├── load_test.go                   # NEW - Load testing
  └── stress_test.go                 # NEW - Stress testing

internal/logging/
  ├── rotate.go                      # NEW - Log rotation
  ├── rotate_test.go                 # NEW - Tests
  ├── rotate_integration_test.go     # NEW - Tests
  └── rotate_edge_test.go            # NEW - Tests
```

### Modified Files
```
internal/manifest/
  └── migrate.go                     # MODIFY - Add rotation before logging

cmd/csm/
  └── main.go                        # MODIFY - Add doctor command
```

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

- [ ] All deliverables clearly defined
- [ ] All acceptance criteria listed
- [ ] All risks identified and mitigated
- [ ] Test strategy comprehensive
- [ ] Implementation order logical
- [ ] Dependencies on S1/S2 identified
- [ ] Success metrics defined
- [ ] Documentation requirements clear
- [ ] Files to create/modify listed
- [ ] Definition of Done complete
- [ ] Technical specifications added
- [ ] Help text drafts added
- [ ] Post-deployment verification added
- [ ] Rollback procedure added
- [ ] Monitoring guidance added

---

**Status**: Ready for Multi-Persona Review Round 1
**Version**: 1.0
**Last Updated**: December 7, 2025
