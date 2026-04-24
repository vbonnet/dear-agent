# Task Tracking API Design

**Status:** Draft
**Date:** 2026-04-12
**Author:** research-task-api session

## Problem

ai-tools has two separate task-tracking implementations:

1. **AGM sentinel intake** (`agm/internal/sentinel/intake/`) — JSONL-based WorkItem
   processor stored at `~/.agm/intake/queue.jsonl`. Rich model with priority, scope,
   evidence, guardrails, and execution tracking. State machine:
   `pending -> approved -> claimed -> in_progress -> completed|rejected`.

2. **Wayfinder taskmanager** (`wayfinder/cmd/wayfinder-session/internal/taskmanager/`) —
   YAML-backed tasks inside `WAYFINDER-STATUS.md`. Phase-scoped tasks with dependencies,
   deliverables, verification commands, and bead IDs. Statuses:
   `pending | in-progress | completed | blocked`.

Neither system provides:
- A unified API that external trackers (Jira, Linear, GitHub Issues) can plug into.
- A deterministic `HasOpenWork() bool` predicate for `/loop` shutdown decisions.
- Crash/sandbox-deletion recovery without manual intervention.

## Requirements

| Req | Description |
|-----|-------------|
| R1 | Local-first default: no network dependency for core operations |
| R2 | Deterministic `HasOpenWork()` — single call, no partial reads |
| R3 | Survive crashes, reboots, sandbox overlay teardown |
| R4 | Pluggable backends: local default + optional external sync |
| R5 | Human-auditable: raw store must be inspectable without tools |
| R6 | Rebuild from git: recovery path when local store is destroyed |
| R7 | Go-only implementation (project language policy) |

## Proposed API (Go interface)

```go
package tasktrack

// Work represents a trackable unit of work.
type Work struct {
    ID          string            `json:"id"`
    Title       string            `json:"title"`
    Description string            `json:"description,omitempty"`
    Status      Status            `json:"status"`
    Priority    Priority          `json:"priority"`
    Labels      []string          `json:"labels,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
    ClosedAt    *time.Time        `json:"closed_at,omitempty"`
    Log         []LogEntry        `json:"log,omitempty"`
    Meta        map[string]string `json:"meta,omitempty"` // backend-specific fields
}

type Status string

const (
    StatusOpen       Status = "open"
    StatusInProgress Status = "in-progress"
    StatusBlocked    Status = "blocked"
    StatusDone       Status = "done"
    StatusWontfix    Status = "wontfix"
)

type Priority string

const (
    PriorityP0 Priority = "P0" // critical
    PriorityP1 Priority = "P1" // high
    PriorityP2 Priority = "P2" // normal
    PriorityP3 Priority = "P3" // low
)

type LogEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Message   string    `json:"message"`
    Source    string    `json:"source,omitempty"` // session ID, user, etc.
}

type Filter struct {
    Status   []Status
    Priority []Priority
    Labels   []string
}

// Tracker is the core facade for work management.
type Tracker interface {
    // AddWork creates a new work item. Returns the assigned ID.
    AddWork(ctx context.Context, title, description string, priority Priority) (string, error)

    // CloseWork transitions a work item to a terminal status.
    CloseWork(ctx context.Context, id string, status Status) error

    // ListWork returns work items matching the filter.
    ListWork(ctx context.Context, filter Filter) ([]Work, error)

    // GetWork retrieves a single work item by ID.
    GetWork(ctx context.Context, id string) (*Work, error)

    // AppendLog adds a progress note to a work item.
    AppendLog(ctx context.Context, id string, message string) error

    // HasOpenWork returns true if any work item is not in a terminal status.
    // This is the predicate /loop uses to decide whether to keep running.
    HasOpenWork(ctx context.Context) (bool, error)
}
```

### Status lifecycle

```
open -> in-progress -> done
  |         |           ^
  |         v           |
  |      blocked -------+
  |                     |
  +---> wontfix         |
  +---------------------+
