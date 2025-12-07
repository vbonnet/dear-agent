# D4: Requirements Specification - Session Persistence

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Awaiting Multi-Persona Approval
**Prerequisites**:
- D1 Discovery ✅ Complete
- D2 Architecture ✅ Approved (8.8/10)
- D3 Implementation ✅ Approved (9.0/10)

---

## Executive Summary

This document specifies the complete requirements for Phase 3.5 (Session Persistence Core). It defines functional requirements, non-functional requirements, acceptance criteria, and test scenarios.

**Scope**: Phase 3.5 only (11 deliverables)
**Out of Scope**: Phase 4 features (context management, archive commands)

---

## 1. Functional Requirements

### FR-1: Manifest Schema Version 2

**Priority**: CRITICAL
**Dependency**: None

#### FR-1.1: Schema Structure
**Requirement**: Manifest MUST support schema version 2.0 with the following structure:

```yaml
schema_version: "2.0"
session_id: string
lifecycle: string  # "" or "archived"
created_at: timestamp
last_activity: timestamp
context:
  purpose: string  # max 256 chars
  tags: [string]   # max 10 tags, each max 32 chars
  notes: string    # max 1024 chars
worktree:
  path: string
claude:
  session_id: string
  session_env_path: string
  file_history_path: string
  started_at: timestamp
  last_activity: timestamp
tmux:
  session_name: string
  window_name: string
  created_at: timestamp
```

**Acceptance Criteria**:
- [ ] Manifest struct includes all v2 fields
- [ ] YAML serialization/deserialization works correctly
- [ ] Old v1 fields are removed (no backward references)

#### FR-1.2: Context Field Validation
**Requirement**: Context fields MUST be validated on write:
- Purpose: max 256 characters
- Tags: max 10 tags, each max 32 characters, no whitespace
- Notes: max 1024 characters

**Acceptance Criteria**:
- [ ] Writing manifest with purpose > 256 chars returns validation error
- [ ] Writing manifest with > 10 tags returns validation error
- [ ] Writing manifest with tag > 32 chars returns validation error
- [ ] Writing manifest with tag containing whitespace returns validation error
- [ ] Writing manifest with notes > 1024 chars returns validation error
- [ ] Valid context passes validation

#### FR-1.3: Lifecycle Field
**Requirement**: Lifecycle field MUST only store "archived" state. Empty string means active/stopped (computed).

**Acceptance Criteria**:
- [ ] Setting lifecycle to "" is valid
- [ ] Setting lifecycle to "archived" is valid
- [ ] Setting lifecycle to any other value returns validation error
- [ ] Status is computed from lifecycle + tmux state, never stored

---

### FR-2: Schema Migration (v1 → v2)

**Priority**: CRITICAL
**Dependency**: FR-1

#### FR-2.1: Automatic Migration
**Requirement**: When loading a v1 manifest, CSM MUST automatically migrate it to v2.

**Acceptance Criteria**:
- [ ] Loading v1 manifest triggers migration
- [ ] Migration creates backup (.v1.bak)
- [ ] Migration writes v2 manifest
- [ ] Subsequent loads read v2 (no re-migration)

#### FR-2.2: Migration Backup
**Requirement**: Before migrating, CSM MUST create backup of v1 manifest as `manifest.yaml.v1.bak`.

**Acceptance Criteria**:
- [ ] Backup file created before migration
- [ ] Backup contains exact copy of v1 manifest
- [ ] If backup already exists, migration fails (prevents overwriting previous backup)

#### FR-2.3: Migration Rollback
**Requirement**: If migration write fails, CSM MUST restore original v1 manifest from backup.

**Acceptance Criteria**:
- [ ] Inject write failure (read-only directory)
- [ ] Migration fails gracefully
- [ ] Original v1 manifest restored from backup
- [ ] Error message indicates rollback occurred

#### FR-2.4: Migration Validation
**Requirement**: Migration MUST validate all fields before writing v2 manifest.

