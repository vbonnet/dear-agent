# Deep Research E2E Test Plan

## Overview

End-to-end testing for the deep-research workflow integration with AGM.

**Bead**: oss-agm-e2e-testing (Phase 6)
**Duration**: 2-3 hours
**Status**: In Progress

---

## Success Criteria

The E2E test must verify all 5 expected behaviors from the ROADMAP:

### 1. ✅ Starts 3 parallel research workflows (one per video)

**Test**: Verify goroutines launched for each URL
**Evidence**: Console output shows `[1/3] Starting research: URL1`, `[2/3]...`, `[3/3]...`
**Pass Criteria**: All 3 URLs shown as starting research

### 2. ✅ Creates 3 research documents

**Test**: Verify research report files created
**Evidence**: 3 files exist in `./research/` directory
**Pass Criteria**: Each URL has corresponding `report.md` file

### 3. ✅ Applies research insights to engram/ai-tools repos

**Test**: Verify ResearchApplicator executed
**Evidence**: Console output shows "Applying research insights..."
**Pass Criteria**: ApplicationResult generated with proposals

### 4. ✅ Generates categorized improvement proposals

**Test**: Verify proposals file created and categorized
**Evidence**: `research-proposals.md` exists with sections for engram and ai-tools
**Pass Criteria**: File contains proposals categorized by repository

### 5. ✅ Logs all results to markdown (crash resilient)

**Test**: Verify log file created and updated incrementally
**Evidence**: `research-{sessionID}-log.md` exists with progress tracking
**Pass Criteria**: Log shows all 3 URLs with completion status

---

## Test Scenarios

### Scenario 1: Happy Path (3 URLs, all succeed)

**Command**:
```bash
agmnew research-test-happy --harness=gemini-cli --workflow=deep-research \
  --prompt="Please research https://www.youtube.com/watch?v=WEEKBlQfGt8, \
  https://www.youtube.com/watch?v=4_2j5wgt_ds, and \
  https://www.youtube.com/watch?v=eT_6uaHNlk8 and come up with ideas we can \
  test on how they can be used to improve the engram and ai-tools repos"
```

**Expected Duration**: 15-30 minutes (3 videos × 5-10 min each)

**Expected Artifacts**:
- 3 research reports: `./research/*/report.md`
- 1 proposals file: `research-proposals.md`
- 1 log file: `research-{sessionID}-log.md`

**Expected Console Output**:
```
[1/3] Starting research: https://www.youtube.com/watch?v=WEEKBlQfGt8
[2/3] Starting research: https://www.youtube.com/watch?v=4_2j5wgt_ds
[3/3] Starting research: https://www.youtube.com/watch?v=eT_6uaHNlk8
✓ Research completed: https://www.youtube.com/watch?v=WEEKBlQfGt8
✓ Research completed: https://www.youtube.com/watch?v=4_2j5wgt_ds
✓ Research completed: https://www.youtube.com/watch?v=eT_6uaHNlk8
✓ Proposals written to: research-proposals.md
✓ Proposals added to log: research-{sessionID}-log.md
```

**Validation Steps**:
1. Check log file exists and contains all 3 URLs
2. Verify each URL marked as `[x]` (completed) in progress section
3. Verify proposals section populated with engram and ai-tools sections
4. Verify all 3 research report files exist
5. Verify proposals file exists and is well-formed

---

### Scenario 2: Resume After Crash (partial completion)

**Setup**: Simulate crash after 2 URLs complete

**Steps**:
1. Run test with 3 URLs
2. After 2 URLs complete, kill the process (Ctrl+C)
3. Re-run same command
4. Verify:
   - Console shows: "⏭️ Resuming session - skipping 2 already-completed URLs"
   - Only URL 3 is researched
   - Final log shows all 3 URLs complete
   - Proposals generated after resume

