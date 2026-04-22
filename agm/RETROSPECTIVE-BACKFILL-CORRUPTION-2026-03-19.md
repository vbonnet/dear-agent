# Retrospective: Backfill Command Data Corruption Incident

**Date**: 2026-03-19
**Severity**: High (P1)
**Sessions Affected**: 17 sessions corrupted
**Recovery Status**: ✅ Complete

---

## Executive Summary

The `agm admin backfill-plan-sessions` command, designed to link parent-child sessions created by Claude Code's "Clear Context and Execute Plan" feature, created **massive data corruption** by incorrectly linking unrelated sessions based solely on timing proximity. This resulted in:

- 13 sessions renamed with bogus "-exec" suffixes
- 4 additional sessions linked with invalid parent relationships
- Session resume failures (wrong tmux sessions opened)
- Complete corruption of session hierarchy tracking

All corruption was successfully recovered by restoring original names from git history and clearing bogus parent links.

---

## Timeline

### 2026-03-13 - Initial Backfill Run
- Backfill command executed to fix "44 orphaned sessions"
- First run: 13 sessions backfilled
- Second run: 24 additional sessions backfilled
- **No validation performed** - assumed all links were legitimate

### 2026-03-19 - Discovery
1. User reported: `agm session resume tool-usage-compliance` opened wrong session
2. Investigation revealed:
   - "tool-usage-compliance-exec" had tmux name "three-tier-verification"
   - "tool-usage-compliance-exec-exec" had tmux name "memory-persistence"
   - Sessions with "-exec" suffixes were actually legitimate standalone sessions

### 2026-03-19 - Root Cause Analysis
- Queried database parent-child relationships
- Discovered false chains of 10+ sessions linked together
- Example chain:
  ```
  cognitive-mechanisms → cognitive-mechanisms-exec → hook-enforcement-exec →
  autonomous-swarm-exec → autonomous-swarm-exec-exec → ecphory-enhancements-exec →
  tool-usage-compliance-exec → tool-usage-compliance-exec-exec → memory-persistence-exec
  ```
- Compared against git history of YAML manifests
- Found all "-exec" renames were bogus

### 2026-03-19 - Recovery
1. Created recovery script to extract original names from git
2. Generated SQL to restore 13 corrupted sessions
3. Executed recovery: all sessions restored
4. Cleared 4 remaining bogus parent links
5. Verified: zero parent-child relationships remain

---

## Root Cause

### Backfill Algorithm Flaws

The backfill command used **dangerously permissive criteria**:

```go
// WRONG: Too permissive
if session.Name == "Unknown" && // Many sessions have this name
   timeDiff > 1*time.Second &&  // Way too broad
   timeDiff < 10*time.Second    // Expanded to 300s in fix!
```

**Problems:**

1. **No CWD matching**: Didn't verify sessions shared same working directory
2. **Timing-only heuristic**: 300-second window created false positives
3. **No name inheritance check**: Didn't verify child inherited parent's name
4. **Assumed all "Unknown" names**: Many legitimate sessions named "Unknown"
5. **No validation**: No checks against git history or manual review

### Why This Happened

1. **Incomplete specification**: Original plan didn't define strict validation criteria
2. **Over-optimization**: Broadened time window to catch "all orphans" without validation
3. **No dry-run verification**: Applied changes immediately without preview
4. **No rollback plan**: No git commit before destructive database changes
5. **Trust in automation**: Assumed backfill algorithm was correct without spot-checking

---

## Impact Assessment

### Data Corruption

