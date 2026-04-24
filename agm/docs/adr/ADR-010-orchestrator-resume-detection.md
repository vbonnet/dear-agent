# ADR-010: Orchestrator Resume Detection Integration

**Status**: Proposed
**Date**: 2026-03-11
**Deciders**: Claude (AGM), Orchestrator v2
**Context**: Orchestrator v2 needs to detect when sessions have been resumed via `agm sessions resume-all` and send restart prompts

## Context and Problem Statement

After machine reboot, `agm sessions resume-all` successfully resumes all stopped sessions by:
1. Creating tmux sessions
2. Sending `claude --resume <uuid>` commands
3. Leaving Claude at the prompt

However, these resumed sessions are **idle** - Claude is waiting at the prompt but has no context about what to work on. The orchestrator v2 (monitoring 25+ sessions) needs to detect this "just resumed, needs restart" state and send appropriate restart prompts to get sessions productive again.

**Key Questions**:
1. How can orchestrator detect post-resume state?
2. What restart prompt should be sent?
3. How can AGM expose resume metadata to orchestrator?

## Decision Drivers

* **Reliability**: Detection must be robust (no false positives/negatives)
* **Performance**: Detection must scale to 50+ sessions
* **Simplicity**: Integration should be minimal, not invasive
* **Timing**: Orchestrator must wait for Claude to be ready (not interrupt initialization)

## Considered Options

### Option 1: Resume Timestamp File (Recommended)

**Mechanism**: AGM writes `.agm/resume-timestamp` file during `resume-all` operation

**Detection Logic** (Orchestrator):
```python
def needs_restart_prompt(session):
    resume_file = f"{session.path}/.agm/resume-timestamp"

    # Check if resume file exists
    if not os.path.exists(resume_file):
        return False

    # Read resume timestamp
    resume_time = datetime.fromisoformat(open(resume_file).read().strip())

    # Check if recently resumed (within 5 minutes)
    if datetime.now() - resume_time > timedelta(minutes=5):
        return False

    # Check if session is idle (no recent output)
    if session.has_recent_output(within_seconds=60):
        return False

    return True  # Needs restart prompt
```

**AGM Changes**:
```go
// In resume_all.go, after successful resume
func writeResumeTimestamp(sessionID string) error {
    agmDir := filepath.Join(cfg.SessionsDir, sessionID, ".agm")
    os.MkdirAll(agmDir, 0755)

    timestampFile := filepath.Join(agmDir, "resume-timestamp")
    timestamp := time.Now().Format(time.RFC3339)

    return os.WriteFile(timestampFile, []byte(timestamp), 0644)
}
```

**Pros**:
- ✅ Simple file-based coordination (no daemons, no sockets)
- ✅ Portable (works across AGM and orchestrator)
- ✅ Stateless (file is self-documenting)
- ✅ Easy to debug (inspect file directly)

**Cons**:
- ⚠️ Requires file I/O for each check
- ⚠️ Clock skew on timestamp comparison

---

### Option 2: Manifest Resume Field

**Mechanism**: Add `LastResumedAt` field to manifest.yaml

**Detection Logic** (Orchestrator):
```python
def needs_restart_prompt(session):
    manifest = load_manifest(session.manifest_path)

    # Check if recently resumed
    last_resumed = manifest.get('last_resumed_at')
    if not last_resumed:
        return False

    resume_time = datetime.fromisoformat(last_resumed)
    if datetime.now() - resume_time > timedelta(minutes=5):
        return False

    # Check if idle
    return not session.has_recent_output(within_seconds=60)
```

**AGM Changes**:
```go
// In internal/manifest/manifest.go
type Manifest struct {
    // ... existing fields ...
    LastResumedAt time.Time `yaml:"last_resumed_at,omitempty"`
}

// In resume_all.go
m.LastResumedAt = time.Now()
manifest.Write(manifestPath, m)
```

**Pros**:
- ✅ Centralized metadata (manifest is source of truth)
- ✅ Git-trackable (manifest changes are committed)
- ✅ No extra files