**Acceptance Criteria**:
- [ ] Migration with missing required fields fails
- [ ] Migration with invalid field types fails
- [ ] Migration with malformed YAML fails
- [ ] Only valid manifests are migrated

#### FR-2.5: Migration Logging
**Requirement**: All migrations MUST be logged to `~/.csm/logs/migration.log`.

**Acceptance Criteria**:
- [ ] Successful migration logged with timestamp and path
- [ ] Failed migration logged with error details
- [ ] Log file created if doesn't exist
- [ ] Log entries are append-only

#### FR-2.6: Migration User Messaging
**Requirement**: In interactive terminals, migration MUST show progress messages. In non-interactive contexts (pipes, CI/CD), messages MUST be suppressed.

**Acceptance Criteria**:
- [ ] In terminal: shows "📝 Migrating..." and "✅ Success"
- [ ] In pipe: no messages to stdout
- [ ] One-time notice shown on first migration per installation
- [ ] Notice file created: `~/.csm/.migration-notice-shown`

---

### FR-3: Context Validation

**Priority**: HIGH
**Dependency**: FR-1

#### FR-3.1: Validation on Write
**Requirement**: All manifest writes MUST validate context fields.

**Acceptance Criteria**:
- [ ] Write with invalid context returns error before touching file
- [ ] Write with valid context succeeds
- [ ] Error messages clearly indicate which field and constraint failed

#### FR-3.2: Validation Error Messages
**Requirement**: Validation errors MUST be clear and actionable.

**Example**:
```
Error: context validation failed: purpose too long (300 chars, max 256)
```

**Acceptance Criteria**:
- [ ] Error includes field name
- [ ] Error includes actual vs max length
- [ ] Error is user-friendly (not technical jargon)

---

### FR-4: File Locking

**Priority**: CRITICAL
**Dependency**: None

#### FR-4.1: Lock Acquisition
**Requirement**: Resume and other write operations MUST acquire exclusive lock on manifest.

**Acceptance Criteria**:
- [ ] First process acquires lock successfully
- [ ] Second concurrent process gets lock error
- [ ] Lock file created: `manifest.yaml.lock`
- [ ] Lock file contains PID and timestamp

#### FR-4.2: Lock Release
**Requirement**: Lock MUST be released when operation completes or fails.

**Acceptance Criteria**:
- [ ] Normal completion releases lock (removes .lock file)
- [ ] Error/panic releases lock via defer
- [ ] Lock file removed after release

#### FR-4.3: Stale Lock Detection
**Requirement**: Locks older than 60 seconds MUST be considered stale and removed automatically.

**Acceptance Criteria**:
- [ ] Create lock file with old timestamp (> 60s)
- [ ] Next lock acquisition detects staleness
- [ ] Stale lock removed automatically
- [ ] New lock acquired successfully

#### FR-4.4: Lock Error Messages
**Requirement**: Lock errors MUST indicate which process holds the lock.

**Example**:
```
Error: session is locked by process 12345 (try: kill 12345 or wait a minute)
```

**Acceptance Criteria**:
- [ ] Error message includes PID
- [ ] Error message suggests remediation

---

### FR-5: Enhanced Resume with Auto-Recreation

**Priority**: CRITICAL
**Dependency**: FR-1, FR-4

#### FR-5.1: Status Detection
**Requirement**: Resume MUST detect session status (active/stopped/archived) before attempting recreation.

**Acceptance Criteria**:
- [ ] Active session (tmux exists): skips recreation, attaches directly
- [ ] Stopped session (tmux missing): triggers auto-recreation
- [ ] Archived session: prompts user to unarchive

#### FR-5.2: Tmux Auto-Recreation
**Requirement**: For stopped sessions, resume MUST recreate tmux session automatically.

**Workflow**:
1. Check if tmux session exists (via `tmux has-session -t <name>`)
2. If not exists:
   a. Create tmux session: `tmux new-session -d -s <name> -c <worktree>`
   b. Send command to tmux: `claude --resume <uuid>`
3. Attach to session

