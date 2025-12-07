# S2: Sprint 2 - Enhanced Resume & Backup

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Awaiting Multi-Persona Approval
**Sprint Goal**: Implement enhanced resume with auto-recreation and session backup
**Prerequisites**:
- S1 Foundation ✅ Complete (schema v2, migration, locking, validation, fileutil)
- D4 Requirements ✅ Approved (9.3/10)

---

## Executive Summary

Sprint 2 builds on the foundation from S1 to implement **user-facing features** that enable session persistence across reboots. This includes automatic tmux session recreation, dynamic status computation, and session backup functionality.

**Scope**: 3 deliverables (of 11 total in Phase 3.5)
**Duration Estimate**: 2-3 days of focused development
**Dependencies**: S1 (manifest schema, locking, migration, fileutil)

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
**Estimated Effort**: 8 hours
**Dependencies**: S1 (locking, manifest schema), D2.1 (status computation)

**Tasks**:
1. Update `cmd/csm/resume.go`:
   - Use status computation to detect active/stopped/archived
   - Implement auto-recreation workflow for stopped sessions
   - Add worktree validation before recreation
   - Add unarchive flow for archived sessions
   - Add partial failure rollback (kill tmux if Claude fails)

2. Auto-recreation workflow (FR-5.2):
   ```
   1. ComputeStatus(manifest) → "stopped"
   2. Validate worktree exists (resolve symlinks)
   3. Create tmux session:
      tmux new-session -d -s <name> -c <worktree>
   4. Send Claude command:
      tmux send-keys -t <name> 'claude --resume <uuid>' C-m
   5. Attach to session:
      tmux attach-session -t <name>
   6. Update manifest last_activity
   ```

3. Exact commands (from D4):
   - Check: `tmux has-session -t <name>` (exit code 0 = exists)
   - Create: `tmux new-session -d -s <name> -c <worktree>`
   - Send: `tmux send-keys -t <name> 'claude --resume <uuid>' C-m`
   - Attach: `tmux attach-session -t <name>`

4. Worktree validation (FR-5.3):
   - Use `filepath.EvalSymlinks()` to resolve symlinks
   - Check if path exists with `os.Stat()`
   - If missing: error with suggestions:
     - "Update worktree path: `csm set <id> --worktree <new-path>`"
     - "Archive session: `csm archive <id>`"
     - "Force resume in current dir: `csm resume <id> --force`"

5. Partial failure rollback (FR-5.4):
   - If Claude fails to start, kill tmux session
   - Use `tmux kill-session -t <name>`
   - Error message: "Claude failed to start" (not "tmux failed")

6. Unarchive flow (FR-5.6):
   - Detect archived status
   - Prompt: "This session is archived. Unarchive and resume? (y/n)"
   - If yes: Set lifecycle to "", proceed with resume
   - If no: Abort with clear message

7. Lock integration:
   - Acquire lock before any manifest modifications
   - Release lock via defer (even on panic)
   - Use lock from S1 (`manifest.AcquireLock()`)

8. User messaging:
   - Active session: "Attaching to active session..."
   - Stopped session: "Session stopped, recreating..."
   - Success: "✅ Session recreated successfully"
   - Worktree missing: Clear error with suggestions
   - Archived: Prompt for unarchive

**Acceptance Criteria**:
- [ ] Active session (tmux exists): skips recreation, attaches directly
- [ ] Stopped session (tmux missing): triggers auto-recreation
- [ ] Archived session: prompts user to unarchive
- [ ] Stopped session detected correctly
- [ ] Tmux session created with correct name
- [ ] Tmux session started in correct worktree directory
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
- [ ] User prompted for unarchive
- [ ] Lock acquired before manifest modification
- [ ] Lock released on completion or error
- [ ] Resume with auto-recreation completes in < 3 seconds (NFR-1.1)

**Tests**:
- `resume_test.go`: Status detection, worktree validation
- `resume_integration_test.go`: Full auto-recreation workflow
- `resume_rollback_test.go`: Partial failure scenarios
- `resume_archive_test.go`: Unarchive flow

---

### D2.3: Backup Command (FR-6)
**Priority**: P1 (Should Have)
**Estimated Effort**: 8 hours
**Dependencies**: S1 (fileutil, manifest schema)

