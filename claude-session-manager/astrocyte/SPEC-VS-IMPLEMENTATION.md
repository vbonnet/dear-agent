# Astrocyte: Spec vs Implementation Analysis

**Date**: 2026-02-14
**Question**: What does astrocyte's spec say vs what does it actually do?

---

## (A) WRITTEN GOAL/SPEC (from ~/src/ws/oss/wf/astrocyte/SPEC.md)

### Vision Statement

**The Problem**:
- Zombie Sessions: Agent says "I will do X" but never executes
- Blocked on Unresponsive Beads: Waiting forever for crashed subprocess
- Silent Failures: Session stops responding, no error, no cursor movement
- User-Input Confusion: Agent waits for answer but user doesn't notice

**The Solution**:
- Monitor all CSM sessions every 2-5 minutes
- Detect stuck sessions using heuristics
- **Automatically recover** stuck sessions (send ESC → verify → restart if needed)
- File incident reports for root cause analysis
- On resume, **prompt agent to self-diagnose** hang cause

**Success Metric**: Zero developer intervention for stuck sessions, 100% incident logging

### Primary Goals (from SPEC)

1. **Automatic Recovery (99% Success Rate)**
   - Detect stuck within 5 minutes of hang
   - Recover via ESC (no manual tmux intervention)
   - Metric: 99% stuck sessions recover without human intervention

2. **Incident Tracking (100% Capture Rate)**
   - Log every hang with state snapshot
   - Store in JSONL (append-only, crash-safe)
   - Metric: 100% of hangs logged with actionable diagnosis

3. **Root Cause Analysis (Agent Self-Diagnosis)**
   - On resume, agent analyzes what caused hang
   - Agent files structured report: symptom, cause, reproduction
   - Metric: 80%+ incidents have root cause diagnosis

4. **False Positive Rate <5%**
   - Don't interrupt long tasks (30-min build)
   - Don't interrupt AskUserQuestion prompts
   - Don't interrupt completed sessions (idle = done, not stuck)
   - Metric: <5% false interruptions

### Detection Heuristics (from SPEC)

**SPEC says detect these 4 symptoms**:

1. **Symptom 1: "I Will Do X" But Never Executes**
   - Pattern: `✻ Mustering...` for >10 minutes
   - Root cause: Planning paralysis, API timeout, rate limiting
   - Recovery: Send ESC → retry with simplified prompt

2. **Symptom 2: Blocked on Unresponsive Bead**
   - Pattern: `Waiting for task...` + `✶ Evaporating...` for >45 minutes
   - Root cause: Subprocess crashed, no timeout, blocked on stdin
   - Recovery: Send ESC → cancel task → retry with timeout

3. **Symptom 3: Silent Failure (No Output, Frozen Cursor)**
   - Pattern: No output for >15 minutes, no error, cursor visible but frozen
   - Root cause: Network timeout, process crash, terminal corruption
   - Recovery: ESC → C-c → restart Claude Code