**Acceptance Criteria**:
- [ ] Stopped session detected correctly
- [ ] Tmux session created with correct name
- [ ] Tmux session started in correct worktree directory
- [ ] Claude resumed with correct UUID
- [ ] User attached to tmux session

#### FR-5.3: Worktree Validation
**Requirement**: Before recreating tmux, resume MUST verify worktree directory exists.

**Acceptance Criteria**:
- [ ] Worktree exists: recreation proceeds
- [ ] Worktree missing: error with helpful suggestions
- [ ] Error suggests: update worktree path, archive session, or force resume in current dir

#### FR-5.4: Partial Failure Rollback
**Requirement**: If Claude fails to start, resume MUST clean up (kill tmux session).

**Acceptance Criteria**:
- [ ] Inject Claude failure (invalid UUID)
- [ ] Tmux session created
- [ ] Claude command fails
- [ ] Tmux session killed automatically
- [ ] Error indicates Claude failure, not tmux

#### FR-5.5: Manifest Update
**Requirement**: After successful resume, manifest last_activity MUST be updated.

**Acceptance Criteria**:
- [ ] last_activity timestamp updated to now()
- [ ] Manifest written to disk
- [ ] If write fails, warning shown but resume continues

#### FR-5.6: Unarchive Flow
**Requirement**: Resuming archived session MUST prompt user to unarchive.

**Acceptance Criteria**:
- [ ] Archived session detected
- [ ] User prompted: "This session is archived. Unarchive and resume? (y/n)"
- [ ] If yes: lifecycle set to "", resume proceeds
- [ ] If no: resume aborts with clear message

---

### FR-6: Backup Command

**Priority**: HIGH
**Dependency**: FR-1

#### FR-6.1: Backup Creation
**Requirement**: `csm backup <identifier>` MUST create timestamped backup directory.

**Structure**:
```
~/sessions/session-<name>/backups/2025-12-07_14-30-00/
├── session-info.yaml       # Manifest snapshot
├── conversation.jsonl      # Filtered history entries
└── conversation.md         # (if --format markdown)
```

**Acceptance Criteria**:
- [ ] Backup directory created with timestamp
- [ ] Manifest copied to session-info.yaml
- [ ] Conversation extracted from history.jsonl
- [ ] Only entries for this session UUID included

#### FR-6.2: Backup Formats
**Requirement**: Backup MUST support JSONL (default) and Markdown formats.

**Acceptance Criteria**:
- [ ] `csm backup claude-1` creates conversation.jsonl
- [ ] `csm backup claude-1 --format markdown` creates conversation.md
- [ ] Markdown format includes headers, timestamps, formatted messages

#### FR-6.3: File Snapshots (Optional)
**Requirement**: With `--include-files`, backup MUST copy file snapshots.

**Acceptance Criteria**:
- [ ] `csm backup claude-1 --include-files` copies file-history/ directory
- [ ] File snapshots copied to backup/file-snapshots/
- [ ] Without flag: file snapshots not copied

#### FR-6.4: Backup Retention
**Requirement**: Backup MUST keep only last 10 backups per session, auto-deleting older ones.

**Acceptance Criteria**:
- [ ] Create 11 backups for a session
- [ ] 11th backup triggers cleanup
- [ ] Oldest backup deleted
- [ ] Only 10 most recent backups remain

#### FR-6.5: Latest Symlink
**Requirement**: Backup MUST create/update `latest` symlink pointing to most recent backup.

**Acceptance Criteria**:
- [ ] After backup, `backups/latest` symlink exists
- [ ] Symlink points to most recent backup directory
- [ ] Subsequent backup updates symlink

#### FR-6.6: Backup Output
**Requirement**: Backup MUST show progress and final location.

**Example**:
```
✓ Manifest: backups/2025-12-07_14-30-00/session-info.yaml
✓ Found 193 messages
✓ Conversation: backups/2025-12-07_14-30-00/conversation.jsonl

✓ Backup complete: ~/sessions/session-claude-1/backups/2025-12-07_14-30-00
   Latest: ~/sessions/session-claude-1/backups/latest
```