**Expected Console Output**:
```
⏭️ Resuming session - skipping 2 already-completed URLs
   ✓ https://www.youtube.com/watch?v=WEEKBlQfGt8 (already complete)
   ✓ https://www.youtube.com/watch?v=4_2j5wgt_ds (already complete)
[1/3] Starting research: https://www.youtube.com/watch?v=eT_6uaHNlk8
✓ Research completed: https://www.youtube.com/watch?v=eT_6uaHNlk8
✓ Proposals written to: research-proposals.md
✓ Proposals added to log: research-{sessionID}-log.md
```

**Validation Steps**:
1. Verify log shows 2 URLs already complete before resume
2. Verify only 1 new research executed
3. Verify final log shows all 3 URLs complete
4. Verify proposals generated correctly

---

### Scenario 3: Partial Failure (1 URL fails)

**Setup**: Use 2 valid URLs + 1 invalid URL

**Command**:
```bash
agmnew research-test-failure --harness=gemini-cli --workflow=deep-research \
  --prompt="Research https://www.youtube.com/watch?v=WEEKBlQfGt8, \
  https://invalid-url-12345.fake, and \
  https://www.youtube.com/watch?v=4_2j5wgt_ds"
```

**Expected Behavior**:
- Research continues despite 1 failure
- Proposals generated from 2 successful results
- Log shows 2 completed, 1 failed

**Expected Console Output**:
```
[1/3] Starting research: https://www.youtube.com/watch?v=WEEKBlQfGt8
[2/3] Starting research: https://invalid-url-12345.fake
[3/3] Starting research: https://www.youtube.com/watch?v=4_2j5wgt_ds
✓ Research completed: https://www.youtube.com/watch?v=WEEKBlQfGt8
✗ Research failed: https://invalid-url-12345.fake (error: ...)
✓ Research completed: https://www.youtube.com/watch?v=4_2j5wgt_ds
Researched 2/3 URLs successfully (1 failed)
✓ Proposals written to: research-proposals.md
```

**Validation Steps**:
1. Verify log shows 2 URLs ✅ Complete, 1 URL ❌ Failed
2. Verify proposals generated from 2 successful results
3. Verify error details included in log
4. Verify metadata shows urls_failed: 1

---

## Test Execution

### Quick Test (Recommended for CI)

For faster testing, use shorter content URLs instead of long videos:

**Command**:
```bash
agmnew research-test-quick --harness=gemini-cli --workflow=deep-research \
  --prompt="Research https://arxiv.org/abs/1706.03762, \
  https://arxiv.org/abs/1810.04805, and \
  https://arxiv.org/abs/2005.14165"
```

**Duration**: ~5-10 minutes (ArXiv papers are shorter)

**Artifacts**: Same as happy path but faster

---

### Full Test (Production Validation)

Use the original 3 YouTube video URLs for full production validation:

**Command**: See Scenario 1 above

**Duration**: 15-30 minutes

**Use Case**: Final validation before release

---

## Validation Checklist

After running E2E test, verify:

- [ ] **Parallel Execution**: Console shows all 3 URLs starting simultaneously
- [ ] **Research Reports**: 3 report files created in research directory
- [ ] **Log File**: Crash-resilient log created with correct format
- [ ] **Progress Tracking**: Log shows - [x] for completed URLs
- [ ] **Proposals File**: Separate proposals markdown created
- [ ] **Proposals in Log**: Proposals section populated in log file
- [ ] **Categorization**: Proposals split by engram/ai-tools
- [ ] **Metadata**: Workflow result metadata includes all expected fields
- [ ] **Resume Logic**: (Scenario 2) Correctly resumes from partial completion
- [ ] **Failure Handling**: (Scenario 3) Continues despite URL failure

---

## Test Automation Script

Create `scripts/test-deep-research-e2e.sh`:

