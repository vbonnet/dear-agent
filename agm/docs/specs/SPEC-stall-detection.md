# SPEC: Stall Detection & Recovery

> Component: `internal/ops/stall_detector.go`, `internal/ops/stall_recovery.go`

## Purpose

Detects and recovers from stalled sessions in the multi-agent system. The stall detector scans active sessions for three classes of stalls: permission prompt timeouts, no-commit workers, and error loops. The stall recovery module takes corrective action (nudges, alerts, escalation) with retry tracking and exponential backoff.

## Interface Contract

### Stall Detection (`StallDetector.DetectStalls`)

| Field | Type | Description |
|-------|------|-------------|
| `ctx` | `context.Context` | Cancellation context |

**Output:** `[]StallEvent`

| Field | Type | Description |
|-------|------|-------------|
| `SessionName` | `string` | Stalled session name |
| `DetectedAt` | `time.Time` | Detection timestamp |
| `StallType` | `string` | `"permission_prompt"` / `"no_commit"` / `"error_loop"` |
| `Duration` | `time.Duration` | How long the stall has persisted |
| `Evidence` | `string` | Detailed stall evidence |
| `Severity` | `string` | `"warning"` or `"critical"` |
| `RecommendedAction` | `string` | Suggested recovery action |

**Detection checks (per session):**

| Stall Type | Condition | Severity | Applies To |
|------------|-----------|----------|------------|
| `permission_prompt` | State = `PERMISSION_PROMPT` for > `PermissionTimeout` | `critical` | All sessions |
| `no_commit` | State = `WORKING` for > `NoCommitTimeout` with 0 commits | `warning` | Worker sessions only |
| `error_loop` | Same error pattern appears >= `ErrorRepeatThreshold` times in tmux output | `warning` | Non-offline, non-ready, non-done sessions |

**Worker detection:** Session is a worker if name or tags contain "worker" (case-insensitive).

**Error pattern extraction:** Lines matching error keywords (`error:`, `failed`, `fatal`, `panic`, `cannot`, `permission denied`, `timeout`, `no such file`, `connection refused`, etc.) are normalized (timestamps removed, paths anonymized, line numbers replaced) and counted.

### Stall Recovery (`StallRecovery.Recover`)

| Field | Type | Description |
|-------|------|-------------|
| `ctx` | `context.Context` | Cancellation context |
| `event` | `StallEvent` | The detected stall to recover from |

**Output:** `RecoveryAction`

| Field | Type | Description |
|-------|------|-------------|
| `SessionName` | `string` | Target session |
| `ActionType` | `string` | `"nudge"` / `"alert_orchestrator"` / `"log_diagnostic"` / `"auto_approve"` / `"escalate"` |
| `Description` | `string` | Human-readable action description |
| `Sent` | `bool` | Whether action succeeded |
| `Error` | `string` | Error if failed |

**Recovery actions by stall type:**

| Stall Type | Action | Target |
|------------|--------|--------|
| `permission_prompt` | Alert orchestrator via `SendMessage` | Orchestrator session |
| `no_commit` | Nudge worker via `SendMessage` | Stalled worker session |
| `error_loop` | Send diagnostic to orchestrator via `SendMessage` | Orchestrator session |

**Retry tracking:**
- Recovery attempts are tracked per session via `RetryTracker`
- Exponential backoff between retries
- After max retries exceeded: escalation to orchestrator with full context (attempt count, last error, stall duration)
- Failed recovery attempts record the error message

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Permission prompt timeout | 5 min | `StallDetector.PermissionTimeout` |
| No-commit timeout | 15 min | `StallDetector.NoCommitTimeout` |
| Error repeat threshold | 3 occurrences | `StallDetector.ErrorRepeatThreshold` |
| Tmux capture depth | 50 lines | `captureSessionOutput(_, 50)` |
| Error message max length | 100 chars | `normalizeErrorMessage` truncation |
| Session scan limit | 1000 | `dolt.SessionFilter.Limit` |

## Dependencies

### Depends on
- `internal/dolt.Adapter` — list active sessions
- `internal/manifest.Manifest` — read session state and timestamps
- `internal/ops.SendMessage` — deliver nudge/alert/diagnostic messages
- `internal/ops.RetryTracker` — track retry attempts with backoff
- `tmux` CLI — capture-pane for error loop detection
- `git` CLI — count recent commits for no-commit detection

### Depended on by
- Orchestrator scan loop — invokes stall detection as part of scan cycle
- Trust protocol — stall events trigger `stall` and `error_loop` trust events

## Failure Modes

| Scenario | Expected Behavior |
|----------|-------------------|
| Tmux capture-pane fails | Error loop check skipped for that session |
| Git log fails | No-commit check returns -1 (skipped) |
| Orchestrator name not configured | Recovery returns error "no orchestrator session configured" |
| SendMessage delivery fails | Recovery action records error, retry tracked |
| Max retries exceeded | Escalation message sent to orchestrator with full context |
| Retry tracker I/O failure | Error propagated to caller |

## Invariants

1. **Only worker sessions are checked for no-commit stalls** — orchestrators and supervisors are excluded from commit-based stall detection.
2. **Offline/ready/done sessions skip error loop detection** — only sessions in active processing states are checked for error loops.
3. **Error patterns are normalized before counting** — timestamps, file paths, and line numbers are stripped to group equivalent errors.
4. **Recovery failures are tracked** — each failed attempt is recorded to prevent infinite retry loops.
5. **Escalation is terminal** — after max retries, the stall is escalated to the orchestrator; no further automatic recovery is attempted.
6. **Permission prompt stalls are critical severity** — they block all session progress and require immediate attention.
