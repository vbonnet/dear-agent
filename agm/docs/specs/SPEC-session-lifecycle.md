# SPEC: Session Lifecycle

> Component: `internal/session/`, `internal/ops/session_archive.go`, `internal/ops/session_gc.go`, `internal/ops/archive_cleanup.go`

## Purpose

Manages the full lifecycle of AGM sessions: creation (via `agm session new` / `agm session associate`), resumption, archival, and garbage collection. Sessions are the fundamental unit of work in AGM — each session binds a Claude Code (or other harness) instance to a tmux pane, a working directory, and a manifest stored in Dolt.

## Interface Contract

### Session Creation (`agm session new` / `agm session associate`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | yes | Human-readable session name |
| `project` | `string` | yes | Working directory / repo path |
| `--create` | `bool` | no | Create session if not found during associate |

**Output:** Manifest written to Dolt with `SessionID` (UUID), `CreatedAt`, `Lifecycle: ""` (active).

**Error conditions:**
- Session name already exists (non-archived) -> error
- Dolt adapter unavailable -> error
- Invalid identifier (path traversal, `/`, `\`, `..`, leading `.`) -> rejected by `validateIdentifier()`

### Session Resume (`session.Resume`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `identifier` | `string` | yes | Session ID, name, or tmux name |
| `cfg` | `*config.Config` | yes | AGM config with `SessionsDir` |
| `adapter` | `*dolt.Adapter` | yes | Dolt storage adapter |

**Flow:**
1. Resolve identifier to manifest (tries: session ID -> tmux name -> manifest Name; skips archived)
2. Health check (`CheckHealth`) — validates working directory exists, checks for Claude Code session file bloat (>100MB / >1000 progress entries)
3. Ensure tmux session exists; create if missing
4. If Claude not already running: send `cd <project> && claude --resume <uuid> && exit` to tmux
5. Wait up to 5s for Claude readiness
6. Update `UpdatedAt` in Dolt
7. Display transcript context (last 3 exchanges)
8. Attach to tmux session

**Error conditions:**
- Session not found -> `"session not found: <identifier>"`
- Health check failure (missing working dir) -> error with summary
- Tmux session creation failure -> error
- Dolt adapter nil -> `"Dolt adapter required"`

### Session Archive (`ops.ArchiveSession`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Identifier` | `string` | yes | Session ID, name, or UUID prefix |
| `Force` | `bool` | no | Skip pre-archive verification |
| `KeepSandbox` | `bool` | no | Preserve sandbox for debugging |

**Output:** `ArchiveSessionResult` with session ID, previous status, verification report, MCP cleanup count, sandbox cleanup status, post-cleanup results.

**Pre-archive checks (skipped with `--force`):**
1. Not already archived
2. No active tmux session
3. Completion verification passes (no critical issues)
4. No pending delegations

**Side effects on archive:**
- `Lifecycle` set to `"archived"`
- Trust event recorded (`success` if commits exist, `false_completion` if none)
- Session deregistered from all monitor lists
- MCP processes killed (via `/proc` scan)
- Process group killed (`kill -TERM -<pane_pid>`)
- Tmux session killed
- Post-archive cleanup: worktree removal, worktree prune, merged branch deletion, sandbox removal
- RBAC `settings.local.json` preserved from sandbox upper layer
- Sandbox path removed from Claude `additionalDirectories`
- Actions logged to `~/.agm/logs/gc.jsonl` and `~/.agm/logs/cleanup.jsonl`

**Error conditions:**
- Empty identifier -> `ErrInvalidInput`
- Session not found -> `ErrSessionNotFound`
- Already archived -> `ErrSessionArchived`
- Active tmux (without `--force`) -> `ErrCodeVerificationFailed`
- Critical verification failure (without `--force`) -> `ErrCodeVerificationFailed`
- Pending delegations (without `--force`) -> `ErrCodeVerificationFailed`

### Session GC (`ops.GC`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `OlderThan` | `time.Duration` | no | Min inactivity period (0 = no filter) |
| `ProtectRoles` | `[]string` | no | Role substrings to protect (default: `orchestrator`, `meta-orchestrator`, `overseer`) |
| `Force` | `bool` | no | Skip pre-archive verification |

**Output:** `GCResult` with scanned/archived/skipped/errors counts and per-session entries.

**Skip conditions (checked in order):**
1. Already archived
2. Lifecycle = `"reaping"`
3. Name matches protected role (case-insensitive substring)
4. Active tmux session (batch-checked via `ComputeStatusBatch`)
5. Active manifest state: `WORKING`, `PERMISSION_PROMPT`, `COMPACTING`, `WAITING_AGENT`, `LOOPING`, `BACKGROUND_TASKS`, `USER_PROMPT`, `READY`
6. Last activity too recent (based on `OlderThan`)

**Safety guarantees (P0 postmortem requirements):**
1. Pre-GC health check: aborts entirely if Dolt storage unreachable (HTTP 503)
2. Active tmux exclusion
3. Active state exclusion
4. Supervisor role exclusion
5. All actions logged to `gc.jsonl`
6. Trust event `gc_archived` recorded for each collected session

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Resume tmux ready wait | 5s max | `tmux.WaitForClaudeReady(_, 5*time.Second)` |
| Session bloat threshold | 100MB file size | `bloatSizeThreshold = 100 * 1024 * 1024` |
| Bloat progress entry threshold | 1000 entries | `progressCount > 1000` |
| GC session scan limit | 1000 per pass | Implicit from `ListSessions` |
| Process kill grace period | 2s before sandbox removal | `time.Sleep(2 * time.Second)` after SIGTERM |

## Dependencies

### Depends on
- `internal/dolt.Adapter` — session storage (Dolt database)
- `internal/manifest.Manifest` — session data model
- `internal/tmux` — tmux session management (create, attach, send-keys, has-session, capture-pane)
- `internal/transcript` — extract context from previous session for display on resume
- `internal/session.VerifyCompletion` — pre-archive completion checks
- `internal/delegation.Tracker` — check for pending delegations before archive
- `internal/mcp` — MCP process cleanup on archive
- `internal/gclog` — GC audit logging

### Depended on by
- `cmd/agm/session.go`, `cmd/agm/archive.go`, `cmd/agm/resume.go` — CLI commands
- `cmd/agm/session_gc.go` — GC CLI command
- Scan loop — identifies sessions for monitoring
- Stall detection — reads session state for stall analysis
- Trust protocol — records trust events on archive/GC

## Failure Modes

| Scenario | Expected Behavior |
|----------|-------------------|
| Dolt unreachable during GC | GC aborts entirely (pre-health-check), returns HTTP 503 |
| Dolt unreachable during resume | Error returned, no tmux session created |
| Tmux session creation fails | Error returned, manifest not updated |
| Claude already running on resume | Skip sending commands, just attach |
| Archive with active tmux (no --force) | Rejected with error suggesting `--force` |
| Sandbox unmount fails | Logged as warning, archive proceeds (best-effort) |
| MCP cleanup fails | Logged as warning, archive proceeds (best-effort) |
| Trust event recording fails on archive | Logged as warning, archive proceeds |
| Branch delete fails (unmerged) | Logged at debug level, archive proceeds |
| Session file bloated (>100MB) | Health check reports issue with remediation steps (link to GitHub #19040) |

## Invariants

1. **No active session is ever GC'd** — sessions with active tmux, active manifest state, or protected role names are always skipped.
2. **Archive is idempotent** — attempting to archive an already-archived session returns a specific error, not a state corruption.
3. **Session identifiers are path-safe** — `validateIdentifier()` rejects `/`, `\`, `..`, leading `.`, and any path cleaning discrepancy.
4. **Resume never overwrites a running Claude** — if Claude is already running in the tmux pane, resume only attaches without sending new commands.
5. **Post-archive cleanup is best-effort** — failures in worktree removal, sandbox cleanup, MCP cleanup, or trust recording never block the archive operation.
6. **GC is logged** — every GC skip and archive action is written to `gc.jsonl` with session ID, name, and reason.
7. **Lifecycle state transitions are unidirectional** — `"" -> "reaping" -> "archived"`. There is no path from archived back to active except via `unarchive`.
8. **Manifest resolution skips archived sessions** — `ResolveIdentifier` only matches active sessions to prevent accidental operations on archived data.