```

Terminal statuses: `done`, `wontfix`. Non-terminal: `open`, `in-progress`, `blocked`.

`HasOpenWork()` returns `true` when any item has a non-terminal status.

## Storage backends

### 1. Local JSONL (default)

**Path:** `~/.agm/tasks/work.jsonl`
**Format:** One JSON object per line, append-only for writes. Full rewrite on
status transitions (same pattern as `agm/internal/sentinel/intake/processor.go`).

| Property | Value |
|----------|-------|
| Crash durability | Good — `fsync` after write |
| Concurrent access | Mutex in-process; file lock (`flock`) cross-process |
| Query speed | O(n) scan — sufficient for <10k items |
| Human-readable | Yes — `cat` or `jq` |
| Git-recoverable | Yes — commit after mutations |
| Dependency | None (stdlib `encoding/json`) |

**Why not SQLite?** The project already uses JSONL for the intake queue. Keeping the
same format reduces cognitive load and avoids adding `modernc.org/sqlite` (~15MB
binary size increase). If query performance becomes a problem (>10k items), a
SQLite backend can be added later behind the same interface.

**Why not Markdown?** Fragile round-trip parsing. The Wayfinder taskmanager already
handles markdown and it's the largest source of parsing bugs in the codebase.

### 2. GitHub Issues (via `gh` CLI)

**Prerequisite:** `gh` CLI authenticated.

| Operation | Implementation |
|-----------|----------------|
| AddWork | `gh issue create --title T --body D --label priority:P` |
| CloseWork | `gh issue close N --reason R` |
| ListWork | `gh issue list --state open --json number,title,state,labels` |
| AppendLog | `gh issue comment N --body M` |
| HasOpenWork | `gh issue list --state open --json number \| jq length` |

**Sync strategy:** GitHub is the source of truth when configured. Local JSONL acts
as a write-ahead cache for offline operation. On reconnect, replay unsynced
mutations.

### 3. Linear (GraphQL)

| Property | Value |
|----------|-------|
| Auth | Bearer API key |
| Create | `mutation { issueCreate(input: {title, description, priority, teamId}) }` |
| Update | `mutation { issueUpdate(id, input: {stateId}) }` |
| Query | `query { issues(filter: {state: {type: {nin: ["completed","cancelled"]}}}) }` |
| Go SDK | Community `steebchen/linear-go` (limited) — prefer raw GraphQL client |
| Rate limit | 1,500 req/hr |

### 4. Jira (REST v3)

| Property | Value |
|----------|-------|
| Auth | API token (Basic), OAuth 2.0 (3LO), PAT |
| Create | `POST /rest/api/3/issue` |
| Update | `PUT /rest/api/3/issue/{id}` |
| Transition | `POST /rest/api/3/issue/{id}/transitions` (explicit workflow) |
| Query | `POST /rest/api/3/search` with JQL |
| Go SDK | Community `andygrunwald/go-jira` v2 (mature) |
| Rate limit | ~100 req/min |

**Note:** Jira's transition model is the most complex — status changes require
querying available transitions first, then posting the transition ID. The facade
must map `CloseWork(id, done)` to the correct Jira transition.

### 5. Notion (Database pages)

| Property | Value |
|----------|-------|
| Auth | Internal integration token (Bearer) |
| Create | `POST /v1/pages` with database parent + properties |
| Update | `PATCH /v1/pages/{id}` property updates |
| Query | `POST /v1/databases/{db_id}/query` with filter objects |
| Go SDK | Community `jomei/notionapi` |
| Rate limit | 3 req/sec |
| Webhooks | None (polling only) |

### 6. Asana (REST)

| Property | Value |
|----------|-------|
| Auth | OAuth 2.0, PAT |
| Create | `POST /api/1.0/tasks` |
| Update | `PUT /api/1.0/tasks/{id}` |
| Query | `GET /workspaces/{id}/tasks/search` |
| Go SDK | Official `asana/asana-go` (unmaintained) |
| Rate limit | 1,500 req/min |

## Backend comparison

| Criterion | JSONL | GitHub | Linear | Jira | Notion | Asana |
|-----------|-------|--------|--------|------|--------|-------|
| Network required | No | Yes | Yes | Yes | Yes | Yes |
| Setup complexity | None | `gh auth` | API key | Token + project config | Integration + DB setup | Token + project config |
| Query power | Linear scan | GitHub search syntax | GraphQL filters | JQL (most powerful) | Filter objects | Query params |
| Go SDK quality | stdlib | `google/go-github` (excellent) | Limited | `go-jira` v2 (good) | `notionapi` (ok) | Unmaintained |
| Offline support | Native | Via local cache | Via local cache | Via local cache | Via local cache | Via local cache |
| Human-readable | Yes | Web UI | Web UI | Web UI | Web UI | Web UI |

## Architecture

```
┌─────────────────────────────────────┐
│          Tracker interface          │  pkg/tasktrack/tracker.go
├────────┬────────┬───────┬───────────┤
│  JSONL │ GitHub │Linear │  Jira ... │  pkg/tasktrack/backends/
└────┬───┴────┬───┴───┬───┴───────────┘
     │        │       │
     v        v       v
  ~/.agm/   gh CLI  HTTP
  tasks/
```

### Package layout

```
pkg/tasktrack/
  tracker.go       — Tracker interface, Work/Status/Filter types
  jsonl/
    backend.go     — Local JSONL implementation
    backend_test.go
  github/
    backend.go     — GitHub Issues via gh CLI
    backend_test.go
  config.go        — Backend selection from env/config
```

### Configuration

```yaml
# ~/.agm/config.yaml (or per-project .agm/config.yaml)
task_tracking:
  backend: jsonl           # jsonl | github | linear | jira | notion | asana
  github:
    repo: owner/repo       # default: current repo from git remote
  linear:
    api_key_env: LINEAR_API_KEY
    team_id: TEAM-123
  jira:
    base_url: https://company.atlassian.net
    project_key: PROJ
    token_env: JIRA_API_TOKEN