**Cons**:
- ❌ Pollutes manifest with ephemeral data
- ❌ Manifest churn (every resume = git commit)
- ❌ Slower to parse (YAML parsing vs file read)

---

### Option 3: Event Bus / Message Queue

**Mechanism**: AGM publishes "SessionResumed" events to message queue, orchestrator subscribes

**Pros**:
- ✅ Real-time notification (no polling)
- ✅ Decoupled architecture

**Cons**:
- ❌ Over-engineered for simple coordination
- ❌ Requires message broker (Redis, RabbitMQ, etc.)
- ❌ Orchestrator must maintain subscription state

---

## Decision Outcome

**Chosen Option**: **Option 1 - Resume Timestamp File**

**Rationale**:
- Simple file-based coordination fits AGM's design philosophy
- No manifest pollution with ephemeral data
- Easy to implement (5 lines of code in AGM, 10 lines in orchestrator)
- Debuggable (inspect `.agm/resume-timestamp` directly)
- Performant (file read is fast, cached by OS)

## Implementation Details

### AGM Changes

**File**: `cmd/agm/resume_all.go`

```go
// After successful resumeSession() call
if err == nil {
    successCount++

    // Write resume timestamp for orchestrator detection
    if err := writeResumeTimestamp(m.SessionID); err != nil {
        // Log but don't fail (non-critical)
        log.Printf("warning: failed to write resume timestamp for %s: %v", m.Name, err)
    }
}

func writeResumeTimestamp(sessionID string) error {
    agmDir := filepath.Join(cfg.SessionsDir, sessionID, ".agm")
    if err := os.MkdirAll(agmDir, 0755); err != nil {
        return err
    }

    timestampFile := filepath.Join(agmDir, "resume-timestamp")
    timestamp := time.Now().Format(time.RFC3339)

    return os.WriteFile(timestampFile, []byte(timestamp), 0644)
}
```

### Orchestrator Changes

**File**: `orchestrator/session_monitor.py` (or equivalent)

```python
import os
from datetime import datetime, timedelta
from pathlib import Path

def detect_sessions_needing_restart(sessions):
    """Detect sessions that need restart prompts after resume-all"""
    needs_restart = []

    for session in sessions:
        if should_send_restart_prompt(session):
            needs_restart.append(session)

    return needs_restart

def should_send_restart_prompt(session):
    """Check if session needs restart prompt"""
    resume_file = Path(session.manifest_dir) / ".agm" / "resume-timestamp"

    # Check if resume file exists
    if not resume_file.exists():
        return False

    try:
        # Read resume timestamp
        resume_time_str = resume_file.read_text().strip()
        resume_time = datetime.fromisoformat(resume_time_str)
    except Exception as e:
        log.warning(f"Failed to parse resume timestamp for {session.name}: {e}")
        return False

    # Ignore if resume was more than 5 minutes ago
    age = datetime.now() - resume_time
    if age > timedelta(minutes=5):
        return False

    # Check if session is idle
    if session.has_recent_output(within_seconds=60):
        return False  # Session is active, don't interrupt

    # Check if we already sent restart prompt
    if session.last_restart_prompt_sent and \
       (datetime.now() - session.last_restart_prompt_sent) < timedelta(minutes=10):
        return False  # Don't spam restart prompts

    return True

def send_restart_prompt(session):
    """Send restart prompt to recently resumed session"""
    prompt = "Your session was resumed after machine reboot. Please resume your previous work."

    session.send_to_tmux(prompt)
    session.last_restart_prompt_sent = datetime.now()

    # Delete resume-timestamp file to avoid re-prompting
    resume_file = Path(session.manifest_dir) / ".agm" / "resume-timestamp"
    resume_file.unlink(missing_ok=True)
```

## Restart Prompt Design

**Recommended Prompt**:
```
Your session was resumed after machine reboot. Please resume your previous work.
```

**Rationale**:
- **Simple**: Claude understands "resume previous work"
- **Context-aware**: Mentions reboot (explains gap in activity)
- **Action-oriented**: Clear instruction (resume work)

**Alternative Prompts** (more specific):
```
Session resumed post-reboot. Continue from: <last_task_from_context>
```

