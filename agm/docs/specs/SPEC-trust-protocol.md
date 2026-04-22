# SPEC: Trust Protocol

> Component: `internal/ops/trust.go`, `cmd/agm/trust.go`

## Purpose

Tracks agent session reliability through a numerical trust score (0-100). Trust events are recorded for each session to build a history of successes and failures. The trust score informs orchestrator decisions about session priority, delegation, and automated recovery. Trust data is stored in per-session JSONL files under `~/.agm/trust/`.

## Interface Contract

### Record Event (`ops.TrustRecord`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `SessionName` | `string` | yes | Target session name |
| `EventType` | `string` | yes | One of the valid event types |
| `Detail` | `string` | no | Optional detail message |

**Output:** `TrustRecordResult` containing the recorded `TrustEvent`.

**Event types and score deltas:**

| Event Type | Delta | Description |
|------------|-------|-------------|
| `success` | +5 | Task completed correctly |
| `false_completion` | -15 | Claimed done but wasn't |
| `stall` | -5 | Session stalled |
| `error_loop` | -3 | Stuck in error loop |
| `permission_churn` | -1 | Excessive permission prompts |
| `quality_gate_failure` | -10 | Quality gate check failed |
| `gc_archived` | 0 | Informational: collected by GC (no score impact) |

**Error conditions:**
- Empty `SessionName` -> `ErrInvalidInput`
- Invalid `EventType` -> `ErrInvalidInput` with list of valid types
- File I/O failure -> `ErrStorageError`

### Compute Score (`ops.TrustScore`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `SessionName` | `string` | yes | Target session name |

**Output:** `TrustScoreResult`

| Field | Type | Description |
|-------|------|-------------|
| `SessionName` | `string` | Session name |
| `Score` | `int` | Trust score (0-100) |
| `Breakdown` | `[]TrustScoreBreakdown` | Per-event-type counts and deltas |
| `TotalEvents` | `int` | Total events recorded |

**Score computation:** `score = 50 (base) + sum(event_delta * event_count)`, clamped to [0, 100].

### Query History (`ops.TrustHistory`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `SessionName` | `string` | yes | Target session name |

**Output:** `TrustHistoryResult` with ordered list of all `TrustEvent` entries.

### Leaderboard (`ops.TrustLeaderboard`)

**Input:** None.

**Output:** `TrustLeaderboardResult` — all sessions ranked by trust score (descending).

### Internal: `RecordTrustEventForSession`

Low-level helper used by archive and GC to record events without requiring an `OpContext`. Called automatically:
- On archive: `success` (if commits exist) or `false_completion` (if no commits)
- On GC collection: `gc_archived` (informational, score impact = 0)

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Base trust score | 50 | `trustBaseScore = 50` |
| Min trust score | 0 | `trustMinScore = 0` |
| Max trust score | 100 | `trustMaxScore = 100` |
| Score delta range | -15 to +5 | `trustEventDeltas` map |

## Dependencies

### Depends on
- Filesystem — JSONL files at `~/.agm/trust/<session-name>.jsonl`
- `encoding/json` — event serialization

### Depended on by
- Session archive (`ops.ArchiveSession`) — records `success`/`false_completion` on archive
- Session GC (`ops.GC`) — records `gc_archived` on collection
- Stall recovery — records `stall` and `error_loop` events
- `cmd/agm/trust.go` — CLI subcommands (score, history, record, leaderboard)
- Orchestrator decisions — trust scores inform delegation and priority

## Failure Modes

| Scenario | Expected Behavior |
|----------|-------------------|
| Trust directory doesn't exist | Auto-created on first write (`os.MkdirAll`) |
| Session has no trust file | `TrustScore` returns base score (50), `TrustHistory` returns empty list |
| Malformed JSONL line | Skipped during read (no error propagated) |
| Concurrent writes to same file | Safe: append-only with per-write file open/close (no shared file handle) |
| Trust recording fails during archive | Warning logged, archive continues (best-effort) |
| Invalid event type via CLI | Error with list of valid types |

## Invariants

1. **Score is always in [0, 100]** — clamped after computation regardless of event history.
2. **Base score is 50** — new sessions with no events start at 50/100.
3. **Events are append-only** — the JSONL file is only appended to, never rewritten or truncated.
4. **`gc_archived` has zero score impact** — it is an audit marker, not a quality judgement.
5. **`false_completion` is the heaviest penalty (-15)** — claiming completion without commits is the most penalized behavior.
6. **Trust recording never blocks operations** — all callers (archive, GC) treat trust failures as best-effort.
7. **Leaderboard includes all sessions with trust data** — no filtering by lifecycle state; archived sessions remain in rankings.
