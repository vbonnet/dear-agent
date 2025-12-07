# S1 Sprint Plan Summary - Foundation & Core Infrastructure

**Date**: December 7, 2025
**Status**: ✅ APPROVED (9.4/10)
**Commit**: f93452a

---

## Executive Summary

S1 Sprint Plan has been **APPROVED** with a score of **9.4/10** after two review rounds.

The comprehensive sprint plan defines all implementation details for Sprint 1, which focuses on foundational infrastructure: manifest schema v2, migration logic, file locking, validation, and core utilities.

---

## Review Results

### Round 1: 8.2/10 ❌ (Revision Needed)

**Critical gaps identified**:
- Lock file format not exactly specified
- Migration concurrency undefined
- File permissions not specified
- Test data preparation missing
- Error messages not fully specified
- Post-deployment verification missing

### Round 2: 9.4/10 ✅ (APPROVED)

**All critical issues resolved**:

| Reviewer | R1 Score | R2 Score | Change |
|----------|----------|----------|--------|
| Senior Go Developer | 8.5/10 | 9.5/10 | +1.0 ⬆️ |
| Software Architect | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| QA Engineer | 7.5/10 | 9.5/10 | +2.0 ⬆️ |
| DevOps/SRE | 8.0/10 | 9.5/10 | +1.5 ⬆️ |
| End User | 8.5/10 | 9.0/10 | +0.5 ⬆️ |
| Security Engineer | 8.5/10 | 9.5/10 | +1.0 ⬆️ |

**Average**: 9.4/10 ✅ **EXCEEDS THRESHOLD (8.5/10)**

---

## Sprint 1 Scope

**Goal**: Implement foundational infrastructure for session persistence

**Deliverables** (5 of 11 total in Phase 3.5):

1. **D1.1: Manifest Schema v2** (FR-1)
   - Schema version "2.0"
   - Lifecycle field (replaces Status)
   - Context struct (Purpose, Tags, Notes)
   - RFC3339 timestamps throughout
   - Constants centralized

2. **D1.2: Context Field Validation** (FR-1.2, FR-3)
   - UTF-8 character counting (not bytes)
   - Boundary conditions (exactly 256 chars = PASS)
   - Clear error messages with actual vs max
   - Validation before serialization

3. **D1.3: File Locking** (FR-4)
   - Exclusive lock acquisition with O_EXCL
   - Lock file format: `"<PID>\n<RFC3339>\n"`
   - Stale lock detection (60s timeout)
   - File permissions: 0600
   - Path validation

4. **D1.4: Schema Migration** (FR-2)
   - Automatic v1→v2 migration
   - Acquire lock before migration (prevents race)
   - Backup (.v1.bak) before migration
   - Rollback on failure
   - Migration logging to `~/.csm/logs/migration.log`
   - One-time user notice

5. **D1.5: Fileutil Package** (FR-10)
   - CopyFile with validation
   - WriteAtomic (temp + rename)
   - CopyDirectory (recursive, symlinks as symlinks)

**Duration Estimate**: 2-3 days

---

## Key Technical Specifications

### Lock File Format
```
<PID>\n<RFC3339>\n
```

**Example**:
```
12345
2025-12-07T14:30:00-08:00
```

### Migration Log Format
```
[<RFC3339>] <STATUS>: <path> [- <error>]
```

**Example**:
```
[2025-12-07T14:30:00-08:00] SUCCESS: /home/user/sessions/session-claude-1/manifest.yaml
[2025-12-07T14:31:00-08:00] FAILED: /home/user/sessions/session-claude-2/manifest.yaml - invalid YAML
```

### File Permissions
- Lock files (`.lock`): 0600
- Log files (`migration.log`): 0600
- Notice file (`.migration-notice-shown`): 0600
- Backup files (`.v1.bak`): Inherit from original

### Error Messages

**Lock Conflict**:
```
Error: session is locked by process 12345 (started 2025-12-07T14:30:00-08:00)

Try one of the following:
  • Wait a minute and retry (process may finish)
  • Check if process is still running: ps -p 12345
  • If process is stuck, kill it: kill 12345
  • Check for stale locks: csm doctor --fix
```