**Tasks**:
1. Create `cmd/csm/backup.go`:
   - `func runBackup(identifier string, format string, includeFiles bool) error`
   - Implement backup creation workflow
   - Support JSONL and Markdown formats
   - Optional file snapshots
   - Backup retention (keep last 10)
   - Latest symlink creation

2. Backup creation workflow (FR-6.1):
   ```
   1. Resolve identifier → manifest path
   2. Load manifest
   3. Create backup directory: ~/sessions/<session>/backups/<timestamp>/
   4. Copy manifest → session-info.yaml
   5. Extract conversation from history.jsonl → conversation.jsonl
   6. (Optional) Copy file-history/ → file-snapshots/
   7. Create/update 'latest' symlink
   8. Clean old backups (keep last 10)
   ```

3. Backup directory structure (from D4):
   ```
   ~/sessions/session-<name>/backups/2025-12-07_14-30-00/
   ├── session-info.yaml       # Manifest snapshot
   ├── conversation.jsonl      # Filtered history entries
   ├── conversation.md         # (if --format markdown)
   └── file-snapshots/         # (if --include-files)
   ```

4. Timestamp format:
   - Format: `2025-12-07_14-30-00` (YYYY-MM-DD_HH-MM-SS)
   - Use local time zone
   - Add microseconds if collision detected

5. Conversation extraction:
   - Read `~/.claude/history.jsonl`
   - Filter entries by session UUID
   - Write only matching entries to `conversation.jsonl`

6. Markdown format (FR-6.2):
   - Header: Session name, UUID, date range
   - Each message:
     - Timestamp
     - Role (user/assistant)
     - Content (formatted)
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

9. Latest symlink (FR-6.5):
   - Create/update `backups/latest` → most recent backup
   - Use relative path for symlink target
   - On Windows without symlink support: show warning, continue
   - Example: `latest → 2025-12-07_14-30-00`