```bash
#!/bin/bash
set -e

echo "=== Deep Research E2E Test ==="
echo ""

# Test 1: Quick test with ArXiv papers
echo "Test 1: Quick test (3 ArXiv papers)"
echo "-----------------------------------"

SESSION_NAME="research-test-$(date +%s)"

agmnew "$SESSION_NAME" --harness=gemini-cli --workflow=deep-research \
  --prompt="Research https://arxiv.org/abs/1706.03762, \
  https://arxiv.org/abs/1810.04805, and \
  https://arxiv.org/abs/2005.14165 and come up with ideas for \
  improving engram and ai-tools repos"

# Validate artifacts
echo ""
echo "Validating artifacts..."

# Check for log file
LOG_FILE=$(find . -name "research-*-log.md" -type f | head -1)
if [ -z "$LOG_FILE" ]; then
  echo "❌ FAIL: Log file not found"
  exit 1
fi
echo "✓ Log file found: $LOG_FILE"

# Check for proposals file
if [ ! -f "research-proposals.md" ]; then
  echo "❌ FAIL: Proposals file not found"
  exit 1
fi
echo "✓ Proposals file found: research-proposals.md"

# Check log file format
if ! grep -q "## Progress" "$LOG_FILE"; then
  echo "❌ FAIL: Log file missing Progress section"
  exit 1
fi
echo "✓ Log file has Progress section"

# Check for completed URLs
COMPLETED_COUNT=$(grep -c "\- \[x\]" "$LOG_FILE" || true)
if [ "$COMPLETED_COUNT" -ne 3 ]; then
  echo "❌ FAIL: Expected 3 completed URLs, found $COMPLETED_COUNT"
  exit 1
fi
echo "✓ All 3 URLs marked complete in log"

# Check proposals categorization
if ! grep -q "## engram Proposals" "research-proposals.md"; then
  echo "❌ FAIL: Proposals missing engram section"
  exit 1
fi
if ! grep -q "## ai-tools Proposals" "research-proposals.md"; then
  echo "❌ FAIL: Proposals missing ai-tools section"
  exit 1
fi
echo "✓ Proposals categorized by repository"

echo ""
echo "=== ✅ E2E Test PASSED ==="
```

---

## Manual Test Procedure

If automated script not available, run manual test:

1. **Start Test**:
   ```bash
   agmnew research-manual-test --harness=gemini-cli --workflow=deep-research \
     --prompt="Research https://arxiv.org/abs/1706.03762, \
     https://arxiv.org/abs/1810.04805, and \
     https://arxiv.org/abs/2005.14165"
   ```

2. **Monitor Console Output**:
   - Watch for `[1/3]`, `[2/3]`, `[3/3]` starting messages
   - Watch for `✓ Research completed` messages
   - Watch for proposals generation messages

3. **Validate Artifacts**:
   ```bash
   # Find log file
   find . -name "research-*-log.md" -type f

   # Check log content
   cat research-*-log.md

   # Verify proposals
   cat research-proposals.md
   ```

4. **Test Resume Logic** (optional):
   - After 1 URL completes, Ctrl+C to kill
   - Re-run same command
   - Verify resume message appears
   - Verify only remaining URLs researched

---

## Known Issues / Limitations

1. **Timeout**: Gemini Deep Research can take 5-10 minutes per URL
   - Mitigation: Use shorter content (ArXiv papers vs YouTube videos) for testing

2. **Rate Limits**: Gemini API has rate limits
   - Mitigation: Space out test runs, avoid parallel E2E tests

3. **Cache**: gemini-dr caches results, may not re-research same URL
   - Mitigation: Use different URLs for each test run

4. **Environment**: Requires GOOGLE_API_KEY and GCP_PROJECT_ID
   - Mitigation: Document environment setup in README

---

## Success Metrics

Test is successful if:

- ✅ All 5 success criteria validated
- ✅ All validation checklist items pass
- ✅ Automated script exits with code 0
- ✅ Artifacts conform to expected format
- ✅ Resume logic works as expected
- ✅ Failure handling works as expected

---

## Documentation

After E2E test passes, document:

1. **Test Results**: Capture console output, artifacts, timing
2. **Known Issues**: Any bugs found during testing
3. **Recommendations**: Improvements for future iterations
4. **Sign-Off**: Mark bead complete and update ROADMAP

---

**Created**: 2026-02-03
**Bead**: oss-agm-e2e-testing
**Phase**: 6 (Validation)
