# S2 Sprint Plan Review - Round 1

**Date**: December 7, 2025
**Document**: S2-SPRINT-PLAN.md
**Review Type**: Multi-Persona Review

---

## Reviewer 1: Senior Go Developer

**Perspective**: Implementation feasibility, Go best practices, code organization

### Assessment ✅

**Sprint Scope**:
- ✅ Well-defined: 3 deliverables, all user-facing
- ✅ Builds on S1 foundation (locking, migration, fileutil)
- ✅ Logical grouping (status + resume + backup)

**Code Organization**:
- ✅ Clear file structure
- ✅ Separation of concerns (status, resume, backup)
- ✅ Integration with S1 components shown

### Strengths ✅

1. **S1 integration clear**: Shows how to use lock, fileutil, validation
2. **Exact commands specified**: tmux commands all documented
3. **Error handling**: Rollback scenarios defined
4. **Performance targets**: Specific metrics from NFRs

### Concerns ⚠️

1. **Backup timestamp collision handling vague**:
   - "Add microseconds if collision detected"
   - How is collision detected?
   - What if collision happens after check?
   - **Better**: Always use microseconds in timestamp: `2025-12-07_14-30-00-123456`

2. **History.jsonl parsing not specified**:
   - How to filter by UUID?
   - What if entry is malformed JSON?
   - Stream or load entire file?
   - **Add**: Parsing strategy specification

3. **Markdown generation details missing**:
   - How to format code blocks in messages?
   - How to handle long messages?
   - Escape sequences?
   - **Add**: Markdown formatting specification

4. **Resume lock timing unclear**:
   - When is lock acquired?
   - Before or after status check?
   - **Clarify**: Acquire lock before any operation

5. **Backup file permissions not specified**:
   - session-info.yaml permissions?
   - conversation.jsonl permissions?
   - **Add**: All backup files 0600 (consistent with S1)

### Missing Details 🔍

1. **Error messages not specified**:
   - Unlike S1, no exact error text provided
   - What does "worktree missing" error say?
   - What does "Claude failed" error say?
   - **Add**: Error message specifications (like S1)

2. **Progress messages not detailed**:
   - "Show progress for each step"
   - What exactly is shown?
   - **Add**: Exact message text

3. **Backup cleanup error handling**:
   - What if old backup deletion fails?
   - Permissions issue?
   - **Clarify**: Best-effort cleanup, log warning

### Recommendation

**Score**: 8.0/10 - Good plan, needs specification details

**Required additions**:
- Backup timestamp format (always include microseconds)
- History.jsonl parsing strategy
- Error message specifications

**Recommended**:
- Markdown formatting details
- Backup file permissions (0600)

---

## Reviewer 2: Software Architect

**Perspective**: System design, dependencies, integration

### Architecture Assessment ✅

**Layering**:
- ✅ Builds on S1 correctly
- ✅ No circular dependencies
- ✅ Clear separation (status, resume, backup independent)

**Integration**:
- ✅ S1 components used correctly (lock, fileutil, validate)
- ✅ Dependencies explicit

### Strengths ✅

1. **Dependency management**: S1 → S2 clear
2. **Status computation placement**: Correct layer (cmd, not manifest)
3. **Lock integration**: All operations respect locks

### Concerns ⚠️

1. **Backup + Resume concurrency undefined**:
   - User runs `csm backup claude-1` in one terminal
   - User runs `csm resume claude-1` in another terminal
   - Both acquire lock, but which wins?
   - What happens to backup if session resumes mid-backup?
   - **Add**: Concurrent operation test scenario

2. **History.jsonl location hardcoded**:
   - `~/.claude/history.jsonl`
   - What if CLAUDE_HOME set?
   - What if history in different location?
   - **Better**: Use Claude's config or make configurable

3. **Backup atomic creation unclear**:
   - Partial backup if interrupted?
   - **Better**: Create in temp directory, move when complete

4. **Status computation caching**:
   - Batch status calls tmux once
   - But what if list takes 10 seconds?
   - Tmux state could change mid-list
   - **Acceptable**: Document as known limitation

5. **Resume manifest update timing**:
   - "Update manifest last_activity"
   - Before or after attach?
   - What if attach fails?
   - **Clarify**: Update after successful attach

### Missing Architectural Details 🔍

1. **Backup partial failure handling**:
   - Manifest copied but conversation extraction fails
   - Partial backup left behind?
   - **Add**: Cleanup on failure

2. **File-history structure not documented**:
   - What's in `~/.claude/file-history/<uuid>/`?
   - Flat or nested?
   - **Document**: Expected structure

3. **Symlink resolution strategy**:
   - `filepath.EvalSymlinks()` used
   - What if symlink is broken?
   - What if circular symlinks?
   - **Add**: Error handling for symlink edge cases

