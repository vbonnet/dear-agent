# S2 Sprint Plan Summary - Enhanced Resume & Backup

**Date**: December 7, 2025
**Status**: ✅ APPROVED (9.5/10)
**Commit**: 200b2e0

---

## Executive Summary

S2 Sprint Plan has been **APPROVED** with a score of **9.5/10** after two review rounds.

The comprehensive sprint plan defines all implementation details for Sprint 2, which focuses on user-facing features: automatic tmux session recreation (auto-recreation), dynamic status computation, and session backup with conversation history preservation.

---

## Review Results

### Round 1: 7.8/10 ❌ (Revision Needed)

**Critical gaps identified**:
- User messages not specified
- Backup file permissions not specified
- Tmux command sanitization missing
- Path validation missing
- Concurrent operation tests missing
- Post-deployment verification missing
- Rollback procedure missing

### Round 2: 9.5/10 ✅ (APPROVED)

**All critical issues resolved**:

| Reviewer | R1 Score | R2 Score | Change |
|----------|----------|----------|--------|
| Senior Go Developer | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| Software Architect | 8.5/10 | 9.5/10 | +1.0 ⬆️ |
| QA Engineer | 7.5/10 | 9.5/10 | +2.0 ⬆️ |
| DevOps/SRE | 7.5/10 | 9.5/10 | +2.0 ⬆️ |
| End User | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| Security Engineer | 7.5/10 | 9.5/10 | +2.0 ⬆️ |

**Average**: 9.5/10 ✅ **EXCEEDS THRESHOLD (8.5/10)**

---

## Sprint 2 Scope

**Goal**: Implement user-facing features that enable session persistence across reboots

**Deliverables** (3 of 11 total in Phase 3.5):

1. **D2.1: Status Computation** (FR-8)
   - Dynamic status computed from tmux state + lifecycle field
   - Batch optimization (single tmux query for all sessions)
   - Performance: List 50 sessions in < 1 second

2. **D2.2: Enhanced Resume with Auto-Recreation** (FR-5)
   - Detect stopped sessions (tmux missing)
   - Automatically recreate tmux session
   - Validate worktree exists
   - Handle archived sessions (prompt to unarchive)
   - Rollback on partial failure
   - **Security**: Tmux command sanitization, path validation
   - **UX**: --yes flag for non-interactive mode

3. **D2.3: Backup Command** (FR-6)
   - Backup conversation history (JSONL or Markdown)
   - Optional file snapshots
   - Atomic creation (temp dir → rename)
   - Backup retention (keep last 10)
   - Latest symlink
   - **Security**: File permissions (0600/0700), path validation
   - **Performance**: Stream processing for large history files

**Duration Estimate**: 2-3 days

---

## Key Technical Specifications

### User Messages & Error Specifications

**All messages specified with exact text** (like S1):

**Resume - Auto-Recreation**:
```
Session 'claude-myapp' stopped, recreating...
✓ Created tmux session
✓ Started Claude (UUID: e6121188-...)
✓ Session recreated successfully

Attaching to session...
```

**Backup - Progress**:
```
Creating backup for session 'claude-myapp'...
✓ Manifest: backups/2025-12-07_14-30-00-123456/session-info.yaml
✓ Found 193 messages in conversation history
✓ Conversation: backups/2025-12-07_14-30-00-123456/conversation.jsonl

✅ Backup complete: ~/sessions/session-claude-myapp/backups/2025-12-07_14-30-00-123456
```

**Error - Worktree Missing**:
```
Error: worktree directory not found: /home/user/projects/myapp

The project directory has been moved or deleted.

Try one of the following:
  • Update worktree path: csm set claude-myapp --worktree <new-path>
  • Archive session: csm archive claude-myapp
  • Force resume in current dir: csm resume claude-myapp --force
```

### Security Hardening

**Tmux Command Sanitization**:
```go
func sanitizeSessionName(name string) (string, error) {
    // Allow only: alphanumeric, hyphen, underscore
    validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
    if !validPattern.MatchString(name) {
        return "", fmt.Errorf("invalid session name: contains prohibited characters")
    }
    return name, nil
}
```

**Path Validation**:
```go
func validateBackupPath(sessionDir string, backupName string) error {
    clean := filepath.Clean(backupName)
    if strings.Contains(clean, "..") {
        return fmt.Errorf("invalid backup name: contains directory traversal")
    }
    // ... verify within session directory ...
}
```

**File Permissions**:
- Backup files: 0600 (user-only read/write)
- Backup directory: 0700 (user-only access)

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

