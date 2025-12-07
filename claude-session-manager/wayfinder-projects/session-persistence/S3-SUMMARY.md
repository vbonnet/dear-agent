# S3 Sprint Plan Summary - Health, Operations & Testing

**Date**: December 7, 2025
**Status**: ✅ APPROVED (9.5/10)
**Commit**: 17a125c

---

## Executive Summary

S3 Sprint Plan has been **APPROVED** with a score of **9.5/10** after two review rounds.

The comprehensive sprint plan defines all implementation details for Sprint 3, which completes Phase 3.5 by implementing operational readiness features: doctor command for health checks, log rotation for sustainable operation, and comprehensive integration and performance testing.

---

## Review Results

### Round 1: 8.0/10 ❌ (Revision Needed)

**Critical gaps identified**:
- Doctor history.jsonl parsing strategy not specified
- Doctor UUID check not optimized (N file reads)
- Doctor fix mode lock age check insufficient
- Test time budget unclear (fast vs slow)
- Rotation temp file permissions not specified
- Missing integration test scenarios
- No test data preparation details

### Round 2: 9.5/10 ✅ (APPROVED)

**All critical issues resolved**:

| Reviewer | R1 Score | R2 Score | Change |
|----------|----------|----------|--------|
| Senior Go Developer | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| Software Architect | 8.5/10 | 9.5/10 | +1.0 ⬆️ |
| QA Engineer | 7.5/10 | 9.5/10 | +2.0 ⬆️ |
| DevOps/SRE | 7.5/10 | 9.5/10 | +2.0 ⬆️ |
| End User | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| Security Engineer | 8.5/10 | 9.5/10 | +1.0 ⬆️ |

**Average**: 9.5/10 ✅ **EXCEEDS THRESHOLD (8.5/10)**

---

## Sprint 3 Scope

**Goal**: Make CSM production-ready through health checks, operational sustainability, and comprehensive testing

**Deliverables** (3 of 11 total in Phase 3.5):

1. **D3.1: Doctor Command** (FR-7)
   - Health checks: Sessions directory, manifests, locks, UUIDs, worktrees
   - Auto-fix capability: Remove stale locks (> 60s)
   - Output modes: Verbose, summary, quiet, dry-run
   - **Optimization**: Single history.jsonl read, O(1) UUID lookup
   - **Safety**: Protect active locks (< 60s), log all fix actions
   - **UX**: --dry-run preview, --summary for many sessions

2. **D3.2: Log Rotation** (OR-3)
   - Policy: Rotate at 10MB, keep 5 files
   - Atomic operations: Temp file + rename
   - **Security**: Temp files with 0600, preserve permissions
   - **Reliability**: Fallback to /tmp on disk full/permissions
   - **Recovery**: Stale .tmp cleanup

3. **D3.3: Integration & Performance Testing**
   - 15 integration tests (TS-INT-1 through TS-INT-15)
   - 9 performance benchmarks (BM-1 through BM-9)
   - **Organization**: Fast (< 2 min for CI) vs slow (nightly)
   - **Infrastructure**: Test fixtures, mock tmux, helpers
   - **Isolation**: Each test uses /tmp/csm-test-XXX

**Duration Estimate**: 2-3 days

---

## Key Technical Specifications

### Doctor Command - UUID Check Optimization

**Problem**: Checking N sessions × history.jsonl = O(N²) file reads