### Recommendation

**Score**: 8.5/10 - Solid architecture, minor gaps

**Required additions**:
- Concurrent backup + resume scenario
- Backup atomic creation (temp dir)
- Partial failure cleanup

**Recommended**:
- History.jsonl location configuration
- File-history structure documentation

---

## Reviewer 3: QA Engineer

**Perspective**: Testability, edge cases, failure modes

### Test Coverage Assessment ✅

**Unit Tests**:
- ✅ Tests per deliverable
- ✅ Integration tests defined

**Integration Tests**:
- ✅ 10 scenarios covering main workflows
- ✅ Performance tests included

### Strengths ✅

1. **Performance tests**: Specific targets with measurements
2. **Integration tests**: Cover main user workflows
3. **Rollback testing**: Partial failure scenarios

### Testing Gaps ⚠️

1. **No concurrent operation tests**:
   - TS-S2-10 tests batch status
   - But no concurrent backup + resume
   - No concurrent multiple resumes
   - **Add**: Concurrent operation scenarios

2. **No corrupted history.jsonl test**:
   - Mentioned in risk mitigation
   - But no test scenario
   - **Add**: TS with malformed JSON in history

3. **No symlink edge cases**:
   - Broken symlink in worktree
   - Circular symlinks
   - **Add**: Symlink edge case tests

4. **No disk full scenario**:
   - Backup fails mid-write
   - Resume can't update manifest
   - **Add**: Disk full tests

5. **No tmux command failure tests**:
   - What if `tmux new-session` fails?
   - What if `tmux send-keys` fails?
   - **Add**: Tmux command failure scenarios

6. **No backup format edge cases**:
   - Message with 10MB of text
   - Message with binary data
   - Message with special characters
   - **Add**: Large message tests

### Missing Test Scenarios 📝

**TS-S2-11: Concurrent Backup and Resume**
- Terminal 1: `csm backup claude-1` (takes time)
- Terminal 2: `csm resume claude-1` (modifies manifest)
- Verify: Lock prevents conflict

**TS-S2-12: Corrupted History File**
- history.jsonl contains malformed JSON lines
- Run backup
- Verify: Skips bad lines, creates backup with good lines

**TS-S2-13: Broken Symlink in Worktree**
- Worktree is symlink to non-existent directory
- Run resume
- Verify: Clear error, no tmux created

**TS-S2-14: Tmux Command Failure**
- Mock `tmux new-session` to fail
- Run resume
- Verify: Clear error, no partial state

**TS-S2-15: Backup with Large Messages**
- Session with messages >1MB each
- Run backup
- Verify: Completes successfully, all messages in backup

**TS-S2-16: Disk Full During Backup**
- Start backup
- Simulate disk full mid-write
- Verify: Cleanup partial backup, clear error

### Recommendation

**Score**: 7.5/10 - Good coverage, missing edge cases

**Critical additions**:
- Concurrent operation tests
- Corrupted history test
- Tmux command failure tests

**Recommended**:
- Symlink edge cases
- Disk full scenarios
- Large message tests

---

## Reviewer 4: DevOps/SRE

**Perspective**: Operations, deployment, observability

### Operational Assessment ✅

**Deployment**:
- ✅ Builds on S1 (no breaking changes)
- ✅ New commands: backup (additive)
- ✅ Resume enhanced (backward compatible)

**Observability**:
- ✅ Performance targets measurable
- ✅ User messages provide feedback

### Strengths ✅

1. **Performance targets**: Specific, measurable
2. **Backward compatible**: Existing resume still works
3. **Additive changes**: New backup command

### Operational Concerns ⚠️

1. **No post-deployment verification checklist**:
   - S1 had excellent checklist
   - S2 should have similar for auto-recreation and backup
   - **Add**: Verification checklist (like S1)

2. **No monitoring guidance**:
   - How to track backup success rate?
   - How to track auto-recreation success rate?
   - **Add**: Metrics to monitor

3. **Backup storage growth unmonitored**:
   - 10 backups per session
   - 100 sessions = 1000 backup directories
   - What if they're large?
   - **Add**: Disk usage monitoring guidance

4. **No rollback procedure for S2**:
   - S1 had complete rollback guide
   - S2 should have similar
   - What if auto-recreation has bugs?
   - **Add**: Rollback procedure

5. **Resume failure not logged**:
   - Migration logged (S1)
   - But resume failures not logged
   - **Consider**: Log auto-recreation attempts

6. **Backup retention not configurable**:
   - Hardcoded 10
   - Some users may want more/less
   - **Acceptable**: Can change in future, 10 reasonable

### Missing Operational Details 🔍

1. **Deployment order with S1**:
   - S1 must be deployed first
   - S2 depends on S1 components
   - **Document**: Deployment dependencies