10. User messaging (FR-6.6):
    - Show progress for each step
    - Example output:
      ```
      ✓ Manifest: backups/2025-12-07_14-30-00/session-info.yaml
      ✓ Found 193 messages
      ✓ Conversation: backups/2025-12-07_14-30-00/conversation.jsonl

      ✓ Backup complete: ~/sessions/session-claude-1/backups/2025-12-07_14-30-00
         Latest: ~/sessions/session-claude-1/backups/latest
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

**Acceptance Criteria**:
- [ ] Backup directory created with timestamp
- [ ] Manifest copied to session-info.yaml
- [ ] Conversation extracted from history.jsonl
- [ ] Only entries for this session UUID included
- [ ] `csm backup claude-1` creates conversation.jsonl
- [ ] `csm backup claude-1 --format markdown` creates conversation.md
- [ ] Markdown format includes headers, timestamps, formatted messages
- [ ] `csm backup claude-1 --include-files` copies file-history/ directory
- [ ] File snapshots copied to backup/file-snapshots/
- [ ] Without flag: file snapshots not copied
- [ ] Create 11 backups for a session → cleanup triggered
- [ ] 11th backup triggers cleanup
- [ ] Oldest backup deleted
- [ ] Only 10 most recent backups remain
- [ ] After backup, `backups/latest` symlink exists (if supported)
- [ ] Symlink points to most recent backup directory (relative path)
- [ ] Subsequent backup updates symlink
- [ ] On Windows without symlink support: warning shown, backup succeeds
- [ ] Shows progress for each step
- [ ] Shows final backup location
- [ ] Shows latest symlink location (if created)
- [ ] Lock acquired before manifest read
- [ ] Lock released on completion or error
- [ ] Backing up 200-message session completes in < 5 seconds (NFR-1.4)

**Tests**:
- `backup_test.go`: Backup creation, conversation extraction
- `backup_format_test.go`: JSONL and Markdown formats
- `backup_retention_test.go`: Cleanup old backups
- `backup_symlink_test.go`: Latest symlink creation
- `backup_integration_test.go`: Full backup workflow

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
- `resume_test.go`: Status detection, worktree validation
- `resume_integration_test.go`: Full auto-recreation workflow
- `resume_rollback_test.go`: Partial failure scenarios
- `resume_archive_test.go`: Unarchive flow

**D2.3 Backup**:
- `backup_test.go`: Backup creation, conversation extraction
- `backup_format_test.go`: JSONL and Markdown formats
- `backup_retention_test.go`: Cleanup old backups
- `backup_symlink_test.go`: Latest symlink creation
- `backup_integration_test.go`: Full backup workflow

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

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All P0 requirements: 100%

---

## Implementation Order

### Day 1 (Status & Resume Foundation)
1. Morning: Status Computation (3h)
2. Afternoon: Resume - Status Detection + Worktree Validation (3h)

### Day 2 (Resume Auto-Recreation)
3. Morning: Resume - Auto-Recreation Workflow (4h)
4. Afternoon: Resume - Rollback + Unarchive (3h)

### Day 3 (Backup)
5. Morning: Backup - Core Functionality (4h)
6. Afternoon: Backup - Formats + Retention (4h)

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

### Risk 3: History.jsonl Corrupted
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- ✅ Skip malformed lines (don't fail entire backup)
- ✅ Report count of skipped entries
- ✅ Create backup even if some entries corrupted
- ✅ Test with corrupted history file

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
- ✅ Test concurrent resume scenario

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

---

## Documentation Requirements

### Code Documentation
- [ ] Godoc comments on all exported functions
- [ ] Inline comments for complex logic
- [ ] tmux commands documented in code
- [ ] Backup format documented

### User Documentation (DR-1)
- [ ] Help text for `csm resume` (enhanced)
- [ ] Help text for `csm backup` (new command)
- [ ] Examples for backup formats
- [ ] Examples for auto-recreation

### Developer Documentation
- [ ] README updated with backup feature
- [ ] CHANGELOG entry for auto-recreation
- [ ] CHANGELOG entry for backup command

---

## Definition of Done

S2 is **DONE** when:

1. ✅ All 3 deliverables implemented and tested
2. ✅ All P0 and P1 acceptance criteria checked off
3. ✅ Test coverage >80% for critical paths
4. ✅ All tests passing (unit + integration + performance)
5. ✅ Code documented (godoc + inline)
6. ✅ Multi-persona review score ≥8.5/10
7. ✅ No known critical or high-severity bugs
8. ✅ Integration with S1 verified
9. ✅ Performance targets met
10. ✅ All code committed and pushed

---

## Files to Create/Modify

### New Files
```
cmd/csm/
  ├── status.go            # NEW - Status computation
  ├── status_test.go       # NEW - Tests
  ├── status_batch_test.go # NEW - Batch tests
  ├── resume_integration_test.go  # NEW - Tests
  ├── resume_rollback_test.go     # NEW - Tests
  ├── resume_archive_test.go      # NEW - Tests
  ├── backup.go            # NEW - Backup command
  ├── backup_test.go       # NEW - Tests
  ├── backup_format_test.go     # NEW - Tests
  ├── backup_retention_test.go  # NEW - Tests
  ├── backup_symlink_test.go    # NEW - Tests
  └── backup_integration_test.go # NEW - Tests
```

### Modified Files
```
cmd/csm/
  ├── resume.go            # MODIFY - Add auto-recreation
  ├── resume_test.go       # MODIFY - Add new tests
  └── list.go              # MODIFY - Use batch status computation
```

---

## Next Sprint Preview (Not in S2 Scope)

### S3: Health & Operations
- Doctor command (FR-7)
- Log rotation (OR-3)
- Integration tests across all features
- Performance benchmarks
- Estimated: 2-3 days

---

## Review Checklist

Before submitting for multi-persona review:

- [ ] All deliverables clearly defined
- [ ] All acceptance criteria listed
- [ ] All risks identified and mitigated
- [ ] Test strategy comprehensive
- [ ] Implementation order logical
- [ ] Dependencies on S1 identified
- [ ] Success metrics defined
- [ ] Documentation requirements clear
- [ ] Files to create/modify listed
- [ ] Definition of Done complete

---

**Status**: Ready for Multi-Persona Review Round 1
**Version**: 1.0
**Last Updated**: December 7, 2025