**Validation Error**:
```
Error: context validation failed: purpose too long (300 chars, max 256)

Please shorten the purpose field and try again.
```

**Migration Success**:
```
📝 Migrating session-claude-1 to schema v2...
✅ Migration successful - backup saved as manifest.yaml.v1.bak
```

---

## Major Improvements from v1 to v2

### Technical Specifications Section (NEW)
- Lock file format exactly specified
- Migration log format defined
- One-time notice full text (~150 words)
- All error messages specified
- File permissions documented

### Security Hardening
- File permissions: 0600 for sensitive files
- Path validation (directory traversal prevention)
- O_EXCL symlink protection
- Security test files added

### Test Coverage Enhanced
- **10 integration tests** (was 4): TS-S1-1 through TS-S1-10
- **Test data preparation**: testdata/ directory with 10+ manifests
- **Stress tests**: 10 concurrent processes, UTF-8 edge cases
- **Security tests**: Permissions, symlinks, path validation

### Operational Readiness
- **Post-deployment verification**: 6-step checklist with commands
- **Rollback procedure**: Complete guide (git + manual manifest restoration)
- **When to rollback**: Clear criteria (<95% success, corruption, deadlocks)
- **Monitoring examples**: Grep commands for log parsing

### Implementation Details
- **Migration acquires lock**: Prevents concurrent race condition
- **Atomic initialization**: Code example provided
- **Error wrapping**: Use `%w` consistently
- **Symlink handling**: Copy as symlink, not target
- **Stale lock timeout**: 60 seconds (confirmed, not 5 min)

---

## Files to Create

### New Files
```
internal/manifest/
  ├── constants.go (all magic values)
  ├── validate.go (context validation)
  ├── lock.go (file locking)
  ├── migrate.go (v1→v2 migration)
  ├── constants_test.go
  ├── validate_test.go
  ├── lock_test.go
  ├── lock_stale_test.go
  ├── lock_security_test.go
  ├── migrate_test.go
  ├── migrate_rollback_test.go
  ├── migrate_edge_test.go
  ├── migrate_concurrent_test.go
  ├── migrate_security_test.go
  └── testdata/manifests/ (10+ test manifests)

internal/fileutil/
  ├── fileutil.go
  ├── fileutil_test.go
  ├── fileutil_symlink_test.go
  └── fileutil_security_test.go
```

### Modified Files
```
internal/manifest/
  ├── manifest.go (add v2 fields)
  └── manifest_test.go (add v2 tests)
```

---

## Test Strategy

### Test Coverage Targets
- Critical paths: >80%
- Overall: >60%
- All P0 requirements: 100%

### Integration Tests (10 scenarios)
1. **TS-S1-1**: V1 migration flow
2. **TS-S1-2**: Concurrent migration (lock prevents race)
3. **TS-S1-3**: Concurrent lock conflict
4. **TS-S1-4**: Migration rollback
5. **TS-S1-5**: Stale lock recovery
6. **TS-S1-6**: Validation boundary conditions
7. **TS-S1-7**: Lock file orphaned (process crash)
8. **TS-S1-8**: Backup file already exists
9. **TS-S1-9**: Validation during migration
10. **TS-S1-10**: Atomic write failure

### Stress Tests
- 10 concurrent lock attempts
- UTF-8 multibyte characters (emoji, combining chars)
- Symlink attack prevention

---

## Post-Deployment Verification

Six-step checklist to verify S1 works:

1. **Migration Works**
   ```bash
   cp testdata/v1-simple.yaml /tmp/test-manifest.yaml
   csm list
   # Verify: .v1.bak created, schema_version: "2.0", migration.log entry
   ```

2. **Locking Works**
   ```bash
   # Terminal 1: csm resume claude-1
   # Terminal 2: csm resume claude-1  # Should fail with clear error
   # Verify error includes PID, timestamp, suggestions
   ```