2. **Performance degradation detection**:
   - How to detect if resume becomes slow?
   - How to detect if backup becomes slow?
   - **Add**: Performance monitoring guidance

3. **User communication**:
   - How to announce new auto-recreation feature?
   - Release notes requirements?
   - **Add**: Release notes draft

### Recommendation

**Score**: 7.5/10 - Functional but needs operational docs

**Required additions**:
- Post-deployment verification checklist
- Rollback procedure
- Monitoring guidance

**Recommended**:
- Deployment dependencies documented
- Performance monitoring
- Release notes draft

---

## Reviewer 5: End User / Developer

**Perspective**: Daily usage, UX, developer experience

### User Experience Assessment ✅

**User Value**:
- ✅ Auto-recreation solves reboot problem
- ✅ Backup preserves conversations
- ✅ Status always accurate

**Messaging**:
- ⚠️ User messages mentioned but not specified

### Strengths ✅

1. **Core value delivered**: Sessions survive reboots
2. **Backup flexibility**: JSONL and Markdown formats
3. **Clear workflows**: Auto-recreation transparent

### UX Concerns ⚠️

1. **User messages not specified**:
   - Resume: "Session stopped, recreating..."
   - Backup: Progress messages
   - But exact text not shown (unlike S1)
   - **Add**: All user message specifications

2. **Unarchive prompt interaction unclear**:
   - "Unarchive and resume? (y/n)"
   - What if non-interactive (script)?
   - What if user wants to see session first?
   - **Add**: --yes flag for scripts

3. **Backup format not obvious to users**:
   - JSONL default
   - Users may not know what JSONL is
   - **Consider**: Markdown default, or ask on first backup

4. **No progress indication for slow backups**:
   - Large history files may take time
   - User sees nothing happening
   - **Add**: Progress bar or message count

5. **Worktree missing suggestions too technical**:
   - "`csm set <id> --worktree <new-path>`"
   - Users may not know what worktree means
   - **Improve**: "The project directory has been moved or deleted"

6. **No help text examples**:
   - DR-1 requires help text
   - But examples not shown in plan
   - **Add**: Draft help text for resume and backup

### Missing UX Details 🔍

1. **Backup output verbosity**:
   - Always show progress?
   - Quiet mode?
   - **Add**: --quiet flag?

2. **Resume progress feedback**:
   - Auto-recreation may take 2-3 seconds
   - Show spinner? Progress?
   - **Add**: User feedback during recreation

3. **Error recovery guidance**:
   - Resume fails, what to do?
   - Backup fails, what to do?
   - **Add**: Troubleshooting section in help

### Recommendation

**Score**: 8.0/10 - Good UX, needs message specs

**Required additions**:
- All user message specifications (like S1)
- Help text drafts
- Non-interactive mode (--yes flag)

**Recommended**:
- Progress indication for slow operations
- Simplified error messages
- Troubleshooting guidance

---

## Reviewer 6: Security Engineer

**Perspective**: Security, data integrity, attack surface

### Security Assessment ✅

**Data Integrity**:
- ✅ Locks prevent concurrent corruption
- ✅ Rollback on partial failures
- ✅ Backup preserves data

**File Operations**:
- ⚠️ File permissions not specified

### Strengths ✅

1. **Lock integration**: Prevents race conditions
2. **Read-only backup**: Doesn't modify original data
3. **Rollback on failure**: Prevents partial state

### Security Concerns ⚠️

1. **Backup file permissions not specified**:
   - session-info.yaml may contain sensitive paths
   - conversation.jsonl contains conversation history
   - file-snapshots/ contains code
   - **Add**: All backup files 0600 (like S1)

2. **History.jsonl path validation missing**:
   - Hardcoded `~/.claude/history.jsonl`
   - What if symlink to /etc/passwd?
   - **Add**: Path validation (like S1)

3. **Backup directory traversal risk**:
   - Backup path constructed from session name
   - What if session name is `../../../tmp/evil`?
   - **Add**: Path validation for backup directory

4. **Tmux command injection risk**:
   - Session name used in tmux commands
   - What if session name contains backticks or semicolons?
   - **Add**: Input sanitization for tmux commands

5. **Symlink attack in backup**:
   - `file-snapshots/` copied with CopyDirectory
   - What if contains symlink to /etc/passwd?
   - **Mitigated**: CopyDirectory copies as symlink (from S1)
   - **Document**: Security property

6. **Resume executes arbitrary commands**:
   - `claude --resume <uuid>` sent to tmux
   - UUID from manifest (trusted)
   - **OK**: UUID validated in manifest

### Missing Security Details 🔍

1. **Input sanitization for tmux**:
   - Session name in shell commands
   - Need escaping/validation
   - **Add**: Shell escaping function