```

## Key design: `HasOpenWork()` predicate

### Requirements for `/loop` integration

`/loop` runs a prompt or slash command on a recurring interval. It should keep
running as long as there is open work, and shut down gracefully when all work is
done.

```go
func (l *Loop) shouldContinue(ctx context.Context) bool {
    hasWork, err := l.tracker.HasOpenWork(ctx)
    if err != nil {
        l.logger.Warn("Failed to check open work, continuing", "error", err)
        return true // fail-open: don't shut down on transient errors
    }
    return hasWork
}
```

**Fail-open policy:** If `HasOpenWork()` returns an error (network failure, corrupt
file), the loop continues. Only an explicit `false` with no error causes shutdown.

### Implementation per backend

| Backend | HasOpenWork implementation |
|---------|---------------------------|
| JSONL | Scan file, return `true` if any item has non-terminal status |
| GitHub | `gh issue list --state open --limit 1 --json number` + check length |
| Linear | `query { issues(filter: {state: {type: {nin: ["completed","cancelled"]}}}, first: 1) }` |

### Durability across failures

| Failure mode | Recovery |
|--------------|----------|
| Process crash | JSONL file survives on disk (fsync'd). Next read picks up last state. |
| Sandbox deletion | JSONL file lost. If git-committed, rebuild via `git checkout -- ~/.agm/tasks/work.jsonl`. If using external backend, no data loss. |
| Reboot | Same as crash — JSONL on disk survives. |
| Network outage | Local JSONL always available. External backends fail-open. |

### Recovery from deletion

For the JSONL backend:
1. **Primary:** Auto-commit `work.jsonl` to a git repo after each mutation.
2. **Fallback:** Write a backup to `~/.agm/tasks/work.jsonl.bak` before each
   full rewrite (same pattern as the intake processor).
3. **Last resort:** External backend is source of truth — re-sync on next query.

## Integration with existing systems

### AGM sentinel intake

The existing `WorkItem` in `agm/internal/sentinel/intake/` is purpose-built for
the sentinel's automated intake pipeline (evidence-based, approval-gated). It
should **not** be replaced. Instead:

- The `Tracker` interface is a general-purpose facade for any work management.
- The sentinel can optionally create `Tracker` items from approved WorkItems
  (one-way sync: intake -> tracker).
- Mapping: `WorkItem.Status=approved` -> `Tracker.AddWork()`,
  `WorkItem.Status=completed` -> `Tracker.CloseWork(id, done)`.

### Wayfinder taskmanager

Wayfinder tasks are scoped to project phases and stored in `WAYFINDER-STATUS.md`.
They have richer semantics (dependencies, deliverables, verification commands).

- Wayfinder tasks should remain in their markdown format — it's the project's
  source of truth during active development.
- A read-only adapter can expose Wayfinder tasks through the `Tracker` interface
  for unified `HasOpenWork()` queries.
- The adapter would parse `WAYFINDER-STATUS.md` and map task statuses.

### Beads

Beads are outcome records, not work items. They track completed work (what was
done, by whom, with what result). The `Tracker` can link beads to work items via
`Work.Meta["bead_id"]`, matching Wayfinder's existing `Task.BeadID` field.

## Decisions

| Decision | Rationale |
|----------|-----------|
| JSONL default, not SQLite | Consistency with existing intake queue; no new dependency; human-readable; git-recoverable |
| Interface, not framework | Backends are independent packages implementing `Tracker`. No plugin registry, no DI container. |
| Fail-open on errors | `/loop` must not shut down due to transient failures. Only explicit "no open work" causes shutdown. |
| Don't replace intake or Wayfinder | Each serves a different purpose. Unify at the query layer, not the storage layer. |
| GitHub Issues as first external backend | Best Go SDK, `gh` CLI already available, simplest auth story |
| Status model is simpler than either existing system | 5 statuses vs AGM's 6 or Wayfinder's 4+verification. Covers the common cases; backends map to their native models. |

## Implementation plan

| Phase | Work | Depends on |
|-------|------|------------|
| 1 | Define `Tracker` interface + types in `pkg/tasktrack/` | — |
| 2 | Implement JSONL backend with `HasOpenWork()` | Phase 1 |
| 3 | Wire `HasOpenWork()` into `/loop` shutdown logic | Phase 2 |
| 4 | Implement GitHub Issues backend | Phase 1 |
| 5 | Add read-only Wayfinder adapter | Phase 1 |
| 6 | Add Linear/Jira backends (as needed) | Phase 1 |

## Open questions

1. **Should `HasOpenWork()` aggregate across backends?** If both JSONL and
   Wayfinder have tasks, should it check both? Recommendation: yes, via a
   `CompositeTracker` that OR's across backends.

2. **Should mutations auto-commit to git?** The intake processor doesn't.
   Adding auto-commit would improve recovery but adds latency. Recommendation:
   make it configurable, default off.

3. **Should the JSONL store live per-project or globally?** Per-project
   (`$PROJECT/.agm/tasks/work.jsonl`) scopes work to a repo. Global
   (`~/.agm/tasks/work.jsonl`) enables cross-project queries. Recommendation:
   per-project default with global index for cross-project `HasOpenWork()`.