**Solution**: Parse history.jsonl once, cache all UUIDs

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
            skippedCount++
            continue
        }
        if entry.SessionID != "" {
            uuids[entry.SessionID] = true
        }
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
    // O(1) lookup for each session
    for _, m := range manifests {
        if uuids[m.SessionID] {
            // Pass
        } else {
            // Warn (not error, may be new)
        }
    }
}
```

**Performance**: 50 sessions: O(N) single read vs O(N²) 50 reads

### Doctor Command - Lock Age Check

**Problem**: Doctor might remove active locks during concurrent resume

**Solution**: Check timestamp from lock file (not file mtime), protect < 60s

```go
func checkStaleLocks(sessionDir string, fix bool) []CheckResult {
    lockPath := filepath.Join(sessionDir, "manifest.yaml.lock")

    // Read lock file
    data, err := os.ReadFile(lockPath)
    lines := strings.Split(strings.TrimSpace(string(data)), "\n")

    // Parse timestamp from lock file (line 2)
    lockTime, err := time.Parse(time.RFC3339, lines[1])
    age := time.Since(lockTime)

    // CRITICAL: Only remove locks > 60s old
    if age < 60*time.Second {
        return []CheckResult{{
            Status: "pass",
            Message: fmt.Sprintf("Active lock (age: %ds)", int(age.Seconds())),
        }}
    }

    // Stale lock detected (> 60s)
    if fix {
        os.Remove(lockPath)
        // Log fix action
        logDoctorFix("REMOVED_STALE_LOCK", lockPath, age)
    }
}
```

### Log Rotation - Fallback Strategy

**Problem**: Disk full or permissions during rotation

**Solution**: Multi-level fallback

```go
func logMigration(status string, path string, err error) {
    logPath := filepath.Join(os.Getenv("HOME"), ".csm", "logs", "migration.log")

    // Attempt rotation
    if needsRotation(logPath) {
        rotateErr := rotate.RotateLog(logPath, 10*1024*1024, 5)
        if rotateErr != nil {
            fmt.Fprintf(os.Stderr, "Warning: log rotation failed: %v\n", rotateErr)
        }
    }

    // Try current log
    f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if err != nil {
        // Fallback to /tmp
        f, err = os.OpenFile("/tmp/csm-migration-fallback.log",
            os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
        if err != nil {
            // Even fallback failed, stderr only
            fmt.Fprintf(os.Stderr, "Error: cannot write migration log: %v\n", err)
            return
        }
    }
    defer f.Close()

    // Write log entry
    fmt.Fprintf(f, "[%s] %s: %s\n", time.Now().Format(time.RFC3339), status, path)
}
```

**Levels**:
1. Normal: ~/.csm/logs/migration.log
2. Fallback: /tmp/csm-migration-fallback.log
3. Last resort: stderr only

### Test Data Preparation

**Location**: `cmd/csm/testdata/`

**Structure**:
```
cmd/csm/testdata/
├── manifests/
│   ├── v1-simple.yaml
│   ├── v2-complete.yaml
│   └── v2-archived.yaml
├── history/
│   ├── simple.jsonl (10 messages)
│   ├── medium.jsonl (200 messages)
│   └── large.jsonl (1000+ messages)
└── worktrees/
    └── sample-project/
```

**Mock Tmux Strategy**:

```go
// For CI (no tmux)
type TmuxMock struct {
    sessions map[string]bool
}

func (m *TmuxMock) HasSession(name string) bool {
    return m.sessions[name]
}

// For local testing (tmux available)
func TestResumeWithRealTmux(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping tmux integration test in short mode")
    }
    // Use real tmux commands
}
```

---

## Major Improvements from v1 to v2

### Technical Specifications Section (NEW)
- Doctor history parsing: Stream processing with code example
- Doctor UUID optimization: Single read, cached map, O(1) lookup
- Doctor lock age check: Timestamp from lock file, protect < 60s
- Rotation temp file: Code example with 0600
- Rotation fallback: Multi-level strategy with code
- Check execution order: Directory → Manifests → Locks → UUIDs → Worktrees

### Doctor Optimizations
- **UUID check**: O(N) vs O(N²) - single history read for all sessions
- **Lock safety**: Timestamp check (not file mtime), protect < 60s active locks
- **Fix logging**: All removals logged to migration.log
- **Idempotency**: Safe to run multiple times

### Doctor New Capabilities
- **--dry-run flag**: Preview fixes without applying
- **--summary flag**: Aggregated results for 50+ sessions
- **Specific suggestions**: Per issue type guidance

### Log Rotation Hardening
- **Temp file permissions**: .log.tmp with 0600
- **Stale .tmp cleanup**: Recovery from previous crash
- **Fallback strategy**: /tmp with 0600 on disk full
- **Permission preservation**: All rotated files keep 0600

### Test Infrastructure (NEW Section)
- **Test data prep**: Fixtures in testdata/ directory
- **Mock strategy**: CI-friendly tmux mocking
- **Test isolation**: /tmp/csm-test-XXX per test
- **Fast vs slow**: < 2 min for CI, nightly for stability
- **Memory leak detection**: pprof integration
- **Test helpers**: Setup, cleanup, mocking

### Additional Tests
- **Integration**: 15 scenarios (was 10)
  - TS-INT-11: Doctor fix + concurrent resume
  - TS-INT-12: Rotation disk full
  - TS-INT-13: Doctor empty system
  - TS-INT-14: Doctor new session (UUID not in history)
  - TS-INT-15: Rotation stale .tmp
- **Benchmarks**: 9 total (was 8)
  - BM-9: Log rotation (< 100ms)

### Operational Guidance
- **Doctor scheduling**: Cron and systemd timer examples
- **Alert integration**: Email, Slack, PagerDuty webhooks
- **Log analysis**: Commands for analyzing rotated logs
- **Disk monitoring**: Alert if logs exceed 100MB
- **Stale lock rationale**: Why 60s threshold

---

## Files to Create

### New Files
```
cmd/csm/
  ├── doctor.go                      # NEW - Doctor command
  ├── doctor_test.go                 # NEW - Tests
  ├── doctor_integration_test.go     # NEW - Tests
  ├── doctor_fix_test.go             # NEW - Tests
  ├── doctor_quiet_test.go           # NEW - Tests
  ├── doctor_specific_test.go        # NEW - Tests
  ├── doctor_optimization_test.go    # NEW - UUID check performance
  ├── doctor_dryrun_test.go          # NEW - Dry-run mode
  ├── integration_test.go            # NEW - End-to-end tests
  ├── benchmark_test.go              # NEW - Performance benchmarks
  ├── load_test.go                   # NEW - Load testing
  ├── stress_test.go                 # NEW - Stress testing
  ├── test_helpers.go                # NEW - Test utilities
  └── testdata/                      # NEW - Test fixtures