**13 Sessions with Renamed Names:**
| Original Name           | Corrupted Name                  | Impact                          |
|------------------------|----------------------------------|----------------------------------|
| three-tier-verification| tool-usage-compliance-exec      | Resume opened wrong session      |
| memory-persistence     | tool-usage-compliance-exec-exec | Resume opened wrong session      |
| claude-code-validation | memory-persistence-exec         | Resume opened wrong session      |
| autonomous-swarm       | hook-enforcement-exec           | Resume opened wrong session      |
| ecphory-enhancements   | autonomous-swarm-exec-exec      | Resume opened wrong session      |
| hook-enforcement       | cognitive-mechanisms-exec       | Resume opened wrong session      |
| context-management     | research-cont-exec              | Resume opened wrong session      |
| astrocyte-improvements | agent-automation-exec           | Resume opened wrong session      |
| auditor-code           | auditor-logs-exec               | Resume opened wrong session      |
| auditor-logs           | scout-external-v2-exec          | Resume opened wrong session      |
| delete-broker          | delete-signing-exec             | Resume opened wrong session      |
| delete-cat             | delete-broker-exec              | Resume opened wrong session      |
| delete-signing         | delete-microvm-exec             | Resume opened wrong session      |

**4 Sessions with Bogus Parent Links:**
- autonomous-swarm-exec (time gap: -1888805s)
- ecphory-enhancements-exec (time gap: -1888929s)
- claude-code-validation-exec (time gap: -1888673s)
- agm-cortex-exec (time gap: -313270s)

### User Impact

- **Session Resume Failures**: Users couldn't reliably resume sessions by name
- **Data Integrity Loss**: Session metadata no longer trustworthy
- **Manual Recovery Required**: Users had to identify correct tmux sessions manually
- **Loss of Confidence**: Session management system appeared broken

---

## Recovery Steps

### 1. Created Recovery Script

`recover-corrupted-sessions.go`:
- Queried git history for original YAML manifests (commit 6fa5652b1~1)
- Compared database session names against git history
- Detected 13 name mismatches and 13 bogus parent links

### 2. Generated SQL

```sql
-- Example: Restore three-tier-verification
UPDATE agm_sessions
SET name = 'three-tier-verification', parent_session_id = NULL
WHERE id = '21261521-7b91-448d-bd93-8f3a501839b0';
```

### 3. Applied Recovery

```
✓ [ 1/13] Restored: astrocyte-improvements
✓ [ 2/13] Restored: auditor-code
✓ [ 3/13] Restored: auditor-logs
...
✓ [13/13] Restored: three-tier-verification
```

### 4. Cleared Remaining Bogus Links

```
✓ [1/4] Cleared parent link: autonomous-swarm-exec
✓ [2/4] Cleared parent link: ecphory-enhancements-exec
✓ [3/4] Cleared parent link: claude-code-validation-exec
✓ [4/4] Cleared parent link: agm-cortex-exec
```

### 5. Verification

- Queried parent-child relationships: **0 remaining**
- Tested session resume: `agm session resume three-tier-verification` → ✅ correct session
- All session names match tmux session names

---

## Lessons Learned

### What Went Well

1. **Git History Saved Us**: YAML manifests in git provided source of truth
2. **Fast Detection**: User reported issue immediately when noticed
3. **Systematic Recovery**: Recovery script validated all changes before applying
4. **Complete Recovery**: Zero data loss, all sessions restored

### What Went Wrong

1. **No Dry-Run Mode**: Backfill applied changes without preview/confirmation
2. **Insufficient Validation**: Time-based heuristic too weak for production use
3. **No Rollback Strategy**: No database backup or git commit before changes
4. **Overconfident Automation**: Trusted algorithm without spot-checking results
5. **No Monitoring**: Corruption went unnoticed for 6 days until manual testing

### What We Should Do Differently

1. **Always Require Dry-Run First**: Preview changes, get user confirmation
2. **Strict Validation Criteria**: Multiple signals required (CWD, timing, name inheritance)
3. **Automated Rollback**: Git commit + Dolt branch before destructive operations
4. **Post-Execution Validation**: Verify changes against expected patterns
5. **Monitoring & Alerts**: Detect anomalies (e.g., mass renaming to "-exec")

---

## Action Items

### Immediate (P0)

- [x] Restore corrupted session names from git history
- [x] Clear all bogus parent_session_id links
- [x] Verify session resume works correctly
- [x] Document incident in retrospective

### Short-term (P1)

- [ ] Add `--dry-run` flag to backfill command (shows changes without applying)
- [ ] Require explicit `--apply` flag for destructive operations
- [ ] Add git commit before database modifications
- [ ] Strengthen backfill validation criteria:
  - Require CWD match between parent/child
  - Require timing within 1-30 seconds (not 300)
  - Require name inheritance pattern (parent name → parent-exec)
  - Require "Unknown" child name OR exact match to Claude Code pattern