2. **Backup directory creation**:
   - Who can read backup directories?
   - **Add**: Backup directory permissions 0700

3. **Conversation data sensitivity**:
   - Contains potentially sensitive information
   - **Document**: User should protect backup directory

### Recommendation

**Score**: 7.5/10 - Good but needs hardening

**Required additions**:
- Backup file permissions (0600)
- Path validation (directory traversal)
- Tmux command sanitization

**Recommended**:
- Backup directory permissions (0700)
- Document data sensitivity

---

## Aggregated Review Results (Round 1)

| Reviewer | Score | Key Concerns |
|----------|-------|--------------|
| Senior Go Developer | 8.0/10 | Timestamp format, history parsing, error messages |
| Software Architect | 8.5/10 | Concurrent operations, atomic backup, partial failure |
| QA Engineer | 7.5/10 | Edge cases, concurrent tests, corrupted history |
| DevOps/SRE | 7.5/10 | Verification checklist, rollback, monitoring |
| End User | 8.0/10 | Message specs, help text, non-interactive mode |
| Security Engineer | 7.5/10 | File permissions, path validation, tmux sanitization |

**Average Score**: 7.8/10 ❌ **BELOW THRESHOLD (8.5/10)**

---

## Critical Issues to Address

### Must Fix (Blocking Approval)

1. **All error and user messages specified** (Go Dev, User):
   - Like S1, specify exact text for all messages
   - Resume messages, backup progress, errors
   - Add "User Messages" section (like S1 Technical Specs)

2. **Backup file permissions** (Security):
   - All backup files: 0600
   - Backup directory: 0700
   - Add to D2.3 specification

3. **Tmux command sanitization** (Security):
   - Escape/validate session name before use in shell
   - Prevent command injection
   - Add shell escaping function

4. **Path validation** (Security):
   - Backup directory (prevent traversal)
   - History.jsonl location
   - Worktree path (resolve symlinks safely)
   - Add validation functions

5. **Concurrent operation tests** (QA, Architect):
   - TS-S2-11: Backup + Resume concurrency
   - TS-S2-12: Corrupted history
   - TS-S2-13: Broken symlinks
   - Add to test scenarios

6. **Post-deployment verification** (DevOps):
   - Checklist like S1
   - Test auto-recreation
   - Test backup creation
   - Add new section

7. **Rollback procedure** (DevOps):
   - Like S1, complete guide
   - What if auto-recreation has bugs
   - Add new section

### Should Fix (Strongly Recommended)

8. **Backup timestamp format** (Go Dev):
   - Always include microseconds: `2025-12-07_14-30-00-123456`
   - Prevents collisions
   - Update D2.3 specification

9. **History.jsonl parsing strategy** (Go Dev):
   - Stream processing (don't load entire file)
   - Skip malformed lines
   - Report skipped count
   - Add to D2.3 specification

10. **Backup atomic creation** (Architect):
    - Create in temp directory
    - Move to final location when complete
    - Prevents partial backups
    - Add to D2.3 workflow

11. **Help text drafts** (User):
    - `csm resume --help`
    - `csm backup --help`
    - Add new section

12. **Monitoring guidance** (DevOps):
    - What metrics to track
    - How to track them
    - Add new section

---

## Recommendations for Revision

### New Sections to Add

1. **User Messages & Error Specifications** (like S1):
   - Resume messages (active, stopped, recreating, success)
   - Backup progress messages
   - Error messages (worktree missing, Claude failed, disk full)
   - Exact text for all messages

2. **Post-Deployment Verification**:
   - Test auto-recreation
   - Test backup (both formats)
   - Test concurrent operations
   - Performance verification

3. **Rollback Procedure**:
   - Git rollback
   - Manual workaround (use old resume behavior)
   - When to rollback

4. **Help Text Drafts**:
   - csm resume --help
   - csm backup --help

5. **Monitoring & Metrics**:
   - Auto-recreation success rate
   - Backup success rate
   - Performance metrics

### Updated Sections

**D2.2 Enhanced Resume**:
- Add tmux command sanitization
- Add path validation
- Add exact user messages
- Add --yes flag for non-interactive

**D2.3 Backup**:
- Timestamp format: always include microseconds
- File permissions: 0600 for files, 0700 for directory
- Atomic creation: temp directory → move
- History parsing: stream processing, skip malformed
- Path validation: prevent directory traversal

**Testing Strategy**:
- Add TS-S2-11 through TS-S2-16
- Add concurrent operation tests
- Add corrupted history tests
- Add security tests

---

## Next Steps

1. Create S2-SPRINT-PLAN-v2.md addressing all feedback
2. Run Round 2 review
3. Target score: ≥8.5/10

**Status**: ❌ REVISION NEEDED - Round 2 Review Required
