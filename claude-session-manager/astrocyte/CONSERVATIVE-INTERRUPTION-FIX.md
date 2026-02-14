# Conservative Interruption Policy - Fix Summary

**Date**: 2026-02-14
**Issue**: Astrocyte daemon over-eagerly interrupting sessions waiting for user input (AskUserQuestion prompts)
**Status**: FIXED ✅

## Problem

The astrocyte daemon was interrupting legitimate sessions that were:
- Waiting for AskUserQuestion responses
- At planning endpoints waiting for user approval
- Generally idle at conversation completion points

**Root Cause**: Missing endpoint detection check in `astrocyte.py` main loop (present in `astrocyte-daemon.py` but not in main script).

## Philosophy

**NEW POLICY**: "Better to miss an interruption than interrupt legitimate work"

- **Default**: Do NOT interrupt
- **Only interrupt for**: Genuine freezes (0 tokens bug, UI completely unresponsive)
- **Never interrupt**: Sessions waiting for user input, planning sessions, natural endpoints

## Changes Made

### 1. Added Endpoint Detection Check (astrocyte.py:2358-2367)

```python
if stuck:
    # CONSERVATIVE INTERRUPTION: Check if session is at a natural endpoint
    # Only interrupt if genuinely frozen (0 tokens bug, UI unresponsive)
    # Default: do not interrupt - better to miss than interrupt legitimate work
    if is_conversation_endpoint_idle(current):
        print(f"\n✓ ENDPOINT DETECTED: {session}")
        print(f"   Session at natural conversation endpoint (waiting for user input)")
        print(f"   Symptom detected: {symptom} (but endpoint signals present)")
        print(f"   Skipping recovery - not a genuine hang")
        print(f"   Rationale: Session may be waiting for AskUserQuestion response")
        continue  # Skip to next session - DO NOT INTERRUPT
```

**Effect**: Sessions at endpoints (completion language + idle prompt + no spinners) are never interrupted.

### 2. Made Detection Thresholds More Conservative

**Before → After**:

| Threshold | Old Value | New Value | Rationale |
|-----------|-----------|-----------|-----------|
| mustering_timeout | 10 min | 20 min | Only genuine hangs, not normal processing |
| zero_token_waiting | 3 min | 15 min | More time before assuming 0 tokens bug |
| cursor_frozen | 15 min | 30 min | UI freeze only, not user thinking time |
| permission_prompt_duration | 5 min | 10 min | More time for user to respond |
| orchestrator_cursor_frozen | 15 min | 30 min | Orchestrators monitor multiple sessions |
| single_task_cursor_frozen | 5 min | 15 min | Single tasks can think longer |
| interactive_cursor_frozen | 3 min | 10 min | Interactive needs user thinking time |

**Files Updated**:
- `astrocyte.py`: Config class defaults (lines 92-107)
- `astrocyte.py`: load_config() fallback values (lines 231-239)
- `config.example.yaml`: Recommended thresholds (lines 7-14)

### 3. Enhanced Completion Language Patterns

**Added patterns** to detect more completion/waiting-for-user states:

```python
# Just the checkmark itself
r"✅",
r"✓",

# All/everything done
r"All.*done",
r"All.*finished",

# Session/job complete
r"Session.*complete",
r"Session.*done",

# Simple completion words
r"\bComplete\b",
r"\bFinished\b",
r"\bDone\b",

# User questions (AskUserQuestion detection)
r"Which.*\?",  # "Which would you prefer?", "Which approach?"
r"What.*\?",   # "What should I...", "What would you..."
r"Should I",   # "Should I proceed?"
r"Would you like",
r"Do you want",
r"Your.*preference",
r"Your.*choice",
```

**Effect**: Better detection of sessions waiting for user input.

### 4. Comprehensive Test Coverage

**New Test Files**:
1. `tests/test_endpoint_detection.py` (437 lines, 43 tests)
   - Unit tests for endpoint detection logic
   - Tests for completion language, idle prompt, spinner detection
   - Conservative interruption integration tests
   - Edge case handling

2. `features/conservative_interruption.feature` (BDD scenarios)
   - 14 scenarios covering conservative interruption policy
   - Examples: AskUserQuestion not interrupted, planning sessions safe
   - Genuine freezes (0 tokens, mustering, UI freeze) are interrupted

3. `features/test_bdd_steps.py` (356 new lines)
   - Step implementations for BDD tests
   - GIVEN/WHEN/THEN steps for conservative interruption

**Test Results**: All 43 endpoint detection tests passing ✅

### 5. Integration Tests Added

**In `tests/test_recovery_integration.py`**:
- `TestConservativeDetectionIntegration` class (207 lines)
- 7 integration tests covering:
  - AskUserQuestion sessions not interrupted
  - Planning sessions not interrupted
  - Genuine freezes (mustering, 0 tokens) are interrupted
  - Multiple detection cycles with consistent results
  - Partial endpoint signals handled correctly

## Detection Flow

**NEW FLOW**:
```
1. Stuck symptom detected (mustering, 0 tokens, cursor frozen)?
   └─ YES → Check endpoint detection

2. Is session at conversation endpoint?
   ├─ Completion language present? (✅, "Done", "Which?", etc.)
   ├─ Idle prompt visible? (❯)
   ├─ No pending tool calls? (no spinners)
   └─ All 3 conditions TRUE?
       ├─ YES → SKIP RECOVERY (endpoint detected)
       └─ NO → PROCEED WITH RECOVERY (genuine hang)
```