**Acceptance Criteria**:
- [ ] Shows progress for each step
- [ ] Shows final backup location
- [ ] Shows latest symlink location

---

### FR-7: Doctor Command

**Priority**: HIGH
**Dependency**: FR-1, FR-2, FR-4

#### FR-7.1: Health Checks
**Requirement**: `csm doctor` MUST perform the following checks:

1. Sessions directory exists
2. All manifests load and validate
3. Stale lock files detection
4. Claude UUIDs exist in history.jsonl
5. Worktrees exist
6. (With --check-migrations) Migration backups present

**Acceptance Criteria**:
- [ ] Each check has clear pass/fail indicator (✓ or ✗)
- [ ] Failed checks show error details
- [ ] Summary shows count of warnings and errors

#### FR-7.2: Stale Lock Cleanup
**Requirement**: `csm doctor --fix` MUST remove stale lock files.

**Acceptance Criteria**:
- [ ] Create stale lock (> 60s old)
- [ ] `csm doctor` detects it as warning
- [ ] `csm doctor --fix` removes it
- [ ] Confirmation message shown

#### FR-7.3: Output Modes
**Requirement**: Doctor MUST support quiet mode for automation.

**Acceptance Criteria**:
- [ ] `csm doctor` shows all checks (verbose)
- [ ] `csm doctor --quiet` shows only warnings/errors
- [ ] Exit code 0 = healthy, non-zero = issues
- [ ] Quiet mode is scriptable

#### FR-7.4: Specific Session Check
**Requirement**: `csm doctor <identifier>` MUST check only specific session.

**Acceptance Criteria**:
- [ ] Validates manifest for specific session
- [ ] Checks worktree exists
- [ ] Checks Claude UUID in history
- [ ] Shows focused output (not all sessions)

---

### FR-8: Status Computation

**Priority**: HIGH
**Dependency**: FR-1

#### FR-8.1: Status Determination
**Requirement**: Session status MUST be computed dynamically:
- If lifecycle == "archived": return "archived"
- Else if tmux session exists: return "active"
- Else: return "stopped"

**Acceptance Criteria**:
- [ ] Archived session: status = "archived"
- [ ] Tmux exists + lifecycle empty: status = "active"
- [ ] Tmux missing + lifecycle empty: status = "stopped"
- [ ] Status never stored in manifest (computed on read)

#### FR-8.2: Batch Status Computation
**Requirement**: For list command, status MUST be computed in batch (single tmux query).

**Acceptance Criteria**:
- [ ] List 50 sessions: only 1 call to `tmux list-sessions`
- [ ] Statuses computed from cached tmux list
- [ ] Performance: list completes in < 1 second

---

### FR-9: Configurable Sessions Directory

**Priority**: MEDIUM
**Dependency**: None

#### FR-9.1: Configuration Hierarchy
**Requirement**: Sessions directory MUST be configurable via (priority order):
1. CLI flag: `--sessions-dir <path>`
2. Environment variable: `CSM_SESSIONS_DIR`
3. Config file: `~/.config/csm/config.yaml`
4. Default: `~/sessions`

**Acceptance Criteria**:
- [ ] CLI flag overrides all
- [ ] Env var overrides config file and default
- [ ] Config file overrides default
- [ ] Default used if nothing else set

#### FR-9.2: Path Expansion
**Requirement**: Sessions directory path MUST expand `~` to home directory.

**Acceptance Criteria**:
- [ ] `~/sessions` expands to `/home/user/sessions`
- [ ] Relative paths made absolute
- [ ] Symlinks resolved

---

### FR-10: Fileutil Package

**Priority**: MEDIUM
**Dependency**: None

#### FR-10.1: CopyFile
**Requirement**: `fileutil.CopyFile(src, dst)` MUST copy file with validation.

**Validation**:
- Source and destination are different
- Source exists and is not a directory
- Destination is writable

**Acceptance Criteria**:
- [ ] Valid copy succeeds
- [ ] Same src and dst returns error
- [ ] Source is directory returns error
- [ ] Permissions preserved

