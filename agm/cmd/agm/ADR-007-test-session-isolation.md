# ADR-007: Test Session Isolation and Cleanup Strategy

**Status:** Accepted
**Date:** 2026-03-20
**Deciders:** Engineering Team
**Context:** Test sessions were polluting production workspace, making it difficult to find actual work sessions

---

## Context and Problem Statement

The `oss` workspace accumulated 98 sessions, including many ephemeral test sessions (28 active, 52 stopped) created without proper isolation. Test sessions appeared in `agm session list` indefinitely, creating data cleanup burden and cognitive overhead.

**Root Cause:** Users and agents created sessions with "test-*" names but forgot to use the `--test` flag, which would properly isolate them in `~/sessions-test/` instead of production `~/.claude/sessions/`.

**Impact:**
- Production workspace cluttered with ephemeral test sessions
- Difficult to find actual work sessions in listings
- Manual cleanup required periodically
- No automated prevention mechanism

---

## Decision Drivers

1. **User Experience:** Minimize cognitive overhead when browsing sessions
2. **Data Hygiene:** Keep production workspace clean and organized
3. **Safety:** Never lose important conversation data during cleanup
4. **Education:** Teach users correct patterns through interaction
5. **Enforcement:** Prevent future pollution automatically
6. **Flexibility:** Allow legitimate test-related work in production

---

## Considered Options

### Option 1: Manual Cleanup Only
**Description:** Provide cleanup command, rely on users to run it periodically

**Pros:**
- Simple implementation
- No automated enforcement overhead
- User maintains full control

**Cons:**
- Pollution continues between cleanup runs
- Requires user discipline
- Doesn't prevent root cause
- Manual process prone to skipping

**Verdict:** ❌ Rejected - Doesn't address prevention

### Option 2: Hard Block Test Patterns
**Description:** Completely block session names containing "test" without override

**Pros:**
- Maximum enforcement
- Zero pollution in production
- Simple rule to understand

**Cons:**
- Breaks legitimate use cases (testing-auth-feature)
- Frustrating for users working on test-related features
- No flexibility for edge cases

**Verdict:** ❌ Rejected - Too restrictive

### Option 3: Layered Defense (SELECTED)
**Description:** Three complementary layers: cleanup (reactive), interactive (educational), automated (preventive)

**Pros:**
- Comprehensive coverage (prevents and cleans)
- Educational (teaches users --test flag)
- Flexible (override mechanism exists)
- Safe (backup before deletion)
- Progressive enforcement (prompt before block)

**Cons:**
- More complex implementation
- Multiple systems to maintain

**Verdict:** ✅ **Selected** - Best balance of safety, education, and enforcement

---

## Decision Outcome

**Chosen Option:** Layered Defense Strategy with three complementary layers

### Layer 1: Cleanup (Reactive)
- **Command:** `agm admin cleanup-test-sessions`
- **Purpose:** Remove existing pollution safely
- **Mechanism:**
  - Scan sessions matching `^test-` pattern
  - Analyze conversation.jsonl for message count
  - Interactive multi-select for user review
  - Automatic backup to `~/.agm/backups/sessions/` before deletion
  - Delete session directory after backup

### Layer 2: Interactive Prevention (Educational)
- **Trigger:** Session name contains "test" (case-insensitive substring)
- **Purpose:** Educate users about `--test` flag at point of action
- **Mechanism:**
  - Detect test pattern in `agm new` command
  - Show interactive prompt with three options:
    1. Use --test flag (recommended)
    2. Cancel and rename
  - Explain consequences of each choice
  - Apply user's choice immediately

### Layer 3: Automated Prevention (Enforcement)
- **Tool:** PreToolUse hook (`~/.claude/hooks/pretool-test-session-guard`)
- **Purpose:** Block test session creation in automated contexts
- **Mechanism:**
  - Intercept `agm session new test-*` commands
  - Block if `--test` flag not present
  - Show clear error message with remediation
  - Allow override with `--allow-test-name` flag
  - Graceful degradation on hook errors

### Override Mechanism
- **Flag:** `--allow-test-name`
- **Purpose:** Allow legitimate production sessions with "test" in name
- **Use Cases:**
  - Working on testing infrastructure (e.g., `test-harness-refactor`)
  - Documentation about testing (e.g., `test-docs-update`)
  - Test-driven development sessions (e.g., `tdd-payment-flow`)
- **Guidance:** Use sparingly - most test work should use `--test` flag

---

## Consequences

### Positive
1. **Clean Production Workspace:** Zero test sessions in production after cleanup
2. **User Education:** Interactive prompts teach --test flag usage naturally
3. **Automated Enforcement:** PreToolUse hook prevents future pollution
4. **Safety:** Backup system ensures no data loss during cleanup
5. **Flexibility:** Override mechanism supports legitimate use cases
6. **Comprehensive Testing:** 200+ test cases ensure reliability