- [ ] Add post-backfill validation (detect unexpected patterns)

### Long-term (P2)

- [ ] Add database backup/restore commands
- [ ] Implement Dolt branching for experimental changes
- [ ] Add session integrity monitoring
- [ ] Create automated regression tests for backfill
- [ ] Document session hierarchy detection algorithm
- [ ] Add `agm admin validate-hierarchy` command

---

## Prevention Strategy

### Enhanced Backfill Algorithm

```go
// CORRECT: Strict validation
func isLegitPlanModeSplit(parent, child *Session) bool {
    // 1. Time window: 1-30 seconds (not 300!)
    timeDiff := child.CreatedAt.Sub(parent.UpdatedAt)
    if timeDiff < time.Second || timeDiff > 30*time.Second {
        return false
    }

    // 2. CWD must match
    if parent.Context.Project != child.Context.Project {
        return false
    }

    // 3. Child name follows pattern
    expectedChildName := parent.Name + "-exec"
    if child.Name != "Unknown" && child.Name != expectedChildName {
        return false
    }

    // 4. Different UUIDs (separate conversations)
    if parent.Claude.UUID == child.Claude.UUID {
        return false
    }

    // 5. Child didn't exist in git history (was truly created by plan mode)
    if existsInGitHistory(child.Name) {
        return false
    }

    return true
}
```

### Required Workflow

```bash
# 1. Dry-run first (preview changes)
agm admin backfill-plan-sessions --dry-run

# 2. Review output, verify each link makes sense

# 3. Apply only if confident
agm admin backfill-plan-sessions --apply

# 4. Verify changes
agm admin validate-hierarchy
```

### Monitoring

```sql
-- Detect mass renaming anomalies
SELECT COUNT(*) FROM agm_sessions
WHERE name LIKE '%-exec%'
  AND created_at < NOW() - INTERVAL 7 DAY;

-- Alert if >5 sessions renamed in last hour
```

---

## Testing Plan

### Regression Tests

1. **False Positive Detection**:
   - Create two unrelated sessions 10 seconds apart
   - Run backfill → should NOT link them
   - Verify: zero parent links created

2. **Legitimate Plan Split**:
   - Create planning session "test-parent"
   - Wait 5 seconds
   - Create execution session "test-parent-exec" with different UUID
   - Run backfill → SHOULD link them
   - Verify: parent_session_id set correctly

3. **Edge Cases**:
   - Same CWD, different timing (outside 1-30s window) → no link
   - Same timing, different CWD → no link
   - Same UUID (not a plan split) → no link
   - Child existed in git history → no link

### Integration Tests

1. Test `agm session resume` with restored sessions
2. Test `agm session archive` with cleared parent links
3. Test `agm session list` shows correct names
4. Verify all tmux session names match AGM session names

---

## Related Commits

- **Recovery**: `960b29c` - fix(agm): resolve schema mismatch in ResolveIdentifier query
- **Documentation**: `6bd399c` - docs(agm): Add session hierarchy architecture documentation
- **Original Backfill**: `00a840c` - fix(agm): improve backfill detection (TOO PERMISSIVE)

---

## Conclusion

This incident demonstrated the critical importance of:

1. **Validation before automation** - algorithms need multiple verification signals
2. **Dry-run modes** - preview destructive changes before applying
3. **Git as source of truth** - version control saved us from data loss
4. **Systematic recovery** - scripts validate before fixing
5. **Retrospectives** - document learnings to prevent recurrence

The backfill feature is valuable for maintaining session continuity, but requires **strict validation criteria** and **manual review** before applying changes. This incident has been fully recovered with zero data loss, and new safeguards will prevent similar issues in the future.

---

**Recovery Artifacts**:
- Recovery script: `recover-corrupted-sessions.go`
- Recovery SQL: `/tmp/recovery-session-corruption.sql`
- Validation script: `verify-legitimate-parents.go`
- Clear script: `clear-remaining-bogus-parents.go`
