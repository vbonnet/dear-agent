# S3 Sprint Plan Review - Round 1

**Date**: December 7, 2025
**Document**: S3-SPRINT-PLAN.md
**Review Type**: Multi-Persona Review

---

## Reviewer 1: Senior Go Developer

**Perspective**: Implementation feasibility, Go best practices, code organization

### Assessment ✅

**Sprint Scope**:
- ✅ Well-defined: 3 deliverables completing Phase 3.5
- ✅ Builds on S1+S2 foundation
- ✅ Logical grouping (doctor, rotation, testing)

**Code Organization**:
- ✅ Doctor in cmd/csm/doctor.go
- ✅ Rotation in internal/logging/rotate.go
- ✅ Clean separation of concerns

### Strengths ✅

1. **Check functions structured**: Each health check returns CheckResult
2. **Rotation algorithm clear**: Temp file + atomic rename
3. **Test infrastructure planned**: Benchmarks use standard Go testing
4. **Integration points shown**: Uses S1/S2 components correctly

### Concerns ⚠️

1. **Doctor check implementation details vague**:
   - "Parse history.jsonl, verify session UUIDs"
   - How exactly? Stream or load entire file?
   - What if history.jsonl is 100MB? 1GB?
   - **Add**: Parsing strategy (reuse backup streaming code from S2)

2. **Rotation file handling not atomic enough**:
   - Shows: .log.4 → .log.5, .log.3 → .log.4, etc.
   - What if failure during rotation (power loss)?
   - Partial rotation could lose .log.1
   - **Better**: Document rollback strategy or use transaction-like approach

3. **Integration test mock strategy missing**:
   - "Test harness for tmux operations"
   - How to mock tmux in tests?
   - Real tmux or mock?
   - **Add**: Mock strategy specification

4. **Benchmark data setup not specified**:
   - BM-2 "Create 50 sessions" - how created?
   - With real tmux? Mock? Fixtures?
   - **Add**: Test data preparation section