#### FR-10.2: WriteAtomic
**Requirement**: `fileutil.WriteAtomic(path, data)` MUST write atomically (temp file + rename).

**Acceptance Criteria**:
- [ ] Creates temp file (path + .tmp)
- [ ] Writes data to temp
- [ ] Renames temp to final path (atomic)
- [ ] On error, temp file removed

#### FR-10.3: CopyDirectory
**Requirement**: `fileutil.CopyDirectory(src, dst)` MUST recursively copy directory.

**Acceptance Criteria**:
- [ ] All files copied
- [ ] All subdirectories copied
- [ ] Permissions preserved
- [ ] Symbolic links handled correctly

---

## 2. Non-Functional Requirements

### NFR-1: Performance

#### NFR-1.1: Resume Auto-Recreation
**Requirement**: Resume with auto-recreation MUST complete in < 3 seconds.

**Measurement**: Time from command execution to tmux attach.

**Acceptance Criteria**:
- [ ] 10 test runs average < 3 seconds
- [ ] No run exceeds 5 seconds

#### NFR-1.2: List Command Performance
**Requirement**: `csm list` with 50 sessions MUST complete in < 1 second.

**Acceptance Criteria**:
- [ ] Create 50 test sessions
- [ ] Run `csm list` 10 times
- [ ] Average time < 1 second

#### NFR-1.3: Migration Performance
**Requirement**: Migration MUST add < 100ms to first manifest load.

**Acceptance Criteria**:
- [ ] Load v1 manifest (triggers migration)
- [ ] Load v2 manifest (no migration)
- [ ] Difference < 100ms

#### NFR-1.4: Backup Performance
**Requirement**: Backing up 200-message session MUST complete in < 5 seconds.

**Acceptance Criteria**:
- [ ] Create session with 200 messages in history.jsonl
- [ ] Run backup (JSONL format)
- [ ] Completes in < 5 seconds

### NFR-2: Reliability

#### NFR-2.1: Data Integrity
**Requirement**: No data loss during migration, backup, or resume.

**Acceptance Criteria**:
- [ ] Migration preserves all v1 data
- [ ] Backup captures all messages for session
- [ ] Resume doesn't corrupt manifest
- [ ] Concurrent operations don't corrupt files

#### NFR-2.2: Error Recovery
**Requirement**: All operations MUST fail gracefully with clear error messages.

**Acceptance Criteria**:
- [ ] File write failures don't leave partial files
- [ ] Lock failures indicate which process holds lock
- [ ] Migration failures rollback to original state
- [ ] No operation leaves system in inconsistent state

### NFR-3: Usability

#### NFR-3.1: Error Messages
**Requirement**: Error messages MUST be clear, concise, and actionable.

**Examples**:
- ✅ "worktree does not exist: /home/user/deleted-project (try: csm archive claude-1)"
- ❌ "os: no such file or directory"

**Acceptance Criteria**:
- [ ] No raw OS errors exposed to user
- [ ] Each error includes suggestion for resolution
- [ ] Errors use plain language (not jargon)

#### NFR-3.2: Output Formatting
**Requirement**: Command output MUST be consistent and well-formatted.

**Standards**:
- Success: ✓ or ✅ prefix
- Warning: ⚠ prefix
- Error: ✗ or ❌ prefix
- Info: No prefix or ℹ️

**Acceptance Criteria**:
- [ ] All commands use consistent formatting
- [ ] Unicode symbols work in terminals
- [ ] Fallback to ASCII if unicode not supported

### NFR-4: Maintainability

#### NFR-4.1: Code Organization
**Requirement**: Code MUST be organized by responsibility:
- `cmd/csm/` - Commands
- `internal/manifest/` - Data model
- `internal/fileutil/` - File utilities
- `internal/tmux/` - Tmux operations
- `internal/claude/` - Claude operations

**Acceptance Criteria**:
- [ ] No circular dependencies
- [ ] Each package has single responsibility
- [ ] Internal packages not exposed publicly

#### NFR-4.2: Test Coverage
**Requirement**: Code coverage MUST exceed 80% for critical paths.