**Stream processing to handle large files**:

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
        if entry.SessionID == sessionUUID {
            entries = append(entries, entry)
        }
    }

    if skippedCount > 0 {
        log.Printf("Skipped %d malformed entries in history.jsonl", skippedCount)
    }

    return entries, nil
}
```

### Backup Atomic Creation

**Create in temp directory, move when complete**:

```go
func createBackupAtomic(sessionDir string, manifest *Manifest) error {
    timestamp := generateBackupTimestamp()
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

## Major Improvements from v1 to v2

### Technical Specifications Section (NEW)
- User messages: All exact text with formatting
- Error messages: All exact text with suggestions
- File permissions: 0600/0700 specified
- Tmux sanitization: Code example with regex
- Path validation: Code example with traversal check
- Backup timestamp: Always microseconds
- History parsing: Stream processing code
- Atomic backup: Temp dir → rename pattern

### Security Hardening
- Tmux command sanitization (prevent injection)
- Path validation (prevent directory traversal)
- File permissions (0600/0700)
- Security tests: TS-S2-17 (injection), TS-S2-18 (traversal)

### Test Coverage Enhanced
- **21 integration tests** (was 10): TS-S2-1 through TS-S2-21
- **Concurrent operation tests**: Backup + resume concurrency
- **Security tests**: Injection, traversal
- **Edge cases**: Corrupted history, broken symlinks, disk full, large messages
- **Performance tests**: Resume <3s, backup <5s, list <1s, streaming memory <100MB

### Operational Readiness
- **Post-deployment verification**: 10-step checklist with exact commands
- **Rollback procedure**: Complete guide (git + artifact cleanup + communication)
- **When to rollback**: Clear criteria (corruption, injection, data loss, security)
- **Partial rollback**: Can disable individual features
- **Monitoring guidance**: Metrics, dashboards, alerts, thresholds

### User Experience
- **Help text drafts**: Complete text for `csm resume --help` and `csm backup --help`
- **User messages**: All specified with exact text and formatting
- **Error messages**: All include suggestions ("Try one of the following:")
- **--yes flag**: Non-interactive mode for scripting
- **Progress indication**: ✓ checkmarks for each step

### Implementation Details
- **Backup timestamp**: Microseconds always included (prevent collisions)
- **History parsing**: Stream processing with 10MB line limit (memory bounded)
- **Backup atomic creation**: Temp dir with defer cleanup (no partial backups)
- **Markdown formatting**: Code blocks, escaping, no truncation specified
- **Performance**: Memory usage bounded, disk usage tracked

---

## Files to Create

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

## Test Strategy

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All P0 requirements: 100%

### Integration Tests (21 scenarios)

**Happy Paths**:
1. TS-S2-1: Resume active session (attach directly)
2. TS-S2-2: Resume stopped session (auto-recreation)
6. TS-S2-6: Backup creation (JSONL)
7. TS-S2-7: Backup creation (Markdown)
8. TS-S2-8: Backup with file snapshots
10. TS-S2-10: Batch status computation

**Error Paths**:
3. TS-S2-3: Resume with missing worktree
4. TS-S2-4: Resume archived session
5. TS-S2-5: Partial failure rollback
14. TS-S2-14: Tmux command failure
16. TS-S2-16: Disk full during backup
21. TS-S2-21: History.jsonl not found

**Edge Cases**:
9. TS-S2-9: Backup retention (11th backup)
12. TS-S2-12: Corrupted history file
13. TS-S2-13: Broken symlink in worktree
15. TS-S2-15: Backup with large messages
19. TS-S2-19: Resume with --yes flag
20. TS-S2-20: Backup atomic failure

**Concurrency**:
11. TS-S2-11: Concurrent backup and resume

**Security**:
17. TS-S2-17: Session name command injection
18. TS-S2-18: Backup path traversal

### Performance Tests
- Resume performance: < 3 seconds (NFR-1.1)
- List performance: < 1 second for 50 sessions (NFR-1.2)
- Backup performance: < 5 seconds for 200 messages (NFR-1.4)
- Backup streaming: < 100MB peak memory (proves streaming works)

---

## Post-Deployment Verification

10-step checklist to verify S2 works:

1. **Status Computation Works**: Create active/stopped/archived, verify `csm list` shows correct status
2. **Auto-Recreation Works**: Kill tmux, resume, verify recreation
3. **Worktree Validation Works**: Delete worktree, verify error with suggestions
4. **Backup Works (JSONL)**: Create backup, verify directory/files/permissions
5. **Backup Works (Markdown)**: Create markdown backup, verify formatting
6. **Backup Retention Works**: Create 11 backups, verify only 10 remain
7. **Concurrent Operations Protected**: Concurrent resume + backup, verify lock prevents conflict
8. **Tmux Sanitization Works**: Try special characters, verify rejection
9. **Archived Session Flow Works**: Archive, resume, verify prompt (and --yes flag)
10. **Performance Acceptable**: Time list (50 sessions <1s) and resume (<3s)

---

## Rollback Procedure

If S2 has critical bugs:

### 1. Git Rollback
```bash
git revert 200b2e0
go build -o csm ./cmd/csm
```

### 2. Verify Old CSM Works
```bash
csm list
csm resume claude-test
```

### 3. Clean Up (Optional)
```bash
# Only if backups causing issues
find ~/sessions -type d -name "backups" -exec rm -rf {} +
```

**When to Rollback**:
- Auto-recreation corrupts manifests
- Command injection exploited
- Data loss in backup
- Deadlocks in concurrent operations
- Security issues exploited

**When NOT to Rollback**:
- Minor UI issues
- Non-critical bugs
- Individual failures
- Performance slightly below target

---

## Monitoring & Metrics

### Metrics to Track

**Success Rates**:
```bash
# Auto-recreation success rate
grep "Session recreated successfully" ~/.csm/logs/csm.log | wc -l
grep "Error.*failed to create tmux session" ~/.csm/logs/csm.log | wc -l

# Backup success rate
grep "Backup complete" ~/.csm/logs/csm.log | wc -l
grep "Error.*backup failed" ~/.csm/logs/csm.log | wc -l
```

**Disk Usage**:
```bash
# Total backup storage
du -sh ~/sessions/*/backups/

# Alert if > 10GB
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

### Alert Thresholds

- Auto-recreation success rate < 95% → Investigate
- Backup success rate < 95% → Investigate
- Resume duration > 5s → Performance issue
- Backup storage > 10GB → Cleanup needed
- Error rate > 1% → Investigate

---

## Help Text Drafts

### csm resume --help

Complete help text provided in S2-SPRINT-PLAN-v2.md including:
- Usage and flags
- Behavior for active/stopped/archived sessions
- Auto-recreation explanation
- Examples
- Troubleshooting
- See also links

### csm backup --help

Complete help text provided in S2-SPRINT-PLAN-v2.md including:
- Usage and flags
- Backup location and structure
- Retention policy
- Format comparison (JSONL vs Markdown)
- Examples
- Notes on edge cases
- See also links

---

## Success Criteria

S2 is **DONE** when:

1. ✅ All 3 deliverables implemented and tested
2. ✅ All P0 and P1 acceptance criteria met
3. ✅ Test coverage >80% critical, >60% overall
4. ✅ All tests passing (unit + integration + performance)
5. ✅ Code documented (godoc + inline)
6. ✅ All user messages match specifications
7. ✅ All error messages match specifications
8. ✅ Help text implemented
9. ✅ Multi-persona review ≥8.5/10
10. ✅ No critical bugs
11. ✅ Integration with S1 verified
12. ✅ Performance targets met
13. ✅ Security hardening complete
14. ✅ Post-deployment verification passed
15. ✅ Rollback procedure tested
16. ✅ All code committed

---

## Out of Scope (Later Sprints)

Not in S2:
- Doctor command (S3)
- Log rotation (S3)
- Archive/unarchive commands (Phase 4)
- Integration tests across sprints (S3)
- Performance benchmarks (S3)

---

## Files Created

- `S2-SPRINT-PLAN.md` (v1 - 7.8/10)
- `S2-SPRINT-PLAN-v2.md` (v2 - 9.5/10) ✅
- `S2-REVIEW-R1.md` (6 personas, detailed feedback)
- `S2-REVIEW-R2.md` (6 personas, final approval)
- `S2-SUMMARY.md` (this document)

**Commits**:
- `200b2e0` - S2 sprint plan and reviews

---

## Wayfinder Progress

- ✅ **D1 Discovery** - Research complete
- ✅ **D2 Architecture** - Approved 8.8/10
- ✅ **D3 Implementation Design** - Approved 9.0/10
- ✅ **D4 Requirements** - Approved 9.3/10
- ✅ **S1 Sprint Plan** - Approved 9.4/10
- ✅ **S2 Sprint Plan** - Approved 9.5/10 ← **CURRENT**
- ⏸️ **S2 Implementation** - Awaiting your approval to proceed

---

## Next Steps

**I'm now paused per Wayfinder methodology.**

You can:
1. **Approve and proceed** - Begin S2 implementation (2-3 days coding)
2. **Review sprint plan** - Examine S2-SPRINT-PLAN-v2.md and suggest changes
3. **Continue planning** - Move to S3 Sprint Plan (Doctor, Log Rotation, Operations)
4. **Different task** - Work on something else

All work is committed and ready for your review at `~/src/repos/ai-tools/base/claude-session-manager/wayfinder-projects/session-persistence/`.

---

**End of S2 Summary**
