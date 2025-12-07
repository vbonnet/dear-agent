# D4: Requirements Specification - Session Persistence (v2)

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Round 2
**Version**: 2.0
**Prerequisites**:
- D1 Discovery ✅ Complete
- D2 Architecture ✅ Approved (8.8/10)
- D3 Implementation ✅ Approved (9.0/10)
- D4 Round 1 ❌ Revision needed (7.7/10)

---

## Executive Summary

This document specifies the complete requirements for Phase 3.5 (Session Persistence Core). It defines functional requirements, non-functional requirements, operational requirements, documentation requirements, acceptance criteria, and test scenarios.

**Scope**: Phase 3.5 only (11 deliverables)
**Out of Scope**: Phase 4 features (context management, archive commands)

**Changes from v1**:
- Added Operational Requirements (OR) section
- Added Documentation Requirements (DR) section
- Added Glossary
- Added 6 negative test scenarios
- Clarified technical ambiguities
- Added granular priorities (P0/P1/P2)
- Added user stories

---

## Table of Contents

1. [Glossary](#glossary)
2. [User Stories](#user-stories)
3. [Functional Requirements](#functional-requirements)
4. [Non-Functional Requirements](#non-functional-requirements)
5. [Operational Requirements](#operational-requirements)
6. [Documentation Requirements](#documentation-requirements)
7. [Test Scenarios](#test-scenarios)
8. [Acceptance Criteria](#acceptance-criteria)
9. [Out of Scope](#out-of-scope)
10. [Dependencies](#dependencies)
11. [Risks & Mitigations](#risks--mitigations)
12. [Success Metrics](#success-metrics)
13. [Definition of Done](#definition-of-done)
14. [Requirements Traceability](#requirements-traceability)

---

## Glossary

**Atomic Write**: A write operation that completes fully or not at all, preventing partial or corrupted file states. Implemented using temp file + rename pattern.

**Character Encoding**: UTF-8 encoding is used for all text fields. Field length limits are measured in UTF-8 characters, not bytes.

**Lifecycle**: Persistent field in manifest indicating session state. Only stores "archived"; empty string means active/stopped (computed from tmux state).

**Lock File**: File created during exclusive operations containing PID (line 1) and RFC3339 timestamp (line 2).

**Manifest**: YAML file (`manifest.yaml`) tracking session metadata including worktree, Claude UUID, tmux session name, and context.

**Migration**: Automatic conversion of v1 manifest schema to v2 schema with backup and rollback support.

**One-time Notice**: User notification shown once per user (tracked via `~/.csm/.migration-notice-shown`).

**RFC3339 Timestamp**: ISO 8601 datetime format with timezone (e.g., `2025-12-07T14:30:00-08:00`).

**Stale Lock**: Lock file older than 60 seconds, assumed to be from crashed process, automatically removed.

**Status**: Computed (not stored) session state: "active" (tmux running), "stopped" (tmux missing), or "archived" (lifecycle="archived").

**Symlink**: Symbolic link. On Windows systems without symlink support, backup "latest" feature is disabled with warning.

**Worktree**: Directory where session work is performed, tracked in manifest's `worktree.path` field.

---

## User Stories

### US-1: Session Survival
**As a** developer
**I want** my Claude sessions to survive computer reboots
**So that** I don't lose my workflow context when my machine restarts

### US-2: Seamless Resume
**As a** developer
**I want** CSM to automatically recreate stopped sessions when I resume them
**So that** I don't have to manually rebuild my tmux environment

### US-3: Safe Migration
**As a** developer
**I want** automatic schema migration with backups
**So that** I can upgrade CSM without risking data loss

### US-4: Session Backup
**As a** developer
**I want** to backup my session conversations
**So that** I can preserve important discussions for later reference

### US-5: Health Monitoring
**As a** developer
**I want** to check the health of my sessions
**So that** I can identify and fix issues before they cause problems

### US-6: Context Tracking
**As a** developer
**I want** to tag and annotate my sessions with purpose and notes
**So that** I can remember why each session exists months later

---

## Functional Requirements

### FR-1: Manifest Schema Version 2

**Priority**: P0 (Must Have)
**Dependency**: None
**User Story**: US-6

#### FR-1.1: Schema Structure
**Requirement**: Manifest MUST support schema version 2.0 with the following structure:

```yaml
schema_version: "2.0"
session_id: string
lifecycle: string  # "" or "archived"
created_at: timestamp  # RFC3339 format
last_activity: timestamp  # RFC3339 format
context:
  purpose: string  # max 256 UTF-8 characters
  tags: [string]   # max 10 tags, each max 32 UTF-8 characters
  notes: string    # max 1024 UTF-8 characters
worktree:
  path: string
claude:
  session_id: string
  session_env_path: string
  file_history_path: string
  started_at: timestamp  # RFC3339 format
  last_activity: timestamp  # RFC3339 format
tmux:
  session_name: string
  window_name: string
  created_at: timestamp  # RFC3339 format
```

**Acceptance Criteria**:
- [ ] Manifest struct includes all v2 fields
- [ ] YAML serialization/deserialization works correctly
- [ ] Timestamps use RFC3339 format
- [ ] Old v1 fields are removed (no backward references)

#### FR-1.2: Context Field Validation
**Requirement**: Context fields MUST be validated on write with UTF-8 character counting:
- Purpose: max 256 UTF-8 characters
- Tags: max 10 tags, each max 32 UTF-8 characters, no whitespace
- Notes: max 1024 UTF-8 characters

**Boundary Conditions**:
- Exactly 256 characters in purpose: PASS
- 257 characters in purpose: FAIL
- Tag with exactly 32 characters: PASS
- Tag with 33 characters: FAIL
- Exactly 10 tags: PASS
- 11 tags: FAIL

**Acceptance Criteria**:
- [ ] Writing manifest with purpose > 256 chars returns validation error
- [ ] Writing manifest with > 10 tags returns validation error
- [ ] Writing manifest with tag > 32 chars returns validation error
- [ ] Writing manifest with tag containing whitespace returns validation error
- [ ] Writing manifest with notes > 1024 chars returns validation error
- [ ] Valid context at exact limits passes validation
- [ ] UTF-8 multibyte characters counted correctly (emoji, etc.)

#### FR-1.3: Lifecycle Field
**Requirement**: Lifecycle field MUST only store "archived" state. Empty string means active/stopped (computed).

**Acceptance Criteria**:
- [ ] Setting lifecycle to "" is valid
- [ ] Setting lifecycle to "archived" is valid
- [ ] Setting lifecycle to any other value returns validation error
- [ ] Status is computed from lifecycle + tmux state, never stored

---

### FR-2: Schema Migration (v1 → v2)

**Priority**: P0 (Must Have)
**Dependency**: FR-1
**User Story**: US-3

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

**Log Format**:
```
[2025-12-07T14:30:00-08:00] SUCCESS: /home/user/sessions/session-claude-1/manifest.yaml
[2025-12-07T14:31:00-08:00] FAILED: /home/user/sessions/session-claude-2/manifest.yaml - invalid YAML syntax
```

**Acceptance Criteria**:
- [ ] Successful migration logged with RFC3339 timestamp and path
- [ ] Failed migration logged with error details
- [ ] Log file created if doesn't exist
- [ ] Log entries are append-only

#### FR-2.6: Migration User Messaging
**Requirement**: In interactive terminals, migration MUST show progress messages. In non-interactive contexts (pipes, CI/CD), messages MUST be suppressed.

**Definition**: One-time notice is shown once per user, tracked by `~/.csm/.migration-notice-shown` file.

**Acceptance Criteria**:
- [ ] In terminal: shows "📝 Migrating..." and "✅ Success"
- [ ] In pipe: no messages to stdout
- [ ] One-time notice shown on first migration per user
- [ ] Notice file created: `~/.csm/.migration-notice-shown`

---

### FR-3: Context Validation

**Priority**: P1 (Should Have)
**Dependency**: FR-1
**User Story**: US-6

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

**Priority**: P0 (Must Have)
**Dependency**: None
**User Story**: US-2

**Technical Specification**:
- Lock file format: Line 1: PID (e.g., `12345`), Line 2: RFC3339 timestamp
- Lock file created with O_EXCL flag
- NFS limitation: File locking not guaranteed on NFS mounts (see OR-6)

#### FR-4.1: Lock Acquisition
**Requirement**: Resume and other write operations MUST acquire exclusive lock on manifest.

**Acceptance Criteria**:
- [ ] First process acquires lock successfully
- [ ] Second concurrent process gets lock error
- [ ] Lock file created: `manifest.yaml.lock`
- [ ] Lock file contains PID (line 1) and RFC3339 timestamp (line 2)

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

**Priority**: P0 (Must Have)
**Dependency**: FR-1, FR-4
**User Story**: US-2

#### FR-5.1: Status Detection
**Requirement**: Resume MUST detect session status (active/stopped/archived) before attempting recreation.

**Acceptance Criteria**:
- [ ] Active session (tmux exists): skips recreation, attaches directly
- [ ] Stopped session (tmux missing): triggers auto-recreation
- [ ] Archived session: prompts user to unarchive

#### FR-5.2: Tmux Auto-Recreation
**Requirement**: For stopped sessions, resume MUST recreate tmux session automatically.

**Exact Commands**:
1. Check: `tmux has-session -t <name>` (exit code 0 = exists)
2. Create: `tmux new-session -d -s <name> -c <worktree>`
3. Send: `tmux send-keys -t <name> 'claude --resume <uuid>' C-m`
4. Attach: `tmux attach-session -t <name>`

**Acceptance Criteria**:
- [ ] Stopped session detected correctly
- [ ] Tmux session created with correct name
- [ ] Tmux session started in correct worktree directory
- [ ] Claude resumed with correct UUID
- [ ] User attached to tmux session

#### FR-5.3: Worktree Validation
**Requirement**: Before recreating tmux, resume MUST verify worktree directory exists (resolve symlinks first).

**Acceptance Criteria**:
- [ ] Worktree exists: recreation proceeds
- [ ] Worktree is symlink: resolved to target, checked
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
- [ ] last_activity timestamp updated to now() in RFC3339 format
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

**Priority**: P1 (Should Have)
**Dependency**: FR-1
**User Story**: US-4

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

**Platform Support**: Symlinks on Windows require developer mode or admin privileges. If symlink creation fails, show warning but continue.

**Acceptance Criteria**:
- [ ] After backup, `backups/latest` symlink exists (if supported)
- [ ] Symlink points to most recent backup directory (relative path)
- [ ] Subsequent backup updates symlink
- [ ] On Windows without symlink support: warning shown, backup still succeeds

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
- [ ] Shows latest symlink location (if created)

---

### FR-7: Doctor Command

**Priority**: P1 (Should Have)
**Dependency**: FR-1, FR-2, FR-4
**User Story**: US-5

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

**Priority**: P0 (Must Have)
**Dependency**: FR-1
**User Story**: US-1

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

**Priority**: P2 (Nice to Have)
**Dependency**: None
**User Story**: US-1

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

**Priority**: P1 (Should Have)
**Dependency**: None
**User Story**: US-3

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
- [ ] Symbolic links copied correctly

---

## Non-Functional Requirements

### NFR-1: Performance

**Priority**: P0 (Must Have)

#### NFR-1.1: Resume Auto-Recreation
**Requirement**: Resume with auto-recreation MUST complete in < 3 seconds.

**Measurement Environment**: Standard development machine (4-core CPU, SSD)
**Measurement**: Time from command execution to tmux attach.

**Acceptance Criteria**:
- [ ] 10 test runs average < 3 seconds
- [ ] No run exceeds 5 seconds
- [ ] Test on SSD storage

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

**Priority**: P0 (Must Have)

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

**Priority**: P1 (Should Have)

#### NFR-3.1: Error Messages
**Requirement**: Error messages MUST be clear, concise, and actionable.

**Examples**:
- ✅ "worktree does not exist: ~/deleted-project (try: csm archive claude-1)"
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

**Priority**: P1 (Should Have)

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
**Requirement**: Code coverage MUST exceed 80% for critical paths, 60% overall.

**Critical paths**:
- Manifest load/write/validate
- Migration logic
- Lock acquisition/release
- Resume auto-recreation

**Acceptance Criteria**:
- [ ] Run `go test -cover`
- [ ] Coverage > 80% for critical packages
- [ ] Coverage > 60% overall
- [ ] All edge cases have tests

#### NFR-4.3: Documentation
**Requirement**: All public functions MUST have godoc comments.

**Acceptance Criteria**:
- [ ] Run `go doc` on all packages
- [ ] All exported functions documented
- [ ] Comments explain purpose, parameters, return values

---

## Operational Requirements

### OR-1: Deployment Strategy

**Priority**: P0 (Must Have)

**Requirement**: Phase 3.5 deployment MUST follow staged rollout to minimize risk.

**Deployment Stages**:
1. **Stage 1 - Internal Testing** (1 week)
   - Deploy to development environments
   - Test with real sessions
   - Monitor migration.log for failures

2. **Stage 2 - Limited Release** (1 week)
   - Deploy to early adopters (opt-in)
   - Monitor for migration failures
   - Collect feedback

3. **Stage 3 - General Availability**
   - Deploy to all users
   - Monitor for issues
   - Have rollback plan ready

**Acceptance Criteria**:
- [ ] Deployment plan documented
- [ ] Rollback procedure tested
- [ ] Each stage has clear success criteria
- [ ] Communication plan for each stage

### OR-2: Monitoring & Metrics

**Priority**: P1 (Should Have)

**Requirement**: CSM MUST provide observability into key operations.

**Metrics to Track**:
1. Migration success/failure rate (from migration.log)
2. Lock timeout frequency (how often stale locks detected)
3. Auto-recreation success rate
4. Backup success/failure rate
5. Average resume time

**Implementation**: Log-based monitoring (parse migration.log and command output)

**Acceptance Criteria**:
- [ ] Migration logging captures all attempts
- [ ] Success/failure events clearly distinguishable
- [ ] Timestamps enable time-series analysis
- [ ] Log format documented for parsing

### OR-3: Log Rotation

**Priority**: P1 (Should Have)

**Requirement**: `~/.csm/logs/migration.log` MUST implement rotation to prevent unbounded growth.

**Rotation Policy**:
- Rotate when log file exceeds 10MB
- Keep last 5 rotated files (migration.log.1 through migration.log.5)
- Oldest file deleted when limit reached

**Acceptance Criteria**:
- [ ] Log rotation implemented
- [ ] Rotation triggered at 10MB threshold
- [ ] Only 5 old log files retained
- [ ] Active writes not interrupted during rotation

### OR-4: Alerting Thresholds

**Priority**: P2 (Nice to Have)

**Requirement**: Define thresholds for operational alerts.

**Recommended Thresholds**:
- Migration failure rate > 5%: Investigate
- Lock timeouts > 10 per day: Investigate
- Backup failures > 3 consecutive: Alert

**Implementation**: External monitoring system can parse logs

**Acceptance Criteria**:
- [ ] Thresholds documented
- [ ] Log format supports threshold detection
- [ ] Recommendations provided for remediation

### OR-5: Capacity Planning

**Priority**: P2 (Nice to Have)

**Requirement**: Document expected resource usage.

**Expected Usage**:
- Sessions per user: 5-20 typical, 100 maximum tested
- Disk space per session:
  - Manifest: ~1KB
  - Backups: ~10MB per backup × 10 backups = 100MB
  - Total per session: ~100MB
- Memory usage: < 50MB for CSM process
- CPU usage: Negligible except during backup

**Acceptance Criteria**:
- [ ] Resource usage documented
- [ ] Tested with 100 sessions
- [ ] Disk space warnings if backups exceed 1GB per session

### OR-6: Platform Limitations

**Priority**: P0 (Must Have)

**Requirement**: Document known platform limitations.

**Limitations**:
1. **NFS File Systems**: File locking not guaranteed on NFS. Do not use CSM with sessions directory on NFS mount.
2. **Windows Symlinks**: Require developer mode or admin privileges. Backup "latest" symlink disabled if unavailable.
3. **Old tmux Versions**: Requires tmux 2.0+ for proper session handling.

**Acceptance Criteria**:
- [ ] Limitations documented in README
- [ ] NFS detection warns user if possible
- [ ] Symlink failure handled gracefully on Windows
- [ ] Minimum tmux version checked on startup

---

## Documentation Requirements

### DR-1: Command Help Text

**Priority**: P0 (Must Have)

**Requirement**: All commands MUST have comprehensive --help text.

**Help Text Must Include**:
- Command description
- Usage examples
- Flag descriptions
- Exit codes
- Common errors and solutions

**Example**:
```
csm backup - Create timestamped backup of session

Usage:
  csm backup <identifier> [flags]

Flags:
  --format string        Output format: jsonl or markdown (default "jsonl")
  --include-files        Include file snapshots in backup

Examples:
  csm backup claude-1
  csm backup claude-1 --format markdown --include-files

Exit Codes:
  0    Success
  1    Session not found
  2    Backup failed
```

**Acceptance Criteria**:
- [ ] All commands have --help flag
- [ ] Help text follows consistent format
- [ ] Examples cover common use cases
- [ ] Exit codes documented

### DR-2: Migration Guide

**Priority**: P0 (Must Have)

**Requirement**: Comprehensive migration guide MUST be written.

**Guide Must Cover**:
1. What happens during migration (v1 → v2)
2. How to verify migration success
3. What to do if migration fails
4. How to manually rollback if needed
5. When it's safe to delete .v1.bak files

**Location**: `docs/MIGRATION_GUIDE.md`

**Acceptance Criteria**:
- [ ] Migration guide created
- [ ] Step-by-step instructions provided
- [ ] Troubleshooting section included
- [ ] Linked from main README

### DR-3: Glossary

**Priority**: P1 (Should Have)

**Requirement**: Glossary of terms MUST be maintained (see Glossary section above).

**Terms to Define**:
- Manifest
- Lifecycle vs Status
- Worktree
- Lock file
- Stale lock
- Atomic write
- Schema migration

**Acceptance Criteria**:
- [ ] Glossary section in documentation
- [ ] All technical terms defined
- [ ] Plain language explanations provided

### DR-4: Error Message Style Guide

**Priority**: P1 (Should Have)

**Requirement**: Error message style guide MUST be defined.

**Style Guidelines**:
1. Start with context (what failed)
2. Explain why it failed
3. Suggest remediation
4. Use plain language
5. Include relevant paths/values

**Template**:
```
Error: <what failed>: <details> (<suggestion>)
```

**Examples**:
- ✅ "worktree does not exist: ~/project (try: csm archive claude-1)"
- ✅ "session locked by process 12345 (try: kill 12345 or wait)"
- ❌ "ENOENT"
- ❌ "operation failed"

**Acceptance Criteria**:
- [ ] Style guide documented
- [ ] Examples provided
- [ ] All error messages follow template
- [ ] No raw error strings from libraries

---

## Test Scenarios

### TS-1: Migration Happy Path

**Priority**: P0
**Given**: A valid v1 manifest exists
**When**: User runs `csm list` (or any command that loads manifest)
**Then**:
- [ ] Migration backup created (.v1.bak)
- [ ] Migration logged to migration.log
- [ ] V2 manifest written successfully
- [ ] In terminal: migration message shown
- [ ] Subsequent loads use v2 (no re-migration)

### TS-2: Migration Failure with Rollback

**Priority**: P0
**Given**: A valid v1 manifest exists
**When**: Migration write fails (read-only directory)
**Then**:
- [ ] Backup created before failure
- [ ] Write failure detected
- [ ] Original v1 manifest restored from backup
- [ ] Error message indicates rollback
- [ ] Migration failure logged to migration.log

### TS-3: Resume Active Session

**Priority**: P0
**Given**: Session manifest exists with active tmux session
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Status detected as "active"
- [ ] No tmux recreation attempted
- [ ] User attached to existing tmux session
- [ ] Manifest last_activity updated

### TS-4: Resume Stopped Session (Auto-Recreation)

**Priority**: P0
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

**Priority**: P0
**Given**: Session manifest exists, worktree directory deleted
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Worktree validation fails
- [ ] Clear error message shown
- [ ] Suggestions provided: update worktree, archive, or force
- [ ] No tmux session created
- [ ] No partial state left

### TS-6: Resume Archived Session

**Priority**: P1
**Given**: Session manifest with lifecycle="archived"
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Archived status detected
- [ ] User prompted: "Unarchive and resume? (y/n)"
- [ ] If yes: lifecycle set to "", resume proceeds
- [ ] If no: command aborts with clear message

### TS-7: Concurrent Resume (Lock Conflict)

**Priority**: P0
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

**Priority**: P0
**Given**: Stale lock file exists (> 60 seconds old)
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Stale lock detected
- [ ] Stale lock removed automatically
- [ ] New lock acquired
- [ ] Resume proceeds normally

### TS-9: Partial Failure Rollback

**Priority**: P0
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

**Priority**: P1
**Given**: Session with 100 messages in history
**When**: User runs `csm backup claude-1`
**Then**:
- [ ] Backup directory created with timestamp
- [ ] Manifest copied to session-info.yaml
- [ ] 100 messages extracted to conversation.jsonl
- [ ] "Latest" symlink created/updated (if supported)
- [ ] Success message shown with path

### TS-11: Backup Retention

**Priority**: P1
**Given**: Session has 10 existing backups
**When**: User runs `csm backup claude-1` (creates 11th)
**Then**:
- [ ] New backup created
- [ ] Oldest backup deleted
- [ ] Only 10 backups remain
- [ ] "Latest" symlink points to newest

### TS-12: Doctor Healthy System

**Priority**: P1
**Given**: All sessions are healthy
**When**: User runs `csm doctor`
**Then**:
- [ ] All checks pass (✓ shown)
- [ ] Summary: "0 warnings, 0 errors"
- [ ] Message: "✓ CSM is healthy"
- [ ] Exit code 0

### TS-13: Doctor with Issues

**Priority**: P1
**Given**: 2 stale locks, 1 missing worktree
**When**: User runs `csm doctor`
**Then**:
- [ ] Stale locks detected (⚠ 2 stale lock files)
- [ ] Missing worktree detected (⚠ 1 session has missing worktree)
- [ ] Summary: "2 warnings, 0 errors"
- [ ] Exit code non-zero

### TS-14: Doctor Auto-Fix

**Priority**: P1
**Given**: 2 stale lock files exist
**When**: User runs `csm doctor --fix`
**Then**:
- [ ] Stale locks detected
- [ ] Stale locks removed
- [ ] Message: "✓ Cleaned 2 stale lock files"
- [ ] Summary: "0 warnings, 0 errors"

### TS-15: Corrupted History File

**Priority**: P1
**Given**: history.jsonl contains malformed JSON
**When**: User runs `csm backup claude-1`
**Then**:
- [ ] Parser detects malformed JSON
- [ ] Clear error message: "history file is corrupted (invalid JSON at line X)"
- [ ] Backup fails gracefully
- [ ] No partial backup left

### TS-16: No Write Permissions

**Priority**: P1
**Given**: Sessions directory is read-only
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Write permission check fails
- [ ] Clear error: "cannot write to sessions directory (permission denied)"
- [ ] Suggests: "check permissions on ~/sessions"
- [ ] No partial files created

### TS-17: Tmux Not Installed

**Priority**: P1
**Given**: tmux binary not in PATH
**When**: User runs `csm resume claude-1`
**Then**:
- [ ] Tmux check fails
- [ ] Clear error: "tmux not found in PATH"
- [ ] Suggests: "install tmux: sudo apt install tmux"
- [ ] No tmux commands attempted

### TS-18: Disk Full During Backup

**Priority**: P1
**Given**: Backup in progress, disk fills up
**When**: Write fails mid-backup
**Then**:
- [ ] Write error detected
- [ ] Partial backup directory removed
- [ ] Clear error: "disk full (unable to write backup)"
- [ ] Suggests: "free up space and retry"

### TS-19: Boundary Condition - Exact Limits

**Priority**: P1
**Given**: Context with purpose exactly 256 characters
**When**: Validation runs
**Then**:
- [ ] Passes validation (256 is allowed)
- [ ] 257 characters fails validation
- [ ] Error message shows: "purpose too long (257 chars, max 256)"

### TS-20: Concurrent Migration and Resume

**Priority**: P0
**Given**: v1 manifest exists
**When**: Two processes try to load simultaneously
**Then**:
- [ ] First process acquires migration lock
- [ ] Second process waits for migration to complete
- [ ] Second process reads migrated v2 manifest
- [ ] No double migration occurs
- [ ] Only one .v1.bak file created

### TS-21: Rollback to Previous CSM Version

**Priority**: P0
**Given**: CSM upgraded to Phase 3.5, v2 manifests exist
**When**: User downgrades to pre-Phase 3.5 CSM
**Then**:
- [ ] Old CSM fails gracefully on v2 manifests
- [ ] Error message: "unsupported schema version 2.0"
- [ ] User can restore from .v1.bak files manually
- [ ] Rollback procedure documented

---

## Acceptance Criteria Summary

### Phase 3.5 is complete when:

#### Must Have (All Required) - P0
- [ ] All FR with P0 priority implemented (FR-1, FR-2, FR-4, FR-5, FR-8)
- [ ] All NFR-1, NFR-2 met
- [ ] All OR-1, OR-6 met
- [ ] All DR-1, DR-2 met
- [ ] All P0 test scenarios pass (TS-1, TS-2, TS-3, TS-4, TS-5, TS-7, TS-8, TS-9, TS-20, TS-21)
- [ ] Code coverage > 80% for critical paths
- [ ] No known critical bugs

#### Should Have (Strongly Recommended) - P1
- [ ] FR-3, FR-6, FR-7, FR-10 implemented
- [ ] NFR-3, NFR-4 met
- [ ] OR-2, OR-3 met
- [ ] DR-3, DR-4 met
- [ ] All P1 test scenarios pass
- [ ] Migration guide written
- [ ] Troubleshooting FAQ created

#### Nice to Have (Optional) - P2
- [ ] FR-9 implemented
- [ ] OR-4, OR-5 met
- [ ] Benchmark tests for performance validation
- [ ] Stress tests with 100+ sessions
- [ ] Integration test simulating reboot

---

## Out of Scope (Phase 4)

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

## Dependencies

### External Dependencies
- Go 1.19+ (for development)
- tmux 2.0+ (runtime requirement)
- Claude CLI (runtime requirement)
- yaml.v3 library (for YAML parsing)

### Internal Dependencies
- Existing manifest/discovery system
- Existing tmux integration
- Existing Claude history parsing

---

## Risks & Mitigations

### Risk 1: Migration Breaks Existing Sessions
**Impact**: HIGH
**Probability**: LOW
**Mitigation**:
- Automatic backup before migration
- Rollback on failure
- Extensive testing with real v1 manifests
- Staged rollout

### Risk 2: Lock Timeout Too Short/Long
**Impact**: MEDIUM
**Probability**: MEDIUM
**Mitigation**:
- Make timeout configurable (constant)
- Document recommended value (60s)
- Monitor lock conflicts in production
- Doctor command detects/cleans stale locks

### Risk 3: Backup Disk Usage
**Impact**: MEDIUM
**Probability**: MEDIUM
**Mitigation**:
- Retention policy (max 10 backups)
- Auto-cleanup of old backups
- User can manually clean if needed
- Capacity planning documented

### Risk 4: Performance with Many Sessions
**Impact**: MEDIUM
**Probability**: LOW
**Mitigation**:
- Batch status checking
- Lazy loading of manifests
- Test with 100+ sessions
- Performance benchmarks

### Risk 5: NFS File System Usage
**Impact**: HIGH
**Probability**: LOW
**Mitigation**:
- Document NFS limitation clearly
- Detect NFS and warn user if possible
- Recommend local filesystem only

---

## Success Metrics

### Development Metrics
- [ ] All 11 deliverables implemented
- [ ] All 100+ acceptance criteria met
- [ ] All 21 test scenarios pass
- [ ] Code coverage > 80% for critical paths, > 60% overall

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
- < 1% rollback rate

---

## Definition of Done

Phase 3.5 is **DONE** when:

1. ✅ All functional requirements (FR-1 through FR-10) implemented
2. ✅ All non-functional requirements (NFR-1 through NFR-4) met
3. ✅ All operational requirements (OR-1 through OR-6) met
4. ✅ All documentation requirements (DR-1 through DR-4) met
5. ✅ All test scenarios (TS-1 through TS-21) pass
6. ✅ All acceptance criteria checked off
7. ✅ Code coverage > 80% for critical paths, > 60% overall
8. ✅ Documentation complete (godoc, migration guide, troubleshooting)
9. ✅ Multi-persona review score ≥ 8.5/10
10. ✅ Deployment plan approved
11. ✅ No known critical or high-severity bugs

---

## Requirements Traceability

| ID | Requirement | Priority | Implementation | Test | User Story |
|----|-------------|----------|----------------|------|------------|
| FR-1.1 | Schema Structure | P0 | manifest.go | manifest_test.go | US-6 |
| FR-1.2 | Context Validation | P0 | validate.go | validate_test.go | US-6 |
| FR-1.3 | Lifecycle Field | P0 | manifest.go | manifest_test.go | US-6 |
| FR-2.1 | Auto Migration | P0 | migrate.go | migrate_test.go | US-3 |
| FR-2.2 | Migration Backup | P0 | migrate.go | migrate_test.go | US-3 |
| FR-2.3 | Migration Rollback | P0 | migrate.go | migrate_rollback_test.go | US-3 |
| FR-2.4 | Migration Validation | P0 | migrate.go | migrate_test.go | US-3 |
| FR-2.5 | Migration Logging | P0 | migrate.go | migrate_test.go | US-3 |
| FR-2.6 | Migration Messaging | P0 | migrate.go | migrate_test.go | US-3 |
| FR-3.1 | Validation on Write | P1 | validate.go | validate_test.go | US-6 |
| FR-3.2 | Validation Errors | P1 | validate.go | validate_test.go | US-6 |
| FR-4.1 | Lock Acquisition | P0 | lock.go | lock_test.go | US-2 |
| FR-4.2 | Lock Release | P0 | lock.go | lock_test.go | US-2 |
| FR-4.3 | Stale Lock Detection | P0 | lock.go | lock_stale_test.go | US-2 |
| FR-4.4 | Lock Error Messages | P0 | lock.go | lock_test.go | US-2 |
| FR-5.1 | Status Detection | P0 | resume.go | resume_test.go | US-2 |
| FR-5.2 | Tmux Auto-Recreation | P0 | resume.go | resume_integration_test.go | US-2 |
| FR-5.3 | Worktree Validation | P0 | resume.go | resume_test.go | US-2 |
| FR-5.4 | Partial Failure Rollback | P0 | resume.go | resume_rollback_test.go | US-2 |
| FR-5.5 | Manifest Update | P0 | resume.go | resume_test.go | US-2 |
| FR-5.6 | Unarchive Flow | P1 | resume.go | resume_test.go | US-2 |
| FR-6.1 | Backup Creation | P1 | backup.go | backup_test.go | US-4 |
| FR-6.2 | Backup Formats | P1 | backup.go | backup_test.go | US-4 |
| FR-6.3 | File Snapshots | P2 | backup.go | backup_test.go | US-4 |
| FR-6.4 | Backup Retention | P1 | backup.go | backup_retention_test.go | US-4 |
| FR-6.5 | Latest Symlink | P1 | backup.go | backup_test.go | US-4 |
| FR-6.6 | Backup Output | P1 | backup.go | backup_test.go | US-4 |
| FR-7.1 | Health Checks | P1 | doctor.go | doctor_test.go | US-5 |
| FR-7.2 | Stale Lock Cleanup | P1 | doctor.go | doctor_test.go | US-5 |
| FR-7.3 | Output Modes | P1 | doctor.go | doctor_test.go | US-5 |
| FR-7.4 | Specific Session Check | P1 | doctor.go | doctor_test.go | US-5 |
| FR-8.1 | Status Determination | P0 | status.go | status_test.go | US-1 |
| FR-8.2 | Batch Status Computation | P0 | status.go | status_batch_test.go | US-1 |
| FR-9.1 | Config Hierarchy | P2 | config.go | config_test.go | US-1 |
| FR-9.2 | Path Expansion | P2 | config.go | config_test.go | US-1 |
| FR-10.1 | CopyFile | P1 | fileutil.go | fileutil_test.go | US-3 |
| FR-10.2 | WriteAtomic | P1 | fileutil.go | fileutil_test.go | US-3 |
| FR-10.3 | CopyDirectory | P1 | fileutil.go | fileutil_test.go | US-3 |

**Total Requirements**: 40 (10 P0, 20 P1, 3 P2)
**Implemented**: 0 (pending implementation)
**Tested**: 0 (pending testing)

---

**Status**: Ready for Round 2 Review
**Version**: 2.0
**Last Updated**: December 7, 2025