**Critical paths**:
- Manifest load/write/validate
- Migration logic
- Lock acquisition/release
- Resume auto-recreation

**Acceptance Criteria**:
- [ ] Run `go test -cover`
- [ ] Coverage > 80% for critical packages
- [ ] All edge cases have tests

#### NFR-4.3: Documentation
**Requirement**: All public functions MUST have godoc comments.

**Acceptance Criteria**:
- [ ] Run `go doc` on all packages
- [ ] All exported functions documented
- [ ] Comments explain purpose, parameters, return values

---

## 3. Test Scenarios

### TS-1: Migration Happy Path

**Given**: A valid v1 manifest exists
**When**: User runs `csm list` (or any command that loads manifest)
**Then**:
- [ ] Migration backup created (.v1.bak)
- [ ] Migration logged to migration.log
- [ ] V2 manifest written successfully
- [ ] In terminal: migration message shown
- [ ] Subsequent loads use v2 (no re-migration)

### TS-2: Migration Failure with Rollback

**Given**: A valid v1 manifest exists
**When**: Migration write fails (read-only directory)
**Then**:
- [ ] Backup created before failure
- [ ] Write failure detected
- [ ] Original v1 manifest restored from backup
- [ ] Error message indicates rollback
- [ ] Migration failure logged to migration.log

### TS-3: Resume Active Session

**Given**: Session manifest exists with active tmux session
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Status detected as "active"
- [ ] No tmux recreation attempted
- [ ] User attached to existing tmux session
- [ ] Manifest last_activity updated

### TS-4: Resume Stopped Session (Auto-Recreation)

**Given**: Session manifest exists, tmux session does not exist
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Status detected as "stopped"
- [ ] Message shown: "Session stopped, recreating..."
- [ ] Tmux session created with correct name and worktree
- [ ] Claude resumed with correct UUID
- [ ] User attached to new tmux session
- [ ] Message shown: "Session recreated successfully"

### TS-5: Resume with Missing Worktree

**Given**: Session manifest exists, worktree directory deleted
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Worktree validation fails
- [ ] Clear error message shown
- [ ] Suggestions provided: update worktree, archive, or force
- [ ] No tmux session created
- [ ] No partial state left

### TS-6: Resume Archived Session

**Given**: Session manifest with lifecycle="archived"
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Archived status detected
- [ ] User prompted: "Unarchive and resume? (y/n)"
- [ ] If yes: lifecycle set to "", resume proceeds
- [ ] If no: command aborts with clear message

### TS-7: Concurrent Resume (Lock Conflict)

**Given**: Session manifest exists
**When**: Two users run `csm resume claude-1` simultaneously
**Then**:
- [ ] First process acquires lock
- [ ] Second process gets lock error
- [ ] Error includes PID of lock holder
- [ ] Error suggests remediation (wait or kill)
- [ ] First process completes successfully
- [ ] First process releases lock

### TS-8: Stale Lock Recovery

**Given**: Stale lock file exists (> 60 seconds old)
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Stale lock detected
- [ ] Stale lock removed automatically
- [ ] New lock acquired
- [ ] Resume proceeds normally

### TS-9: Partial Failure Rollback

**Given**: Session manifest exists, tmux missing
**When**: Resume creates tmux but Claude fails to start
**Then**:
- [ ] Tmux session created
- [ ] Claude command sent to tmux
- [ ] Claude fails (invalid UUID)
- [ ] Tmux session killed (rollback)
- [ ] Error message indicates Claude failure
- [ ] No orphaned tmux session left

### TS-10: Backup Creation

**Given**: Session with 100 messages in history
**When**: User runs `csm backup claude-1`
**Then**:
- [ ] Backup directory created with timestamp
- [ ] Manifest copied to session-info.yaml
- [ ] 100 messages extracted to conversation.jsonl
- [ ] "Latest" symlink created/updated
- [ ] Success message shown with path

### TS-11: Backup Retention