internal/logging/
  ├── rotate.go                      # NEW - Log rotation
  ├── rotate_test.go                 # NEW - Tests
  ├── rotate_integration_test.go     # NEW - Tests
  ├── rotate_edge_test.go            # NEW - Tests
  └── rotate_fallback_test.go        # NEW - Fallback tests
```

### Modified Files
```
internal/manifest/
  └── migrate.go                     # MODIFY - Add rotation + fallback

cmd/csm/
  └── main.go                        # MODIFY - Add doctor command
```

---

## Testing Strategy

### Integration Tests (15 scenarios)

**Happy Paths**:
1. TS-INT-1: Full lifecycle
2. TS-INT-2: Reboot simulation
3. TS-INT-3: Migration + resume
6. TS-INT-6: Backup + rotation

**Concurrent Operations**:
4. TS-INT-4: Resume + backup + doctor concurrently
5. TS-INT-5: Doctor fixes + resume
11. TS-INT-11: Doctor fix + concurrent resume (NEW)

**Error Recovery**:
7. TS-INT-7: Error recovery (disk full)
12. TS-INT-12: Rotation disk full (NEW)

**Security**:
8. TS-INT-8: Security validation

**Performance**:
9. TS-INT-9: Performance under load (50 sessions)
10. TS-INT-10: Long-running stability (100 iterations)

**Edge Cases**:
13. TS-INT-13: Doctor empty system (NEW)
14. TS-INT-14: Doctor new session (UUID not in history) (NEW)
15. TS-INT-15: Rotation stale .tmp (NEW)

### Performance Benchmarks (9 total)

- BM-1: Resume auto-recreation (< 3s)
- BM-2: List 50 sessions (< 1s)
- BM-3: Backup 200 messages (< 5s)
- BM-4: Doctor 50 sessions (< 2s)
- BM-5: Migration v1 → v2 (< 100ms)
- BM-6: Lock acquire/release (< 10ms)
- BM-7: Manifest validation (< 1ms)
- BM-8: Status computation (< 50ms for 50 sessions)
- BM-9: Log rotation (< 100ms) (NEW)

### Test Organization

**Fast Tests (< 2 minutes, for CI/PR)**:
- All unit tests
- Quick integration tests (single session)
- Quick benchmarks (1 iteration for validation)

```bash
# Run in CI
go test ./... -short -timeout=2m
```

**Slow Tests (nightly builds)**:
- Load testing (100 sessions)
- Stress testing (concurrent operations)
- Long-running stability (100 iterations)
- Full benchmarks (10+ iterations)

```bash
# Run nightly
go test ./... -timeout=30m
```

---

## Doctor Command Examples

### Basic Usage

```bash
# Check all sessions (verbose)
$ csm doctor
Running health checks...
✓ Sessions directory exists: ~/sessions
✓ Manifest valid: session-claude-1
✓ Manifest valid: session-claude-2
⚠ Stale lock detected: session-claude-3 (lock age: 125s)
✓ UUID found: session-claude-1 (e6121188-...)
✓ Worktree exists: session-claude-1 → /home/user/projects/app

Summary: 1 warning, 0 errors

Suggestions:
  • Fix stale locks: csm doctor --fix
```

### Summary Mode (Many Sessions)

```bash
# Summary for 50+ sessions
$ csm doctor --summary
Running health checks...
✓ Sessions directory exists
✓ 50 manifests validated (all valid)
✓ No stale locks detected
✓ 48 UUIDs found in history (2 new sessions)
✓ 50 worktrees checked (all exist)

Summary: 0 warnings, 0 errors
```

### Fix Mode

```bash
# Auto-fix stale locks
$ csm doctor --fix
Running health checks with auto-fix...
✓ Sessions directory exists
✓ All manifests valid
⚠ Stale lock detected: session-claude-3
  → Removed stale lock: ~/sessions/session-claude-3/manifest.yaml.lock
  → Lock age: 125s (threshold: 60s)
  → Logged fix to migration.log

Summary: 1 warning fixed, 0 errors

