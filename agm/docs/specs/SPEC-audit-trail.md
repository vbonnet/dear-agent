# SPEC: Audit Trail

> Component: `internal/ops/audit.go`, `cmd/agm/admin_audit.go`

## Purpose

Provides a tamper-evident, append-only audit log of all AGM command executions. Every command invocation is recorded with timestamp, command name, session context, arguments, result, duration, and any errors. The audit trail is a compliance requirement for tracking what actions were taken, by whom, and when in the multi-agent system.

## Interface Contract

### Write Event (`AuditLogger.Log`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Timestamp` | `time.Time` | auto | Set to `time.Now()` if zero |
| `Command` | `string` | yes | Command name (e.g., `"session.archive"`) |
| `Session` | `string` | no | Target session name |
| `User` | `string` | no | User or agent identity |
| `Args` | `map[string]string` | no | Command arguments |
| `Result` | `string` | yes | Outcome (e.g., `"success"`, `"error"`) |
| `DurationMs` | `int64` | yes | Execution duration in milliseconds |
| `Error` | `string` | no | Error message if failed |

**Output:** Event appended as JSON line to audit log file.

**Error conditions:**
- Cannot open audit log file -> error
- JSON encoding failure -> error

### Create Logger (`NewAuditLogger`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `filePath` | `string` | no | Custom log path (default: `~/.agm/logs/audit.jsonl`) |

**Behavior:** If `filePath` is empty, creates `~/.agm/logs/` directory and writes to `audit.jsonl`.

### Read Events (`ReadRecentEvents`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `filePath` | `string` | yes | Path to audit log |
| `limit` | `int` | no | Max events to return (0 = all, >0 = last N) |

**Output:** `[]AuditEvent` — last N events from the file.

**Error conditions:**
- File not found -> returns `nil, nil` (empty result, no error)
- Malformed JSONL line -> skipped silently
- Scanner error -> error returned with partial results

### Search Events (`SearchEvents`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `filePath` | `string` | yes | Path to audit log |
| `params.Command` | `string` | no | Substring filter on command name |
| `params.Session` | `string` | no | Substring filter on session name |
| `params.Limit` | `int` | no | Max results (0 = all) |

**Output:** `[]AuditEvent` — matching events, last N if limit > 0.

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Max line buffer | 1MB | `scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)` |
| Default log path | `~/.agm/logs/audit.jsonl` | `defaultAuditDir()` |
| Log directory permissions | 0755 | `os.MkdirAll(dir, 0755)` |
| Log file permissions | 0644 | `os.OpenFile(_, O_CREATE|O_WRONLY|O_APPEND, 0644)` |

## Dependencies

### Depends on
- Filesystem — append-only JSONL file at `~/.agm/logs/audit.jsonl`
- `encoding/json` — event serialization/deserialization
- `sync.Mutex` — thread-safe writes within a single process

### Depended on by
- All `cmd/agm/` CLI commands — log command executions
- `cmd/agm/admin_audit.go` — CLI for querying audit events
- Compliance reporting — audit trail is a compliance requirement
- Post-incident analysis — audit trail provides command-level forensics

## Failure Modes

| Scenario | Expected Behavior |
|----------|-------------------|
| Audit log directory doesn't exist | Auto-created with 0755 permissions |
| Audit log file doesn't exist | Auto-created with 0644 permissions (O_CREATE) |
| Audit file not found on read | Returns empty result, no error |
| Malformed JSONL line on read | Line skipped, other events still returned |
| Concurrent writes from multiple processes | Safe at OS level (O_APPEND), but no cross-process locking |
| Disk full | Write error propagated to caller |
| Scanner buffer overflow (line > 1MB) | Line skipped by scanner |

## Invariants

1. **Append-only** — the audit log is only ever appended to via `O_APPEND`; events are never modified or deleted.
2. **Timestamp is always set** — if caller provides zero timestamp, `Log()` sets it to `time.Now()`.
3. **Thread-safe within process** — `sync.Mutex` protects concurrent writes from goroutines in the same process.
4. **Read tolerates corruption** — malformed JSONL lines are silently skipped; partial data is always returned.
5. **Default path is deterministic** — `~/.agm/logs/audit.jsonl` is always used unless explicitly overridden.
6. **Search is substring-based** — both command and session filters use `strings.Contains`, not exact match.
7. **Events are ordered by write time** — JSONL append ordering preserves chronological order; `ReadRecentEvents` returns the tail.
