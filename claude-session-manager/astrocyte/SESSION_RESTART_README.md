# Session Restart Recovery - Implementation

## Overview

Added session restart capability to astrocyte for recovering severely stuck sessions.

## New Modules

### 1. astrocyte_restart_tracking.py
**Purpose**: Track restart history and enforce rate limiting

**Functions**:
- `record_restart(session_name, outcome)` - Log restart event to JSONL file
- `get_last_restart_time(session_name)` - Get most recent restart timestamp
- `check_rate_limit(session_name, window_hours=1)` - Prevent excessive restarts

**File**: `~/.agm/astrocyte/restart-history.jsonl`

---

### 2. astrocyte_ctrlc_recovery.py
**Purpose**: Ctrl+C recovery for stuck sessions

**Functions**:
- `try_ctrlc_recovery(session_name)` - Send Ctrl+C and verify recovery

**Implementation**:
- Sends `tmux send-keys -t {session} C-c`
- Waits 2 seconds
- Verifies pane state changed

---

### 3. astrocyte_session_restart.py
**Purpose**: Hard restart using AGM commands

**Functions**:
- `should_restart_session(session_name, esc_failures, ctrlc_failures, stuck_hours)` - Check if unrecoverable
- `restart_session(session_name)` - Execute `csm kill --force` + `csm resume`

**Safety Checks**:
- Rate limit: max 1 restart/hour
- Age check: session >30 min old
- Process check: no active python/claude processes

---

## Integration

Modified `astrocyte.py`:
- Added imports for new modules
- Updated `recover_with_ctrl_c()` to use new module
- Updated `recover_with_session_restart()` to use new module with AGM commands

## Recovery Chain

Current chain (via strategy_chain config):
1. Kill stuck csm send processes
2. Send ESC
3. **Send Ctrl+C** (NEW - user requested)
4. **Session restart** (NEW - last resort)

## Configuration

Set recovery method in astrocyte config:
```yaml
recovery_method: chain
recovery_strategy_chain:
  - escape
  - ctrl_c
  - session_restart
```

## Testing

Integration test needed:
1. Simulate stuck session (>1 hour, ESC failed 3+ times)
2. Verify Ctrl+C attempted before restart
3. Verify restart triggered with safety checks
4. Verify restart history logged

## Deployment

1. Copy modules to astrocyte directory (done)
2. Restart astrocyte daemon
3. Monitor `~/.agm/astrocyte/restart-history.jsonl` for restart events
4. Monitor astrocyte logs for recovery chain execution