✅ System healthy
```

### Dry-Run Mode

```bash
# Preview fixes without applying
$ csm doctor --fix --dry-run
Running health checks (dry-run mode)...
⚠ Stale lock detected: session-claude-3 (lock age: 125s)
  → Would remove: ~/sessions/session-claude-3/manifest.yaml.lock

Summary: 1 warning (would fix), 0 errors

Run without --dry-run to apply fixes.
```

---

## Operational Guidance

### Doctor Scheduling

**Cron (Daily at 2 AM)**:

```bash
# Add to crontab
0 2 * * * csm doctor --quiet --summary && echo "CSM healthy" || echo "ALERT: CSM issues" | mail -s "CSM Health" admin@example.com
```

**Systemd Timer**:

```bash
# /etc/systemd/system/csm-doctor.timer
[Unit]
Description=Run CSM doctor health checks daily

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

### Alert Integration

**Email**:
```bash
csm doctor --quiet || echo "CSM health check failed" | mail -s "CSM Alert" admin@example.com
```

**Slack**:
```bash
csm doctor --quiet || curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK \
  -d '{"text":"CSM health check failed"}'
```

**PagerDuty**:
```bash
csm doctor --quiet || curl -X POST https://events.pagerduty.com/v2/enqueue \
  -H 'Content-Type: application/json' \
  -d '{"routing_key":"YOUR_KEY","event_action":"trigger","payload":{"summary":"CSM health check failed"}}'
```

### Log Analysis

**View all logs**:
```bash
cat ~/.csm/logs/migration.log.{5..1} ~/.csm/logs/migration.log | less
```

**Search across rotated logs**:
```bash
grep "FAILED" ~/.csm/logs/migration.log*
```

**View doctor fix actions**:
```bash
grep "DOCTOR-FIX" ~/.csm/logs/migration.log*
```

---

## Success Criteria

S3 is **DONE** when:

1. ✅ All 3 deliverables implemented and tested
2. ✅ Doctor UUID check optimized (single history read)
3. ✅ Doctor fix mode protects active locks (< 60s)
4. ✅ Doctor fix actions logged
5. ✅ Log rotation with fallback
6. ✅ All integration tests passing (15 scenarios)
7. ✅ All performance benchmarks meeting targets (9 benchmarks)
8. ✅ Test coverage >80% critical, >60% overall
9. ✅ Fast tests < 2 min, slow tests documented
10. ✅ Multi-persona review ≥8.5/10
11. ✅ No critical bugs
12. ✅ Post-deployment verification passed
13. ✅ All code committed

---

## Phase 3.5 Completion

With S3 approval, **all 11 deliverables of Phase 3.5 are planned**:

**S1 Foundation (5)** - Approved 9.4/10:
1. ✅ Manifest schema v2
2. ✅ Migration v1 → v2
3. ✅ Context validation
4. ✅ File locking
5. ✅ Fileutil package

**S2 User Features (3)** - Approved 9.5/10:
6. ✅ Status computation
7. ✅ Enhanced resume (auto-recreation)
8. ✅ Backup command

**S3 Operations (3)** - Approved 9.5/10:
9. ✅ Doctor command
10. ✅ Log rotation
11. ✅ Integration & performance testing

**Phase 3.5: Planning Complete** → Ready for implementation

---

## Files Created

- `S3-SPRINT-PLAN.md` (v1 - 8.0/10)
- `S3-SPRINT-PLAN-v2.md` (v2 - 9.5/10) ✅
- `S3-REVIEW-R1.md` (6 personas, detailed feedback)
- `S3-REVIEW-R2.md` (6 personas, final approval)
- `S3-SUMMARY.md` (this document)

**Commits**:
- `17a125c` - S3 sprint plan and reviews

---

## Wayfinder Progress

- ✅ **D1 Discovery** - Research complete
- ✅ **D2 Architecture** - Approved 8.8/10
- ✅ **D3 Implementation Design** - Approved 9.0/10
- ✅ **D4 Requirements** - Approved 9.3/10
- ✅ **S1 Sprint Plan** - Approved 9.4/10
- ✅ **S2 Sprint Plan** - Approved 9.5/10
- ✅ **S3 Sprint Plan** - Approved 9.5/10 ← **CURRENT**
- ⏸️ **Phase 3.5 Implementation** - Awaiting your approval to proceed

---

## Next Steps

**I'm now paused per Wayfinder methodology.**

You can:
1. **Approve and proceed** - Begin Phase 3.5 implementation (S1+S2+S3, 6-9 days total)
2. **Review sprint plan** - Examine S3-SPRINT-PLAN-v2.md and suggest changes
3. **Implementation order** - Decide: S1 first, then S2, then S3? Or different order?
4. **Different task** - Work on something else

All work is committed and ready for your review at `~/src/repos/ai-tools/base/claude-session-manager/wayfinder-projects/session-persistence/`.

---

**End of S3 Summary**