**Given**: Session has 10 existing backups
**When**: User runs `csm backup claude-1` (creates 11th)
**Then**:
- [ ] New backup created
- [ ] Oldest backup deleted
- [ ] Only 10 backups remain
- [ ] "Latest" symlink points to newest

### TS-12: Doctor Healthy System

**Given**: All sessions are healthy
**When**: User runs `csm doctor`
**Then**:
- [ ] All checks pass (✓ shown)
- [ ] Summary: "0 warnings, 0 errors"
- [ ] Message: "✓ CSM is healthy"
- [ ] Exit code 0

### TS-13: Doctor with Issues

**Given**: 2 stale locks, 1 missing worktree
**When**: User runs `csm doctor`
**Then**:
- [ ] Stale locks detected (⚠ 2 stale lock files)
- [ ] Missing worktree detected (⚠ 1 session has missing worktree)
- [ ] Summary: "2 warnings, 0 errors"
- [ ] Exit code non-zero

### TS-14: Doctor Auto-Fix

**Given**: 2 stale lock files exist
**When**: User runs `csm doctor --fix`
**Then**:
- [ ] Stale locks detected
- [ ] Stale locks removed
- [ ] Message: "✓ Cleaned 2 stale lock files"
- [ ] Summary: "0 warnings, 0 errors"

---

## 4. Acceptance Criteria Summary

### Phase 3.5 is complete when:

#### Must Have (All Required)
- [ ] All FR-1 through FR-10 requirements implemented
- [ ] All NFR performance targets met
- [ ] All 14 test scenarios pass
- [ ] Code coverage > 80% for critical paths
- [ ] No known critical bugs
- [ ] All commands documented (help text)

#### Should Have (Recommended)
- [ ] Migration guide written
- [ ] Troubleshooting FAQ created
- [ ] Deployment checklist ready
- [ ] Log rotation implemented for migration.log

#### Nice to Have (Optional)
- [ ] Benchmark tests for performance validation
- [ ] Stress tests with 100+ sessions
- [ ] Integration test simulating reboot

---

## 5. Out of Scope (Phase 4)

The following are explicitly **NOT** part of Phase 3.5:

- [ ] `csm set` command (context management)
- [ ] `csm info` command (detailed session info)
- [ ] `csm archive` / `csm unarchive` commands
- [ ] Interactive session picker
- [ ] Session templates
- [ ] Multi-machine sync
- [ ] Telemetry/metrics collection

These will be addressed in Phase 4.

---

## 6. Dependencies

### External Dependencies
- Go 1.19+ (for development)
- tmux (runtime requirement)
- Claude CLI (runtime requirement)
- yaml.v3 library (for YAML parsing)

### Internal Dependencies
- Existing manifest/discovery system
- Existing tmux integration
- Existing Claude history parsing

---

## 7. Risks & Mitigations

### Risk 1: Migration Breaks Existing Sessions
**Impact**: HIGH
**Probability**: LOW
**Mitigation**:
- Automatic backup before migration
- Rollback on failure
- Extensive testing with real v1 manifests

### Risk 2: Lock Timeout Too Short/Long
**Impact**: MEDIUM
**Probability**: MEDIUM
**Mitigation**:
- Make timeout configurable (constant)
- Document recommended value
- Monitor lock conflicts in production

### Risk 3: Backup Disk Usage
**Impact**: MEDIUM
**Probability**: MEDIUM
**Mitigation**:
- Retention policy (max 10 backups)
- Auto-cleanup of old backups
- User can manually clean if needed

### Risk 4: Performance with Many Sessions
**Impact**: MEDIUM
**Probability**: LOW
**Mitigation**:
- Batch status checking
- Lazy loading of manifests
- Test with 100+ sessions

---

## 8. Success Metrics

### Development Metrics
- [ ] All 11 deliverables implemented
- [ ] All 90+ acceptance criteria met
- [ ] All 14 test scenarios pass
- [ ] Code coverage > 80%

### Performance Metrics
- [ ] Resume auto-recreation < 3s (target met)
- [ ] List 50 sessions < 1s (target met)
- [ ] Migration overhead < 100ms (target met)
- [ ] Backup 200 messages < 5s (target met)

