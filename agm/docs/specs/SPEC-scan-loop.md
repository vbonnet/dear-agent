# SPEC: Scan Loop

> Component: `cmd/agm/scan.go`, `internal/ops/cross_check.go`

## Purpose

The scan loop is the orchestrator's heartbeat. It runs periodic scan cycles that monitor all active AGM sessions, detect anomalies (permission prompts, stalled workers, unmanaged sessions), check worker branch commits, run metrics health checks, and optionally cross-check session state via tmux capture-pane. It is the primary mechanism by which the orchestrator maintains situational awareness of the multi-agent system.

## Interface Contract

### Scan Cycle (`performScanCycle`)

**Input:** None (reads from Dolt and tmux directly).

**Output:** `ScanCycleResult`

| Field | Type | Description |
|-------|------|-------------|
| `Timestamp` | `time.Time` | When the scan ran |
| `Sessions` | `*ListSessionsResult` | All active sessions |
| `Metrics` | `*MetricsResult` | Health metrics with alerts |
| `MetricsAlerts` | `[]Alert` | Extracted alerts |
| `WorkerBranches` | `map[string][]WorkerCommit` | Recent commits per branch |
| `CrossCheck` | `*CrossCheckReport` | Cross-check results (if enabled) |
| `Findings` | `ScanFindings` | Aggregated key findings |
| `Errors` | `[]string` | Non-fatal errors encountered |

**Scan steps (executed sequentially):**
1. List sessions, identify those in `PERMISSION_PROMPT` state
2. Scan worker branches (`impl-*`, `agm/*`) for commits in last 24h
3. Run metrics health check (1h window)
4. If `--cross-check`: verify session state via tmux capture-pane

### Cross-Check (`ops.RunCrossCheck`)

| Field | Type | Description |
|-------|------|-------------|
| `cfg` | `*CrossCheckConfig` | Configuration for timeouts and RBAC |

**Output:** `CrossCheckReport` with per-session results and unmanaged session list.

**Detected states:**

| State | Condition | Auto-Recovery |
|-------|-----------|---------------|
| `HEALTHY` | Normal operation | None |
| `DOWN` | Tmux session no longer exists | None (reported) |
| `STUCK` | Permission prompt visible > `StuckTimeout` | Auto-approve if matches RBAC allowlist |
| `NOT_LOOPING` | Supervisor has no scan output in capture | Send restart nudge |
| `ENTER_BUG` | `[From:` message stuck in input buffer | Send Enter key |

**RBAC Allowlist (default safe patterns for auto-approve):**
`Read`, `Glob`, `Grep`, `Bash(git`, `Bash(go test`, `Bash(go build`, `Bash(go vet`, `Bash(ls`, `Write`, `Edit`

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--interval` | `5m` | Scan interval in loop mode |
| `--once` | `true` | Run single scan |
| `--loop` | `false` | Run continuous scan loop |
| `--cross-check` | `false` | Enable tmux capture-pane verification |

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Default scan interval | 5 min | `scanInterval` default |
| Stuck timeout | 5 min | `CrossCheckConfig.StuckTimeout` |
| Scan gap timeout | 10 min | `CrossCheckConfig.ScanGapTimeout` |
| Worker commit lookback | 24h | `time.Now().Add(-24 * time.Hour)` |
| Metrics window | 1h | `MetricsRequest.Window` |
| Tmux capture depth | 30 lines | `capture-pane -S -30` |
| Session list limit | 1000 | `ListSessionsRequest.Limit` |

## Dependencies

### Depends on
- `internal/ops.ListSessions` — enumerate active sessions from Dolt
- `internal/ops.GetMetrics` — health metrics with alert thresholds
- `internal/ops.RunCrossCheck` — tmux-based state verification
- `git` CLI — branch listing and commit log queries
- `tmux` CLI — capture-pane, send-keys, list-sessions
- `agm session select-option` — auto-approve permission prompts
- `agm send msg` — nudge stalled supervisors

### Depended on by
- Orchestrator session — runs scan in loop mode as primary monitoring
- Stall detection — scan findings feed into stall analysis
- Metrics dashboards — scan results provide health status

## Failure Modes

| Scenario | Expected Behavior |
|----------|-------------------|
| Dolt unreachable during scan | Error appended to `Errors[]`, partial result returned |
| Git not available | Worker branches scan returns empty map |
| Tmux capture-pane fails for session | Session marked as `DOWN` if "no such session", else `HEALTHY` with error detail |
| Auto-approve fails | Error appended to cross-check detail, no retry |
| Enter key send fails | Error appended to cross-check detail |
| Metrics check fails | Error appended to `Errors[]`, metrics section empty |

## Invariants

1. **Scan never blocks on failure** — each step catches errors and continues; partial results are always returned.
2. **Cross-check is the authority for session state** — `DetectSessionState` from tmux capture output overrides manifest state when cross-check is enabled.
3. **Auto-approve only matches RBAC allowlist** — only tools in the explicit allowlist are approved; unknown tools require manual approval.
4. **Well-known non-AGM sessions are excluded** — `main` and `default` tmux sessions are never reported as unmanaged.
5. **Health status escalation** — `healthy` -> `warning` (any alert or cross-check issue) -> `critical` (any critical-level alert).
6. **Worker branches are identified by prefix** — only `impl-*` and `agm/*` branches are scanned for commits.