### Negative
1. **Complexity:** Three systems to maintain vs single approach
2. **Hook Dependency:** Requires hook installation for full enforcement
3. **Override Misuse:** Users might overuse `--allow-test-name`

### Neutral
1. **Pattern Detection:** Substring matching ("test" anywhere) more restrictive than planned ("test-*" prefix only), but acceptable with override
2. **Test Pattern Scope:** Catches more cases but provides clear override path

---

## Implementation Details

### Test Pattern Detection
**Pattern:** Case-insensitive substring match for "test"

**Triggers Prompt/Block:**
- `test-foo` (exact target pattern)
- `TEST-FOO` (uppercase variant)
- `Test-Bar` (mixed case)
- `my-testing` (contains "test")
- `contest` (contains "test" - false positive but acceptable)

**Does NOT Trigger:**
- `my-test-feature` (starts with legitimate prefix)
- `latest` (substring "test" not standalone word)

**Rationale:** Conservative approach catches more cases, override available for false positives.

### Cleanup Safety
**Backup Format:** Tarball (`.tar.gz`) with full directory structure

**Backup Location:** `~/.agm/backups/sessions/`

**Backup Naming:** `{session-name}-{timestamp}.tar.gz`

**Restore Process:** Manual extraction or future `agm admin restore-session` command

### Test Coverage
**Hook Tests:** 5 test functions with 18 sub-tests
- Block test-* patterns
- Allow with --test flag
- Allow with --allow-test-name flag
- Allow legitimate names
- Edge cases (uppercase, mixed case, graceful degradation)

**Integration Tests:** Full lifecycle coverage
- Cleanup command dry-run
- Cleanup command execution
- Interactive prompt scenarios
- Override flag behavior

**Total Coverage:** 95 packages, 200+ test cases, 100% pass rate

---

## Architectural Impact

### Component Changes
1. **cmd/agm/new.go:** Add --allow-test-name flag, pattern detection
2. **cmd/agm/cleanup_test_sessions.go:** New admin command
3. **internal/conversation/analyzer.go:** Message counting logic
4. **internal/backup/session_backup.go:** Tarball backup/restore
5. **scripts/hooks/pretool-test-session-guard.py:** PreToolUse hook

### Dependencies
- Existing conversation.jsonl format (no changes)
- Existing backup infrastructure (extended)
- Existing hook system (new hook added)
- Existing admin command structure (new command added)

### Backward Compatibility
- ✅ Existing sessions unaffected
- ✅ --test flag behavior unchanged
- ✅ No breaking changes to CLI API
- ✅ Hook is optional (degraded UX without it, not broken)

---

## Alternatives Considered

### Alternative 1: Auto-Delete After N Days
**Description:** Automatically delete test-* sessions after 7 days

**Rejected Because:**
- Risk of deleting active work
- No user control over deletion
- Timestamp-based approach less reliable than message count
- Doesn't prevent pollution, just delays cleanup

### Alternative 2: Separate Database for Test Sessions
**Description:** Store test sessions in separate Dolt database

**Rejected Because:**
- Over-engineering for the problem
- Maintenance overhead for two databases
- Complicates session resolution
- `--test` flag already provides isolation via filesystem

### Alternative 3: Mandatory --test Flag for All Test Patterns
**Description:** Hard requirement, no override mechanism

**Rejected Because:**
- Breaks legitimate use cases
- Frustrating user experience
- No flexibility for edge cases
- Education + enforcement better than pure enforcement

---

## Related Decisions

- **ADR-006:** Test Isolation Enforcement (PreToolUse hooks)
- **ADR-012:** Test Infrastructure Dolt Migration (test isolation patterns)
- **NFR4:** Testability requirements (--test flag original purpose)

---

## Follow-Up Actions

1. **Monitor Hook Effectiveness:** Track how often hook blocks vs allows
2. **Measure --allow-test-name Usage:** Identify if override is being misused
3. **Add Metrics Collection:** Count cleanup command usage, test session creation patterns
4. **Enhance Cleanup Command:** Add workspace filtering, batch operations, restore command
5. **Automated Lifecycle:** Consider auto-expiring test sessions after N days (future iteration)
6. **Pattern Detection Extensions:** Configurable patterns via `~/.agm/test-patterns.yaml`

---

## References

- **Implementation:** `feat/test-session-cleanup` branch
- **Tests:** `test/integration/lifecycle/test_session_guard_hook_test.go`
- **Documentation:** `docs/TEST-SESSION-GUIDE.md`
- **Retrospective:** `RETROSPECTIVE-TEST-SESSION-CLEANUP.md`
- **Plan:** `~/.claude/plans/tranquil-dreaming-crane.md`

---

**Last Reviewed:** 2026-03-20
**Review Cycle:** Quarterly (or when test session patterns change significantly)