### Quality Metrics
- [ ] Zero critical bugs found in testing
- [ ] Zero data loss incidents
- [ ] All edge cases handled gracefully
- [ ] Error messages are clear and actionable

### User Metrics (Post-Deployment)
- Migration success rate > 99%
- User satisfaction with auto-recreation
- Reduction in "session lost" support tickets

---

## 9. Definition of Done

Phase 3.5 is **DONE** when:

1. ✅ All functional requirements (FR-1 through FR-10) implemented
2. ✅ All non-functional requirements (NFR-1 through NFR-4) met
3. ✅ All test scenarios (TS-1 through TS-14) pass
4. ✅ All acceptance criteria checked off
5. ✅ Code coverage > 80% for critical paths
6. ✅ Documentation complete (godoc, migration guide, troubleshooting)
7. ✅ Multi-persona review score ≥ 8.5/10
8. ✅ Deployment plan approved
9. ✅ No known critical or high-severity bugs

---

## Appendix A: Requirements Traceability

| Requirement | Implementation | Test | Status |
|-------------|----------------|------|--------|
| FR-1.1 | manifest.go | manifest_test.go | Pending |
| FR-1.2 | validate.go | validate_test.go | Pending |
| FR-1.3 | manifest.go | manifest_test.go | Pending |
| FR-2.1 | migrate.go | migrate_test.go | Pending |
| FR-2.2 | migrate.go | migrate_test.go | Pending |
| FR-2.3 | migrate.go | migrate_rollback_test.go | Pending |
| FR-2.4 | migrate.go | migrate_test.go | Pending |
| FR-2.5 | migrate.go | migrate_test.go | Pending |
| FR-2.6 | migrate.go | migrate_test.go | Pending |
| FR-3.1 | validate.go | validate_test.go | Pending |
| FR-3.2 | validate.go | validate_test.go | Pending |
| FR-4.1 | lock.go | lock_test.go | Pending |
| FR-4.2 | lock.go | lock_test.go | Pending |
| FR-4.3 | lock.go | lock_stale_test.go | Pending |
| FR-4.4 | lock.go | lock_test.go | Pending |
| FR-5.1 | resume.go | resume_test.go | Pending |
| FR-5.2 | resume.go | resume_integration_test.go | Pending |
| FR-5.3 | resume.go | resume_test.go | Pending |
| FR-5.4 | resume.go | resume_rollback_test.go | Pending |
| FR-5.5 | resume.go | resume_test.go | Pending |
| FR-5.6 | resume.go | resume_test.go | Pending |
| FR-6.1 | backup.go | backup_test.go | Pending |
| FR-6.2 | backup.go | backup_test.go | Pending |
| FR-6.3 | backup.go | backup_test.go | Pending |
| FR-6.4 | backup.go | backup_retention_test.go | Pending |
| FR-6.5 | backup.go | backup_test.go | Pending |
| FR-6.6 | backup.go | backup_test.go | Pending |
| FR-7.1 | doctor.go | doctor_test.go | Pending |
| FR-7.2 | doctor.go | doctor_test.go | Pending |
| FR-7.3 | doctor.go | doctor_test.go | Pending |
| FR-7.4 | doctor.go | doctor_test.go | Pending |
| FR-8.1 | status.go | status_test.go | Pending |
| FR-8.2 | status.go | status_batch_test.go | Pending |
| FR-9.1 | config.go | config_test.go | Pending |
| FR-9.2 | config.go | config_test.go | Pending |
| FR-10.1 | fileutil.go | fileutil_test.go | Pending |
| FR-10.2 | fileutil.go | fileutil_test.go | Pending |
| FR-10.3 | fileutil.go | fileutil_test.go | Pending |

**Total Requirements**: 40
**Implemented**: 0 (pending implementation)
**Tested**: 0 (pending testing)

---

**Status**: Ready for Multi-Persona Review
**Version**: 1.0
**Last Updated**: December 7, 2025