**Implementation**: Orchestrator can extract `last_task` from session context/notes if available.

## Timing Considerations

### AGM Resume Timing

```
T0: agm sessions resume-all starts
T1: Create tmux session (instant)
T2: Send "claude --resume <uuid>" (instant)
T3: Claude initializes (1-2 seconds)
T4: Claude displays prompt (ready)
T5: AGM writes resume-timestamp file
```

**Critical**: AGM should write timestamp **after** Claude prompt appears, not immediately after sending resume command.

**Solution**: Add small delay or wait for ready signal:

```go
// After resumeSession() returns
time.Sleep(2 * time.Second)  // Wait for Claude to initialize
writeResumeTimestamp(m.SessionID)
```

### Orchestrator Detection Timing

```
T0: Orchestrator polling loop (every 30 seconds)
T1: Check resume-timestamp files for all sessions
T2: Filter sessions (recent resume + idle + not already prompted)
T3: Send restart prompts
T4: Delete resume-timestamp files
```

**Grace Period**: 5 minutes allows time for:
- User to manually interact with session
- Other automation to handle session
- Network/filesystem delays

## AGM API Exposure

**Question**: Should AGM expose resume metadata via CLI?

**Answer**: Yes, useful for debugging and integration testing.

```bash
# Get last resume time for session
agm session get-resume-time <session-name>
# Output: 2026-03-11T11:30:45Z

# Check if session needs restart (per orchestrator logic)
agm session needs-restart <session-name>
# Output: true/false
```

**Implementation**:
```go
var getResumeTimeCmd = &cobra.Command{
    Use:   "get-resume-time [session-name]",
    Short: "Get last resume timestamp for session",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Find session
        sessionID := resolveSessionIdentifier(args[0])

        // Read timestamp file
        timestampFile := filepath.Join(cfg.SessionsDir, sessionID, ".agm", "resume-timestamp")
        data, err := os.ReadFile(timestampFile)
        if err != nil {
            return fmt.Errorf("no resume timestamp found")
        }

        fmt.Println(strings.TrimSpace(string(data)))
        return nil
    },
}
```

## Testing Strategy

### AGM Tests

1. **Unit Test**: `writeResumeTimestamp()` creates file with correct format
2. **Integration Test**: `agm sessions resume-all` writes timestamp for each resumed session
3. **Manual Test**: Verify `.agm/resume-timestamp` exists after resume-all

### Orchestrator Tests

1. **Unit Test**: `should_send_restart_prompt()` logic
2. **Integration Test**: Detect recently resumed sessions correctly
3. **E2E Test**: Full flow (resume-all → orchestrator detect → send prompt)

## Consequences

### Positive

- ✅ Orchestrator can detect and restart idle sessions automatically
- ✅ Seamless post-reboot recovery for 25+ session environments
- ✅ Simple file-based coordination (no complex infrastructure)
- ✅ AGM and orchestrator remain loosely coupled

### Negative

- ⚠️ Adds `.agm/resume-timestamp` files (clutter)
- ⚠️ Orchestrator must poll periodically (not real-time)
- ⚠️ Clock skew issues if system time changes

### Neutral

- 🔵 New coordination protocol between AGM and orchestrator
- 🔵 Requires orchestrator update to consume resume timestamps

## Related Decisions

- **ADR-008**: Status Aggregation - Similar file-based coordination pattern
- **ADR-009**: EventBus Multi-Agent Integration - Alternative event-driven approach (rejected for this use case)

## Notes

**Orchestrator Coordination Points**:
1. **Detection**: Poll `.agm/resume-timestamp` files every 30 seconds
2. **Restart Prompt**: "Your session was resumed after machine reboot. Please resume your previous work."
3. **Cleanup**: Delete timestamp file after sending prompt (avoid re-prompting)

**AGM Responsibilities**:
1. Write timestamp file after successful resume
2. Expose `get-resume-time` CLI for debugging
3. Document timestamp file format in SPEC.md

**Future Enhancements**:
- Add `--skip-orchestrator-signal` flag to resume-all (for testing)
- Support custom restart prompts via config
- Event-driven notification (when message queue infrastructure exists)