3. **Validation Works**
   ```bash
   csm set claude-1 --purpose "$(printf 'x%.0s' {1..300})"
   # Verify: validation error, shows actual (300) vs max (256)
   ```

4. **Backups Created**
   ```bash
   ls -la ~/sessions/session-*/
   # Verify: .v1.bak files exist with correct permissions
   ```

5. **Logging Works**
   ```bash
   cat ~/.csm/logs/migration.log
   # Verify: All migrations logged, RFC3339 timestamps, permissions 0600
   ```

6. **Stale Lock Cleanup**
   ```bash
   # Create artificial stale lock > 60s old
   # Run operation
   # Verify: Stale lock removed, operation proceeds
   ```

---

## Rollback Procedure

If S1 has critical bugs:

### 1. Git Rollback
```bash
git revert f93452a
go build -o csm ./cmd/csm
```

### 2. Manual Manifest Rollback
```bash
# Restore all .v1.bak files
for backup in ~/sessions/*/manifest.yaml.v1.bak; do
    manifest="${backup%.v1.bak}"
    mv "$backup" "$manifest"
done
```

### 3. Clean Up
```bash
find ~/sessions -name "*.lock" -delete
rm ~/.csm/.migration-notice-shown
cat ~/.csm/logs/migration.log | grep FAILED
```

**When to Rollback**:
- Migration success rate < 95%
- Data corruption detected
- Deadlocks in locking

---

## Implementation Plan

### Day 1 (Foundation)
- Morning: Constants (1h) + Manifest Schema (3h)
- Afternoon: Validation (3h) + Fileutil (3h)

### Day 2 (Core Logic)
- Morning: File Locking (4h)
- Afternoon: Schema Migration (4h)

### Day 3 (Testing & Polish)
- Morning: Unit tests (4h)
- Afternoon: Integration tests (2h) + Documentation (2h)

---

## Success Criteria

S1 is **DONE** when:

1. ✅ All 5 deliverables implemented
2. ✅ All P0 acceptance criteria met
3. ✅ Test coverage >80% critical, >60% overall
4. ✅ All tests passing (unit + integration + stress)
5. ✅ Code documented (godoc + inline)
6. ✅ Technical specs implemented
7. ✅ Multi-persona review ≥8.5/10
8. ✅ No critical bugs
9. ✅ Post-deployment verification passed
10. ✅ Rollback procedure tested

---

## Out of Scope (Later Sprints)

Not in S1:
- Enhanced resume with auto-recreation (S2)
- Backup command (S2)
- Doctor command (S3)
- Status computation (S2)
- Log rotation (S3)
- Performance benchmarks (S3)

---

## Files Created

- `S1-SPRINT-PLAN.md` (v1 - 8.2/10)
- `S1-SPRINT-PLAN-v2.md` (v2 - 9.4/10) ✅
- `S1-REVIEW-R1.md` (6 personas, detailed feedback)
- `S1-REVIEW-R2.md` (6 personas, final approval)
- `S1-SUMMARY.md` (this document)

**Commits**:
- `f93452a` - S1 sprint plan and reviews

---

## Wayfinder Progress

- ✅ **D1 Discovery** - Research complete
- ✅ **D2 Architecture** - Approved 8.8/10
- ✅ **D3 Implementation Design** - Approved 9.0/10
- ✅ **D4 Requirements** - Approved 9.3/10
- ✅ **S1 Sprint Plan** - Approved 9.4/10 ← **CURRENT**
- ⏸️ **S1 Implementation** - Awaiting your approval to proceed

---

## Next Steps

**I'm now paused per Wayfinder methodology.**

You can:
1. **Approve and proceed** - Begin S1 implementation (2-3 days coding)
2. **Review sprint plan** - Examine S1-SPRINT-PLAN-v2.md and suggest changes
3. **Different task** - Work on something else

All work is committed and ready for your review at `~/src/repos/ai-tools/base/claude-session-manager/wayfinder-projects/session-persistence/`.

---

**End of S1 Summary**