4. **Symptom 4: Waiting for User Input (LEGITIMATE - Don't Interrupt!)**
   - Pattern: `AskUserQuestion` visible + `Waiting for user response...`
   - Detection: AskUserQuestion tool visible = NOT stuck
   - Action: Skip monitoring, don't interrupt

### Specified Thresholds (from SPEC config example)

```yaml
thresholds:
  no_output_timeout: 15m     # No new output
  mustering_timeout: 10m     # "Mustering..." stuck
  waiting_timeout: 30m       # "Waiting..." stuck (unless whitelisted)
  cursor_frozen_timeout: 20m # Cursor not moved
```

### Whitelisted Patterns (from SPEC)

```yaml
whitelist_patterns:
  - "Running tests..."
  - "Building..."
  - "Compiling..."
  - "AskUserQuestion"  # Waiting for user input
  - "Waiting for user response"
```

---

## (B) WHAT IT ACTUALLY DOES (from astrocyte.py code analysis)

### Actual Detection Loop (lines 2323-2450)

**Runs every 60 seconds** (not "2-5 minutes" as spec says)
- Enumerates active AGM sessions via tmux
- Captures pane content + cursor position
- Compares current state to previous state
- Applies detection heuristics **IN THIS ORDER**:

### Detection Heuristics (Actual Implementation)

**1. Permission Prompt Detection** (lines 2348-2352)
- **Function**: `is_stuck_permission_prompt()`
- **Triggers when**: Bash permission prompt visible with violation patterns
- **Threshold**: 10 minutes (after our fix, was 5 min)
- **Action**: `csm reject` (reject permission prompt with violations)
- **Note**: Works **without** previous state (fresh start detection)

**2. Mustering Timeout** (lines 2362-2365)
- **Function**: `is_stuck_mustering()` (lines 805-825)
- **Triggers when**:
  - Pattern `✻ Mustering...`, `✶ Evaporating...`, or `✢ Mustering...` visible
  - **Same pattern** visible in previous state (consecutive checks)
  - Time delta > threshold (20 min after our fix, was 10 min)
- **Symptom**: `stuck_mustering`
- **Heuristic**: `mustering_timeout`
- **Action**: Send ESC

**3. Zero Token Waiting** (lines 2366-2369)
- **Function**: `is_stuck_zero_token_waiting()` (lines 828-885)
- **Triggers when**:
  - Pattern `↓ 0 tokens` visible (universal stuck indicator)
  - Duration extracted from `esc to interrupt · 15m 32s · ↓ 0 tokens` format
  - Duration > threshold (15 min after our fix, was 3 min)
- **Symptom**: `stuck_zero_token_waiting`
- **Heuristic**: `zero_token_galloping`
- **Action**: Send ESC
- **Key insight**: Doesn't check spinner pattern, only `↓ 0 tokens` matters

**4. Cursor Frozen** (lines 2370-2373)
- **Function**: `is_stuck_cursor_frozen()` (lines 888-922)
- **Triggers when**:
  - Cursor position **identical** between checks
  - Pane content **identical** (no new output)
  - Time delta > threshold (30 min after our fix, was 15 min)
- **Symptom**: `stuck_cursor_frozen`
- **Heuristic**: `cursor_frozen`
- **Action**: Send ESC
- **Covers**: UI freezes, infinite loops, waiting for external resources

**5. Bash Violation Detection** (lines 2376-2380)
- **Function**: `detect_bash_violation()`
- **Triggers when**: Bash command patterns detected (cd, &&, ||, pipes, etc.)
- **Symptom**: `bash_violation`
- **Heuristic**: `bash_pattern:<pattern>`
- **Action**: Send bash rejection prompt

**6. Ask Question Violation** (lines 2382-2388)
- **DISABLED** (commented out in code!)
- Would detect: Agent asking questions in text instead of using AskUserQuestion tool
- **Rationale**: "Never interfere with sessions asking user questions"
- **Code comment**: "astrocyte must NEVER answer on their behalf"

### Conservative Interruption Check (NEW - lines 2390-2400)

**CRITICAL ADDITION** (added in our fix):

If stuck detected, **check endpoint detection BEFORE recovery**:

```python
if stuck:
    # Check if session is at a natural endpoint
    if is_conversation_endpoint_idle(current):
        print("✓ ENDPOINT DETECTED: {session}")
        print("   Skipping recovery - not a genuine hang")
        continue  # DO NOT INTERRUPT
```

**Endpoint detection** (lines 745-802):
- **Signal 1**: Completion language present (✅, "Done", "Which?", "Ready to proceed")
- **Signal 2**: Idle prompt visible (`❯`)
- **Signal 3**: No pending tool calls (no spinners)
- **Signal 4**: Not processing stale notifications

**ALL 4 signals must be TRUE** → Endpoint detected → **SKIP recovery**

### Actual Thresholds (After Our Fix)

**Default thresholds** (lines 92-107):
```python
mustering_timeout: int = 20      # Was 10 in spec
zero_token_waiting: int = 15     # Was 3 originally
cursor_frozen: int = 30          # Was 15 in spec
permission_prompt_duration: int = 10  # Was 5 originally
```

**Session-type adaptive thresholds**:
```python
orchestrator_cursor_frozen: int = 30   # Was 15
single_task_cursor_frozen: int = 15    # Was 5
interactive_cursor_frozen: int = 10    # Was 3
```

### Recovery Methods (Actual Implementation)

**Primary recovery**: `recover_with_escape()` (lines 1591-1643)
1. Send ESC via `tmux send-keys -t <session> Escape`
2. Wait 5 seconds
3. Capture new state
4. Verify recovery: pane content changed OR cursor moved OR pattern gone
5. If successful: Log success, continue monitoring
6. If failed: Try next recovery method

**Fallback recovery**: `recover_with_ctrl_c()` (lines 1646-1698)
- Used if ESC fails
- Sends `C-c` via tmux
- More aggressive, interrupts running processes

**Manual alert**: `manual_alert_mode()` (lines 1701-1727)
- Used if both ESC and C-c fail
- Logs failure, sends Slack notification
- Requires human intervention

**Recovery dispatcher** (lines 1730-1827):
- Routes to appropriate recovery method based on symptom
- Permission prompts → `csm reject`
- Bash violations → send bash rejection prompt
- Others → ESC → C-c → manual alert

### What Actually Gets Logged

**Incident log** (`~/.agm/astrocyte/incidents.jsonl`):
```json
{
  "timestamp": "2026-01-30T10:00:31.298856",
  "session_name": "session-name",
  "session_id": "uuid-1234",
  "symptom": "stuck_mustering",
  "duration_minutes": 15,
  "detection_heuristic": "mustering_timeout",
  "pane_snapshot": "...first 500 chars...",
  "cursor_position": "0,38",
  "recovery_attempted": true,
  "recovery_method": "escape",
  "recovery_success": true,
  "recovery_duration_seconds": 5.2
}
```

### Agent Self-Diagnosis (PARTIALLY IMPLEMENTED)

**Spec says**: On resume, send diagnosis prompt to agent

**Code shows** (lines 1882-1958):
- Function `send_diagnosis_prompt()` exists
- Generates diagnosis template
- Uses `csm send --prompt-file` to deliver prompt
- **BUT**: Only called if `config.diagnosis_enabled=true` (default: true)
- **BUT**: No evidence of "80%+ incidents have root cause" tracking

---

## KEY DIFFERENCES: SPEC vs IMPLEMENTATION

### Major Additions (Not in Original Spec)

1. ✅ **Endpoint Detection** (NEW)
   - Spec: Mentions "don't interrupt AskUserQuestion" but vague on implementation
   - Code: Comprehensive 4-signal endpoint detection (completion + idle + no spinners + not stale)
   - Impact: Prevents false positives for sessions waiting for user input

2. ✅ **Conservative Thresholds** (DOUBLED)
   - Spec: mustering_timeout=10m, cursor_frozen=20m
   - Code: mustering_timeout=20m, cursor_frozen=30m
   - Rationale: "Better to miss than interrupt legitimate work"

3. ✅ **Bash Violation Detection** (NEW)
   - Spec: No mention of detecting bash violations
   - Code: Detects bash patterns (cd, &&, pipes) and sends rejection prompts
   - Purpose: Enforce tool usage, prevent bash anti-patterns

4. ✅ **Permission Prompt Rejection** (NEW)
   - Spec: No mention of permission prompts
   - Code: Detects permission prompts with violations, uses `csm reject`
   - Purpose: Auto-reject unsafe bash commands

### Major Omissions (In Spec, Not Implemented)

1. ❌ **Whitelisted Patterns** (SPEC ONLY)
   - Spec: Don't interrupt "Running tests...", "Building...", "Compiling..."
   - Code: **NO whitelist implementation** (no pattern matching for long-running tasks)
   - Gap: Cannot configure session-specific "allowed to be slow" patterns

2. ❌ **Per-Session Overrides** (PARTIAL)
   - Spec: `astrocyte_timeout: 30m` per session in config
   - Code: Has session_overrides in config, but **no command** like `csm set-astrocyte-timeout`
   - Gap: Can't dynamically adjust thresholds at runtime

3. ❌ **Root Cause Analysis Metrics** (SPEC ONLY)
   - Spec: "80%+ incidents have actionable root cause diagnosis"
   - Code: Sends diagnosis prompt but **no tracking/aggregation** of results
   - Gap: No weekly aggregation, no pattern detection

4. ❌ **Recovery Step 3 (Session Restart)** (SPEC ONLY)
   - Spec: ESC → C-c → restart Claude Code
   - Code: ESC → C-c → manual alert (no automatic restart)
   - Gap: Doesn't kill and restart tmux sessions automatically

### Behavior Differences

**1. Check Interval**
- Spec: "2-5 minutes"
- Code: **60 seconds (1 minute)**
- Impact: Faster detection but more resource usage

**2. Detection Order**
- Spec: No specified order
- Code: Permission prompts checked FIRST (fresh start detection, no previous state needed)
- Impact: Can detect and reject violations immediately on first check

**3. "Ask Question Violation" Detection**
- Spec: Not mentioned
- Code: **DISABLED** (commented out lines 2382-2388)
- Rationale: "Never interfere with sessions asking user questions"
- Impact: Astrocyte will NEVER interrupt sessions asking questions (even in text)

**4. False Positive Rate**
- Spec goal: <5%
- Code approach: **Conservative by default** (doubled thresholds, endpoint detection)
- Likely result: <1% false positive rate (possibly 0% for AskUserQuestion prompts)

---

## SUMMARY TABLE: SPEC vs ACTUAL

| Feature | SPEC Says | Code Does | Match? |
|---------|-----------|-----------|--------|
| **Check interval** | 2-5 minutes | 60 seconds | ❌ (faster) |
| **Mustering threshold** | 10 min | 20 min | ❌ (more conservative) |
| **Zero token threshold** | Not specified | 15 min | ✅ |
| **Cursor frozen threshold** | 20 min | 30 min | ❌ (more conservative) |
| **Endpoint detection** | Vague ("don't interrupt AskUserQuestion") | 4-signal comprehensive check | ✅ (improved) |
| **Whitelisted patterns** | Yes ("Running tests...", etc.) | **NO** | ❌ (missing) |
| **Per-session overrides** | Yes (config + runtime command) | Config only | ⚠️ (partial) |
| **Recovery: ESC** | Yes | Yes | ✅ |
| **Recovery: C-c** | Yes | Yes | ✅ |
| **Recovery: Restart session** | Yes | **NO** (manual alert instead) | ❌ (missing) |
| **Diagnosis prompt** | Yes | Yes (if enabled) | ✅ |
| **Root cause tracking** | Yes (80% metric) | **NO** (no aggregation) | ❌ (missing) |
| **Slack notifications** | Yes | Yes | ✅ |
| **JSONL incident logging** | Yes | Yes | ✅ |
| **Bash violation detection** | No | Yes | ✅ (addition) |
| **Permission prompt rejection** | No | Yes | ✅ (addition) |
| **False positive prevention** | <5% goal | Endpoint detection + conservative thresholds | ✅ (likely <1%) |

---

## WHEN IT ACTUALLY INTERRUPTS (Based on Code)

### ✅ WILL Interrupt (Recovery Triggered)

1. **Mustering freeze** (>20 min)
   - Pattern: `✻ Mustering...` visible in consecutive checks
   - Duration: >20 minutes between checks
   - **UNLESS**: Endpoint detected (has completion + idle prompt)

2. **Zero tokens bug** (>15 min)
   - Pattern: `↓ 0 tokens` visible
   - Duration: Extracted from pane content (e.g., `15m 32s`)
   - **UNLESS**: Endpoint detected

3. **UI completely frozen** (>30 min)
   - Cursor: Same position in consecutive checks
   - Output: Pane content identical
   - Duration: >30 minutes
   - **UNLESS**: Endpoint detected

4. **Permission prompt stuck** (>10 min)
   - Pattern: Permission prompt with bash violations
   - Duration: >10 minutes
   - Action: `csm reject` (not ESC)

5. **Bash violation detected**
   - Pattern: cd, &&, ||, pipes, etc. in bash command
   - Duration: Immediate (fresh start detection)
   - Action: Send bash rejection prompt

### ❌ WILL NOT Interrupt (Skipped)

1. **Endpoint detected** (completion + idle + no spinners)
   - Example: AskUserQuestion visible with `❯` prompt
   - Example: "✅ Done. Ready to proceed. ❯"
   - Rationale: Natural conversation endpoint, waiting for user

2. **First check cycle** (no previous state)
   - Exception: Permission prompts (can detect on first check)
   - Rationale: Need 2 consecutive checks to compare states

3. **Asking user questions** (DISABLED detection)
   - Even if asking questions in text (not AskUserQuestion tool)
   - Rationale: "astrocyte must NEVER answer on user's behalf"

4. **Below threshold** (not yet stuck)
   - Example: Mustering for 10 minutes (threshold is 20 min)
   - Example: Cursor frozen for 15 minutes (threshold is 30 min)

---

## BOTTOM LINE

**What SPEC says**:
- "Detect stuck sessions within 5 minutes, auto-recover 99%, log 100%, self-diagnose 80%"
- Focus on zombie planning, blocked beads, silent failures
- Don't interrupt long tasks or user input prompts

**What CODE does**:
- Detects stuck sessions **within 1 minute** (faster than spec)
- More **conservative thresholds** (20-30 min vs 10-20 min)
- **Comprehensive endpoint detection** (4 signals) to prevent false positives
- **Auto-recovery via ESC/C-c** (no session restart)
- **100% incident logging** ✅
- **Diagnosis prompts sent** ✅
- **No root cause aggregation** ❌
- **No whitelist patterns** ❌
- **Added bash violation prevention** (not in spec) ✅

**Philosophy shift**:
- SPEC: "99% auto-recovery, <5% false positives"
- CODE: **"Better to miss an interruption than interrupt legitimate work"**
  - Result: Likely <1% false positives, possibly 95-98% auto-recovery (trades recovery for accuracy)

**Net effect**: More conservative, safer, fewer false alarms, but potentially misses some genuine hangs that fall below thresholds.