5. **Doctor fix mode concurrent safety**:
   - What if doctor --fix runs while resume is running?
   - Doctor might remove lock that resume just acquired
   - **Add**: Check lock age more carefully (< 60s = don't touch)

6. **Error handling in rotation not fully specified**:
   - "Rotation failure: Log to stderr, continue"
   - But what if current log also fails to write?
   - **Add**: Fallback strategy (write to /tmp?)

### Missing Details 🔍

1. **Doctor output buffering**:
   - Checks run sequentially or parallel?
   - Output streamed or buffered?
   - **Clarify**: Sequential checks with immediate output

2. **Rotation file permissions**:
   - Rotated files keep 0600?
   - **Add**: Preserve permissions during rotation

3. **Integration test cleanup**:
   - How to clean up tmux sessions between tests?
   - **Add**: Test cleanup strategy

### Recommendation

**Score**: 8.0/10 - Good plan, needs implementation details

**Required additions**:
- Doctor history.jsonl parsing strategy
- Rotation partial failure handling
- Test mock strategy
- Lock age check in doctor fix mode

**Recommended**:
- Error fallback for rotation
- Test data preparation details

---

## Reviewer 2: Software Architect

**Perspective**: System design, dependencies, integration

### Architecture Assessment ✅

**Layering**:
- ✅ Doctor uses S1 (manifest, lock) and S2 (status)
- ✅ Rotation integrated with S1 migration logger
- ✅ Testing validates all layers

**Dependencies**:
- ✅ S3 correctly depends on S1+S2 (not vice versa)
- ✅ No circular dependencies

### Strengths ✅

1. **Doctor as diagnostic tool**: Good separation (doesn't modify except --fix)
2. **Rotation on-demand**: No background daemon (simpler)
3. **Testing strategy comprehensive**: Unit + integration + performance

### Concerns ⚠️

1. **Doctor UUID check performance**:
   - Checks all UUIDs in history.jsonl
   - History file grows unbounded (Claude's responsibility)
   - What if 50 sessions × check each UUID = 50 full file reads?
   - **Optimization**: Cache history.jsonl parse (read once, check all UUIDs)

2. **Rotation race with concurrent writers**:
   - Migration logs from multiple CSM processes
   - One process rotates, another tries to write to old file
   - **Issue**: File descriptor might become invalid
   - **Better**: Document that rotation only happens on write (caller's file handle)

3. **Integration test isolation unclear**:
   - Tests create real sessions in ~/sessions?
   - Or temporary directory?
   - **Clarify**: Use isolated test directories (e.g., /tmp/csm-test-XXX)

4. **Doctor fix mode idempotency**:
   - Running `csm doctor --fix` twice should be safe
   - But not specified
   - **Add**: Idempotency guarantee

5. **Rotation cleanup strategy**:
   - "Delete .log.5 before rotation"
   - What if .log.6 somehow exists (manual creation)?
   - **Add**: Clean up all .log.N where N > 5

### Missing Architectural Details 🔍

1. **Doctor check ordering**:
   - Which checks run first?
   - Does order matter?
   - **Clarify**: Logical ordering (directory → manifests → locks → UUIDs → worktrees)

2. **Rotation transaction semantics**:
   - What's the atomic unit?
   - One log line? Or full rotation?
   - **Document**: Rotation is not transactional (best-effort)

3. **Integration test concurrency control**:
   - Tests run in parallel (go test -parallel)?
   - Or sequential?
   - **Add**: Parallel safety or sequential requirement

### Recommendation

**Score**: 8.5/10 - Solid architecture, minor optimizations needed

**Required additions**:
- Doctor UUID check optimization (cache history parse)
- Integration test isolation strategy
- Doctor fix idempotency guarantee

**Recommended**:
- Rotation race condition documentation
- Check ordering specification

---

## Reviewer 3: QA Engineer

**Perspective**: Testability, edge cases, failure modes

### Test Coverage Assessment ✅

**Unit Tests**:
- ✅ Tests per deliverable
- ✅ Individual check functions testable

**Integration Tests**:
- ✅ 10 scenarios covering main workflows
- ✅ Performance benchmarks defined

### Strengths ✅

1. **Integration test scenarios well-designed**: Cover key user workflows
2. **Performance benchmarks specific**: Each has target metric
3. **Test coverage targets**: >80% critical, >60% overall

### Testing Gaps ⚠️

1. **No doctor false positive tests**:
   - What if worktree is symlink to another location? (should resolve)
   - What if UUID not in history but session is brand new? (warn, not fail)
   - **Add**: TS for doctor edge cases

2. **No rotation failure tests**:
   - Disk full during rotation
   - Permissions changed mid-rotation
   - .log.tmp already exists (previous crash)
   - **Add**: TS for rotation failures

3. **No integration test for doctor + concurrent resume**:
   - Mentioned in risk management
   - But no test scenario
   - **Add**: TS-INT-11: Doctor fix runs concurrently with resume

4. **No long-running stability test details**:
   - TS-INT-10 mentions "no memory leaks"
   - But how to detect? Tools? Thresholds?
   - **Add**: Memory leak detection strategy

5. **No test for doctor quiet mode exit codes**:
   - Separate test needed for each exit code (0, 1, 2)
   - **Add**: Explicit test cases for exit codes

6. **No benchmark for rotation**:
   - Performance target: < 100ms
   - But no benchmark to verify
   - **Add**: BM-9: Log rotation performance

7. **No test for doctor with 0 sessions**:
   - Fresh install, no sessions
   - Doctor should handle gracefully
   - **Add**: TS for empty sessions directory

### Missing Test Scenarios 📝

**TS-INT-11: Doctor Fix + Concurrent Resume**:
- Doctor --fix starts (finds stale lock)
- Resume starts (acquires same lock)
- Verify: Doctor doesn't remove active lock

**TS-INT-12: Rotation Disk Full**:
- Create .log.tmp successfully
- Simulate disk full during rename
- Verify: Cleanup .tmp, fallback to current log

**TS-INT-13: Doctor Empty System**:
- No sessions exist
- Run doctor
- Verify: Passes with "0 sessions" message

**TS-INT-14: Doctor New Session (UUID Not in History)**:
- Create session
- Delete from history.jsonl (simulate brand new)
- Run doctor
- Verify: Warning (not error)

**TS-INT-15: Rotation .log.tmp Already Exists**:
- Create stale .log.tmp (previous crash)
- Trigger rotation
- Verify: Cleanup .tmp, proceed with rotation

### Recommendation

**Score**: 7.5/10 - Good coverage, missing edge cases

**Critical additions**:
- Doctor + concurrent resume test
- Rotation failure tests
- Exit code tests
- Empty system test

**Recommended**:
- Memory leak detection strategy
- Rotation benchmark
- Doctor false positive tests

---

## Reviewer 4: DevOps/SRE

**Perspective**: Operations, deployment, observability

### Operational Assessment ✅

**Deployment**:
- ✅ Builds on S1+S2 (no breaking changes)
- ✅ Additive: doctor command, rotation feature

**Observability**:
- ✅ Doctor provides health check endpoint
- ✅ Monitoring guidance provided

### Strengths ✅

1. **Doctor as monitoring tool**: Exit codes enable automation
2. **Log rotation prevents disk full**: 10MB × 6 files = max 60MB
3. **Post-deployment verification**: 8-step checklist

### Operational Concerns ⚠️

1. **No guidance on doctor scheduling**:
   - How often to run doctor in production?
   - Cron job? On-demand?
   - **Add**: Recommended doctor schedule (daily? hourly?)

2. **No alert triggers specified**:
   - Doctor finds errors - then what?
   - Who gets notified? How?
   - **Add**: Alert integration examples (email, Slack, PagerDuty)

3. **Rotation impact on log analysis not discussed**:
   - Logs split across .log.1, .log.2, etc.
   - How to analyze? Concatenate?
   - **Add**: Log analysis guidance with rotation

4. **No guidance on stale lock threshold tuning**:
   - 60s hardcoded
   - What if operations legitimately take > 60s?
   - **Document**: Why 60s, when to adjust

5. **Integration test CI/CD time budget**:
   - "All tests complete in < 2 minutes"
   - But TS-INT-10 is "long-running stability" (100 iterations)
   - 100 iterations × 3s/resume = 300s = 5 minutes minimum
   - **Clarify**: Separate long tests (nightly) from fast tests (PR)

6. **No doctor performance impact discussion**:
   - Doctor reads all manifests + history.jsonl
   - Could be heavy in large installations (100+ sessions)
   - **Add**: Performance guidance for large installations

### Missing Operational Details 🔍

1. **Doctor in CI/CD pipeline**:
   - Should CI run doctor after deploy?
   - **Add**: CI integration examples

2. **Log rotation disk usage monitoring**:
   - How to alert if approaching 60MB?
   - **Add**: Disk usage monitoring commands

3. **Rollback impact on logs**:
   - Rollback S3 → rotation stops
   - What happens to rotated logs?
   - **Document**: Logs remain, rotation just stops

### Recommendation

**Score**: 7.5/10 - Functional but needs operational guidance

**Required additions**:
- Doctor scheduling recommendations
- Alert integration examples
- Test time budget clarification (fast vs slow tests)

**Recommended**:
- Log analysis guidance
- Stale lock threshold rationale
- Performance guidance for large installations

---

## Reviewer 5: End User / Developer

**Perspective**: Daily usage, UX, developer experience

### User Experience Assessment ✅

**User Value**:
- ✅ Doctor enables self-service troubleshooting
- ✅ Automatic log management (transparent)
- ✅ Testing ensures reliability

**Messaging**:
- ✅ Doctor help text comprehensive
- ✅ Output examples provided

### Strengths ✅

1. **Doctor help text excellent**: Examples, exit codes, troubleshooting
2. **Output messages clear**: ✓, ⚠, ✗ indicators intuitive
3. **Fix mode user-friendly**: Auto-fixes with confirmation

### UX Concerns ⚠️

1. **Doctor output too verbose for many sessions**:
   - 50 sessions × 6 checks = 300 lines
   - Example shows only 2 sessions
   - **Add**: Summary format for many sessions option

2. **No progress indication for slow checks**:
   - UUID check might take time (large history.jsonl)
   - User sees nothing for several seconds
   - **Add**: Progress indicator for slow checks

3. **Doctor doesn't suggest next steps**:
   - Shows "2 warnings, 1 error"
   - But doesn't tell user what to do beyond "Run --fix"
   - **Improve**: Add specific suggestions per issue type

4. **Fix mode doesn't preview**:
   - `csm doctor --fix` immediately fixes
   - No --dry-run to see what would be fixed
   - **Add**: --dry-run flag

5. **No doctor --watch mode**:
   - Users might want continuous monitoring
   - **Future**: Document as Phase 4 enhancement

6. **Error message for missing sessions directory unclear**:
   - What if ~/sessions doesn't exist (fresh install)?
   - Just says "✗ Sessions directory missing"
   - **Improve**: Add "Run 'csm create <name>' to create first session"

### Missing UX Details 🔍

1. **Doctor --fix confirmation**:
   - Should ask "Remove 3 stale locks? (y/n)"
   - Or just do it?
   - **Clarify**: Auto-fix (no prompt) but show what was fixed

2. **Quiet mode verbosity level**:
   - Only show errors? Or warnings too?
   - **Clarify**: Warnings + errors (not just errors)

3. **Help text for rotation**:
   - Users don't run rotation manually
   - But should understand behavior
   - **Add**: Brief rotation explanation in doctor help or docs

### Recommendation

**Score**: 8.0/10 - Good UX, needs polish

**Required additions**:
- Summary format for many sessions
- Specific suggestions per issue type
- --dry-run flag for fix mode

**Recommended**:
- Progress indicator for slow checks
- Better fresh install messaging
- Fix confirmation clarity

---

## Reviewer 6: Security Engineer

**Perspective**: Security, data integrity, attack surface

### Security Assessment ✅

**Data Integrity**:
- ✅ Doctor doesn't modify data (except --fix for locks)
- ✅ Rotation preserves log integrity
- ✅ Tests validate security from S1/S2

**Attack Surface**:
- ⚠️ Doctor reads history.jsonl (untrusted data?)

### Strengths ✅

1. **Doctor read-only by default**: --fix required for modifications
2. **Rotation atomic**: Temp file + rename pattern
3. **Lock age check**: Prevents removing active locks

### Security Concerns ⚠️

1. **Doctor history.jsonl parsing**:
   - history.jsonl controlled by Claude (external tool)
   - Could be maliciously modified
   - Malformed JSON → crash?
   - **Mitigate**: Use same stream parsing from S2 (skip malformed lines)

2. **Doctor symlink following in worktree check**:
   - Uses `filepath.EvalSymlinks()` (from S2)
   - What if symlink points to /etc/passwd?
   - Doctor would report "exists" for system files
   - **Acceptable**: Doctor is diagnostic, not modifying
   - **Document**: Symlink following is intentional

3. **Rotation temp file permissions**:
   - Creates .log.tmp
   - Permissions not specified
   - **Add**: .log.tmp created with 0600

4. **Doctor --fix without audit log**:
   - Removes stale locks
   - No record of what was removed (except stdout)
   - **Recommend**: Log fix actions to migration.log

5. **Integration tests might leak test data**:
   - Tests create sessions in /tmp
   - But what if /tmp is cleared mid-test?
   - **Add**: Test data in dedicated temp dir with proper cleanup

### Missing Security Details 🔍

1. **Doctor input validation**:
   - Session identifier from user
   - Already validated in S1/S2 (manifest resolution)
   - **OK**: Reuses existing validation

2. **Rotation file permissions during move**:
   - .log → .log.1 rename
   - Permissions preserved?
   - **Add**: Document permission preservation

### Recommendation

**Score**: 8.5/10 - Secure, minor hardening needed

**Required additions**:
- Rotation temp file permissions (0600)
- Doctor fix action logging

**Recommended**:
- History.jsonl parsing strategy (reuse S2 streaming)
- Symlink following documentation

---

## Aggregated Review Results (Round 1)

| Reviewer | Score | Key Concerns |
|----------|-------|--------------|
| Senior Go Developer | 8.0/10 | History parsing, rotation safety, test mocks, lock age check |
| Software Architect | 8.5/10 | UUID check optimization, test isolation, idempotency |
| QA Engineer | 7.5/10 | Edge cases, rotation failures, concurrent tests, benchmarks |
| DevOps/SRE | 7.5/10 | Doctor scheduling, alerts, test time budget, large install perf |
| End User | 8.0/10 | Summary format, progress indication, dry-run mode |
| Security Engineer | 8.5/10 | Rotation permissions, fix action logging |

**Average Score**: 8.0/10 ❌ **BELOW THRESHOLD (8.5/10)**

---

## Critical Issues to Address

### Must Fix (Blocking Approval)

1. **Doctor history.jsonl parsing strategy** (Go Dev):
   - Reuse S2 backup streaming approach
   - Skip malformed lines
   - Report skipped count
   - Add to D3.1 specification

2. **Doctor UUID check optimization** (Architect):
   - Read history.jsonl once, cache all UUIDs
   - Check all sessions against cached set
   - Not 50 separate file reads
   - Add to D3.1 implementation details

3. **Doctor fix mode lock age check** (Go Dev):
   - Check timestamp more carefully
   - Don't remove locks < 60s old
   - Test concurrent doctor + resume
   - Add to D3.1 tasks

4. **Test time budget clarification** (DevOps):
   - Separate fast tests (< 2 min for CI)
   - Separate slow tests (nightly, no time limit)
   - Update Testing Strategy section

5. **Rotation temp file permissions** (Security):
   - Create .log.tmp with 0600
   - Add to D3.2 specification

6. **Additional integration tests** (QA):
   - TS-INT-11: Doctor fix + concurrent resume
   - TS-INT-12: Rotation disk full
   - TS-INT-13: Doctor empty system
   - TS-INT-14: Doctor new session (UUID not in history)
   - Add to D3.3 test scenarios

### Should Fix (Strongly Recommended)

7. **Doctor fix action logging** (Security):
   - Log removed locks to migration.log
   - Format: `[timestamp] DOCTOR-FIX: Removed stale lock: <path>`
   - Add to D3.1 tasks

8. **Doctor scheduling guidance** (DevOps):
   - Recommended: Run doctor nightly (cron)
   - Alert integration examples (exit code check)
   - Add to Monitoring & Metrics section

9. **Doctor --dry-run flag** (User):
   - Show what --fix would do without doing it
   - Add to D3.1 flags

10. **Test data preparation section** (Go Dev):
    - How benchmarks create sessions
    - Mock strategy for tmux
    - Add new section to Testing Strategy

11. **Rotation partial failure handling** (Go Dev):
    - Document rollback strategy
    - Or accept best-effort (document)
    - Add to D3.2 error handling

12. **Doctor summary format for many sessions** (User):
    - Option to show summary only (not all checks)
    - Add to D3.1 output modes

---

## Recommendations for Revision

### New Sections to Add

1. **Test Data Preparation** (Testing Strategy):
   - Mock tmux strategy
   - Benchmark session creation
   - Fixture data structure

2. **Doctor Implementation Details** (D3.1):
   - History.jsonl parsing (streaming, cached)
   - UUID check optimization
   - Check execution order

3. **Fast vs Slow Tests** (Testing Strategy):
   - Fast: Unit + quick integration (< 2 min)
   - Slow: Load + stress + long-running (nightly)

### Updated Sections

**D3.1 Doctor Command**:
- Add history.jsonl parsing strategy
- Add UUID check optimization (cache parse)
- Add lock age check (< 60s = don't remove)
- Add --dry-run flag
- Add fix action logging
- Add summary format for many sessions

**D3.2 Log Rotation**:
- Add temp file permissions (0600)
- Clarify partial failure handling (best-effort)
- Add permission preservation during rename

**D3.3 Integration & Performance Testing**:
- Add TS-INT-11 through TS-INT-15
- Clarify test time budget (fast vs slow)
- Add test data preparation details
- Add BM-9 for rotation

**Monitoring & Metrics**:
- Add doctor scheduling recommendations
- Add alert integration examples
- Add log analysis guidance

---

## Next Steps

1. Create S3-SPRINT-PLAN-v2.md addressing all feedback
2. Run Round 2 review
3. Target score: ≥8.5/10

**Status**: ❌ REVISION NEEDED - Round 2 Review Required