## Endpoint Detection Signals

A session is at an **endpoint** (should NOT be interrupted) if ALL of these are true:

1. ✅ **Completion language** visible
   - Task done phrases: "✅ complete", "All done", "Finished"
   - User questions: "Which?", "What?", "Should I?"
   - Ready state: "Ready to proceed", "Waiting for input"

2. ✅ **Idle prompt** visible
   - `❯` character at end of pane

3. ✅ **No pending tool calls**
   - No spinner patterns: "✶ Thinking", "✻ Mustering", "Galloping"

**If ANY condition fails** → NOT an endpoint → eligible for interruption

## What Gets Interrupted

**ONLY interrupt these genuinely stuck states**:

1. **0 tokens bug**: `↓ 0 tokens` for >15 minutes
2. **Mustering freeze**: `✻ Mustering...` for >20 minutes (unchanged in consecutive checks)
3. **UI completely frozen**: Cursor unchanged + no output + no completion language for >30 minutes
4. **Permission prompts**: Stuck on bash permission prompt for >10 minutes

**NEVER interrupt**:
- AskUserQuestion prompts
- Planning sessions at completion
- Sessions with completion language + idle prompt
- User thinking time (idle with partial signals)

## Examples

### ✅ NOT Interrupted (Endpoint Detected)

**AskUserQuestion Session**:
```
● Which authentication approach fits best?

A) Passport.js (enhance existing)
B) Auth0 (commercial service)
C) Build from scratch

❯
```
- Has completion language: "Which?" pattern
- Has idle prompt: `❯`
- No spinners
- **Endpoint detected** → NOT interrupted

**Planning Session**:
```
● ✅ Plan finalized

Ready for your approval.

❯
```
- Has completion language: "✅", "Ready for"
- Has idle prompt: `❯`
- No spinners
- **Endpoint detected** → NOT interrupted

### ❌ WILL Be Interrupted (NOT Endpoint)

**0 Tokens Bug**:
```
● Bootstrapping…

Bootstrapping… (esc to interrupt · 15m 32s · ↓ 0 tokens)

```
- NO completion language
- NO idle prompt
- **NOT endpoint** → Interrupted after 15 min

**Mustering Freeze**:
```
● Processing task...

✻ Mustering...

```
- NO completion language
- NO idle prompt
- **NOT endpoint** → Interrupted after 20 min

## Files Changed

| File | Changes | Lines |
|------|---------|-------|
| `astrocyte.py` | Added endpoint check, conservative thresholds, enhanced patterns | +11, ~20 modified |
| `config.example.yaml` | Updated recommended thresholds | ~7 lines modified |
| `tests/test_endpoint_detection.py` | New unit tests | +437 (new file) |
| `features/conservative_interruption.feature` | New BDD scenarios | +207 (new file) |
| `features/test_bdd_steps.py` | BDD step implementations | +356 |
| `tests/test_recovery_integration.py` | Integration tests | +207 |

**Total**: ~1,245 lines added/modified

## Verification

**Run tests**:
```bash
# Unit tests
pytest tests/test_endpoint_detection.py -v
# All 43 tests passing ✅

# Integration tests
pytest tests/test_recovery_integration.py::TestConservativeDetectionIntegration -v
# All 7 tests passing ✅

# BDD tests
pytest features/test_bdd_steps.py -v
# Conservative interruption scenarios passing ✅
```

## Incident Log Examples

**Before (Over-Eager)**:
```json
{
  "symptom": "stuck_cursor_frozen",
  "session_name": "planning-session",
  "recovery_attempted": true,
  "recovery_method": "escape"
}
```
**Issue**: Interrupted planning session waiting for user approval

**After (Conservative)**:
```
✓ ENDPOINT DETECTED: planning-session
   Session at natural conversation endpoint (waiting for user input)
   Symptom detected: stuck_cursor_frozen (but endpoint signals present)
   Skipping recovery - not a genuine hang
   Rationale: Session may be waiting for AskUserQuestion response
```
**Fixed**: Endpoint detected, no interruption

## Migration Guide

**For existing astrocyte users**:

1. **Update config** (`~/.agm/astrocyte/config.yaml`):
   ```yaml
   thresholds:
     mustering_timeout: 20      # Was 10
     zero_token_waiting: 15     # Was 3
     cursor_frozen: 30          # Was 15
     permission_prompt_duration: 10  # Was 5
   ```

2. **Restart daemon**:
   ```bash
   systemctl --user restart astrocyte
   ```

3. **Verify behavior**:
   - Check `~/.agm/astrocyte/incidents.jsonl` for "ENDPOINT DETECTED" logs
   - Sessions with AskUserQuestion should show endpoint detection

## References

- **Issue**: Session `astrocyte-overeager` being interrupted during AskUserQuestion
- **Design**: Conservative interruption philosophy ("better to miss than interrupt")
- **Tests**: `tests/test_endpoint_detection.py`, `features/conservative_interruption.feature`
- **Docs**: This file, `config.example.yaml`, test docstrings

---

**Status**: COMPLETE ✅
**Tested**: 43 unit + 7 integration + 14 BDD scenarios passing
**Impact**: Eliminates false positive interruptions while still catching genuine freezes
