# AGM v4 — Technical Specification

**Version:** 4.0-reviewed
**Status:** Reviewed (5-persona review complete)
**Last Updated:** 2026-03-06

## Motivation

When running detached AGM sessions, the developer must manually switch to
tmux windows or run `agm status` to check whether any session needs
attention. During a typical 2-hour deep work session with 3+ agents,
this happens 8-12 times, breaking focus even when no action is needed.
The PERMISSION_PROMPT stall scenario is particularly costly: a missed
prompt blocks a session indefinitely with no signal to the developer.

Additionally, there is no way to compare how different agents handle the
same task without manual worktree setup, and the tmux UI provides no
ambient state indicators.

This spec adds three features in priority order. A fourth (Dolt migration)
is deferred pending concrete evidence of need.

### Out of Scope

The following are explicitly deferred:
- Notification history persistence (no log of past notifications)
- Mobile push notifications
- Comparison quality metrics (build/test results in v4; manual inspection)
- Dolt remote sync or replication
- Multi-user/multi-machine scenarios
- SQLite message queue migration (remains separate backend)
- Agentic API unification — see `docs/AGENTIC-API.md` and `docs/adr/ADR-016-shared-ops-layer.md`

### Assumptions

- The developer uses AGM in a private space where audible alerts are
  acceptable (configurable via `notify.sound`)
- The developer does not actively work between 22:00 and 08:00 (DND
  default, configurable)
- Agent comparisons occur less than weekly initially
- 30 seconds between notifications is short enough to be useful but long
  enough to avoid alert fatigue

---

## Feature 1: Cross-Platform Desktop Notifications

**Priority:** HIGH — most frequent pain point (manual polling) with
lowest implementation complexity.
**Phase:** 1 (weeks 1–3)
**Dependency:** `gen2brain/beeep` (BSD-2-Clause)

### Problem

When running detached AGM sessions, the developer must manually switch to
tmux windows or run `agm status` to check session state. A missed
PERMISSION_PROMPT stalls a session indefinitely. The developer has no
signal for task completion, errors, or long-running sessions without
active polling.

**Validated by:** Personal experience — observed 8-12 manual polls per
2-hour work session. PERMISSION_PROMPT stalls observed multiple times
per week with detached sessions.

### Success Criteria

After one week of use:
1. Manual `agm status` invocations drop below 2 per work session
   (measurable via shell history)
2. Zero PERMISSION_PROMPT stalls lasting >5 minutes (notifications
   catch them within one poll interval)
3. `agm notify status` shows >95% delivery success rate

### Design

#### New package: `internal/notify/`

```
internal/notify/
├── manager.go       # Orchestration: eval triggers, DND
├── backend.go       # Backend interface + beeep implementation
├── config.go        # Notification preferences from config.yaml
└── manager_test.go
```

**Manager** subscribes to the existing eventbus (`internal/eventbus/`)
for state-change events rather than polling independently. This reuses
the existing event infrastructure and ensures notifications are
temporally synchronized with actual state changes.

```go
// backend.go
type Backend interface {
    Notify(title, message string, priority Priority) error
}

type Priority int
const (
    PriorityLow    Priority = iota
    PriorityNormal
    PriorityHigh   // includes audible alert via beeep (if sound enabled)
)

// BeeepBackend implements Backend using gen2brain/beeep.
type BeeepBackend struct{ SoundEnabled bool }

func (b *BeeepBackend) Notify(title, message string, p Priority) error {
    if p == PriorityHigh && b.SoundEnabled {
        _ = beeep.Beep(beeep.DefaultFreq, beeep.DefaultDuration)
    }
    return beeep.Notify(title, message, "")
}
```

```go
// manager.go — reuses EscalationTracker from internal/astrocyte/
// for cooldown/dedup instead of reimplementing map+mutex
type Manager struct {
    backend  Backend
    cfg      Config
    tracker  *astrocyte.EscalationTracker // reuse existing cooldown logic
    failures atomic.Int64                 // track backend failures
}

func (m *Manager) HandleStateChange(event eventbus.Event) {
    payload := event.Payload.(eventbus.SessionStateChangePayload)
    trigger, priority := m.matchTrigger(payload.OldState, payload.NewState)
    if trigger == "" {
        return
    }
    if !m.tracker.ShouldPublish(event.SessionID) || m.isDND() {
        return
    }
    title := fmt.Sprintf("AGM: %s", event.SessionName)
    if err := m.backend.Notify(title, trigger, priority); err != nil {
        m.failures.Add(1)
        log.Warn("notification failed", "err", err, "session", event.SessionID)
    }
    m.tracker.RecordEscalation(event.SessionID)
}
```

**Reuse notes:**
- `EscalationTracker` from `internal/astrocyte/watcher.go` handles
  per-session time-windowed dedup with `sync.Map` (safer than map+mutex).
  Move to a shared location or import directly.
- Subscribe to `EventSessionStateChange`, `EventSessionCompleted`, and
  `EventSessionEscalated` from the existing `internal/eventbus/` hub
  instead of adding a second poll path to the daemon.
- Severity mapping from `astrocyte.mapSymptomToSeverity()` maps directly
  to notification Priority.

#### Notification triggers

| Previous State | Current State | Trigger Message | Priority |
|---|---|---|---|
| `THINKING` | `PERMISSION_PROMPT` | "Needs permission" | High |
| `THINKING` | `READY` | "Task complete" | Normal |
| any | `ERROR` | "Session error" | High |
| `THINKING` (>10 min) | `THINKING` | "Still thinking (10m+)" | Low |

#### New state: `ERROR`

Add to `internal/manifest/manifest.go`:

```go
const (
    StateReady            = "READY"
    StateBusy             = "THINKING"
    StatePermissionPrompt = "PERMISSION_PROMPT"
    StateCompacting       = "COMPACTING"
    StateOffline          = "OFFLINE"
    StateError            = "ERROR"  // NEW
)
```

`ERROR` is detected via three low-false-positive signals only (no
string matching — the existing 37.5% false-positive rate for tmux pane
parsing makes string-match error detection prohibitively noisy):

1. **Pane dead:** `tmux list-panes -F "#{pane_dead}"` returns `1` when
   the agent process has exited. Unambiguous process-level signal.
2. **Hook-reported:** Existing hook pipeline writes `ERROR` state.
3. **Health check:** 3 consecutive health check failures on a session.

**Rejected approach:** String matching (`"Error:"`, `"FATAL"`, `"panic:"`)
in tmux pane output. These strings appear frequently in normal agent
output (compilation errors, quoted error messages). This would cause
notification spam and train users to mute the system.

#### State file for fast reads

Each session writes a lightweight state file alongside its manifest:

```
~/.agm/sessions/{session-id}/state
```

Contents: single line, e.g. `THINKING 1709654321` (state + unix timestamp).

Written by the notification manager's event handler as a side effect of
every state-change event (not coupled to message delivery). This ensures
state files stay current even for sessions with no pending messages.

**Important:** The state file is a tmux UI artifact (consumed by Feature
2's status helper), not a storage artifact. It persists regardless of
which storage backend is active, including if Feature 4 ever ships.

#### Daemon integration

The notify manager subscribes to the eventbus hub at daemon startup.
No changes to the poll loop's tick handler. The manager runs in its
own goroutine with panic recovery and a 5-second context timeout per
notification to prevent blocking the event bus.

```go
// daemon.go startup
hub := eventbus.NewHub()
notifyMgr := notify.NewManager(cfg.Notify, hub)
go notifyMgr.Listen(ctx) // subscribes to state-change events
```

#### DND schedule

```yaml
# ~/.config/agm/config.yaml
notify:
  enabled: true
  sound: true                  # separate from visual notification
  cooldown: 30s
  dnd:
    enabled: false
    start: "22:00"             # local system timezone
    end: "08:00"
  triggers:
    permission_prompt: true    # DND-exempt: always fires
    task_complete: true
    error: true                # DND-exempt: always fires
    long_running: true
    long_running_threshold: 10m
```

Note: `permission_prompt` and `error` triggers are DND-exempt by default
(configurable). These represent stalled or failed sessions that require
attention regardless of time of day.

#### CLI: `agm notify`

New file: `cmd/agm/notify.go`

```
agm notify test              # Send test notification (non-zero exit on failure)
agm notify status            # Show config, recent failure count, DND window
agm notify dnd [on|off]      # Toggle DND mode
agm notify dnd --until 14:00 # DND until specific time
```

`agm notify status` displays:
- Current DND state and resolved window (local time + UTC)
- Backend failure count since daemon start
- Last successful notification timestamp

#### Verification

- `agm notify test` sends a test notification end-to-end; exits non-zero
  on backend failure with error message
- `agm doctor` validates beeep can fire (new check); exits with warning
  code if notification permissions are revoked
- Unit tests mock the `Backend` interface
- `agm notify test` with beeep disabled confirms daemon continues running
  and `agm notify status` shows degraded state

---

## Feature 2: Tmux Status Lines

**Priority:** MEDIUM — aesthetic complement to notifications; reduces
cognitive load with 3+ concurrent sessions.
**Phase:** 2 (weeks 3–4)
**Dependency:** Feature 1 (state file format)

### Problem

When the developer has 3+ active sessions, they cannot tell at a glance
which sessions need action without switching to each tmux window. This
is felt most acutely in the 30-second window after a notification fires
— the developer knows a session needs attention but cannot immediately
see which one without a visual anchor in the tmux UI.

**Validated by:** Personal experience with 3-5 concurrent sessions.
Notifications (Feature 1) push state changes but don't identify which
tmux window to switch to.

### Success Criteria

After one week of use with 3+ concurrent sessions:
1. The developer has not disabled `tmux.status_lines`
2. The pane border is the primary mechanism for identifying which
   session to focus on after receiving a notification

### Design

#### Simplified approach (per complexity review)

Instead of per-session injection with multiple styles, provide a single
`agm tmux-status` command that outputs formatted strings suitable for
direct use in tmux configuration. The developer adds it to their
`~/.tmux.conf` once. No install step. No style system.

```go
// cmd/agm/tmux_status.go
// agm tmux-status {session-name} {position}
// position: left | right | pane
//
// Reads state file directly by session ID (no glob).
// Session name → session ID mapping via lightweight index file.
```

#### Session-to-state-file mapping

To enable O(1) state file lookups (no glob), maintain a lightweight
index at `~/.agm/index/name-to-id.json`:

```json
{"my-session": "abc-123-def", "other-session": "xyz-789-uvw"}
```

Written by `agm session new` and updated by rename operations. The
tmux status command reads this index to resolve session name → session
ID → state file path.

#### Per-session injection in `NewSession()`

In `internal/tmux/tmux.go:NewSession()`, after existing settings,
inject status line configuration if enabled:

```go
if cfg.Tmux.StatusLines {
    helper := "agm tmux-status"
    settings := [][]string{
        {"set", "-t", name, "status-interval", "5"},
        {"set", "-t", name, "status-left-length", "40"},
        {"set", "-t", name, "status-left",
            fmt.Sprintf(" #(%s %s left) ", helper, name)},
        {"set", "-t", name, "status-right",
            fmt.Sprintf(" #(%s %s right) ", helper, name)},
        {"set", "-t", name, "pane-border-format",
            fmt.Sprintf(" #(%s %s pane) ", helper, name)},
        {"set", "-t", name, "pane-border-status", "top"},
    }
    for _, args := range settings {
        if err := RunWithTimeout(ctx, globalTimeout, args...); err != nil {
            log.Debug("status line setting failed", "setting", args)
        }
    }
}
```

#### Output format

**Status-left:** `AGM: session-name [claude]`
**Status-right:** `STATE elapsed` (color-coded)
**Pane border:** `session-name | THINKING | 12m`

#### State colors

| State | Color | Tmux Color Code |
|---|---|---|
| `READY` | Green | `#[fg=green]` |
| `THINKING` | Yellow | `#[fg=yellow]` |
| `PERMISSION_PROMPT` | Red (bold) | `#[fg=red,bold]` |
| `COMPACTING` | Cyan | `#[fg=cyan]` |
| `ERROR` | Red (blink) | `#[fg=red,blink]` |
| `OFFLINE` | Grey | `#[fg=colour245]` |

#### Config

```yaml
# ~/.config/agm/config.yaml
tmux:
  status_lines: true          # default: true
  status_interval: 5          # seconds between updates
```

#### Performance

- `agm tmux-status` reads index file + state file (two file reads, <5ms)
- Built-in `timeout 2` guard in the command to prevent tmux hangs
- 5s update interval, one status-left + one status-right + one pane-border
  per session = 3 invocations per session per 5 seconds
- Estimated overhead: <1% CPU on macOS with 10 sessions. **Must be
  benchmarked before Phase 2 ships** (run 10 sessions, measure p50/p99
  latency of `agm tmux-status` and `top` CPU sample)

#### Verification

- `agm session new` creates session with visible status bar
- Status bar updates within 5s of state change
- `tmux.status_lines: false` disables injection
- Benchmark: `time agm tmux-status test-session right` < 10ms p99

---

## Feature 3: A/B Agent Comparison

**Priority:** MEDIUM — high developer curiosity but low daily frequency.
Start as a shell script; graduate to Go if usage warrants it.
**Phase:** 3 (weeks 5–6)
**Dependency:** None

### Problem

As a developer evaluating which agent (Claude, Gemini, Codex, or OpenCode)
works best for a specific class of task, I want to run the same prompt
against multiple agents on a real codebase and see which one produced the
best implementation, so I can make a data-driven choice rather than a
subjective one.

**Note:** All 4 agents are now fully supported with BDD test coverage and
feature parity (Phase 1 & 2 complete).

**Validated by:** Not yet validated — shipping to test assumption.
Manual comparison attempted twice; took ~15 minutes each time.

### Success Criteria

Within the first month after shipping, the developer:
1. Runs at least 3 comparisons
2. Uses the results to make at least one agent selection decision

If usage is below this threshold after 2 months, the feature is
deprioritized.

### Design

#### Shell script approach (per complexity review)

Instead of a full Go package with YAML storage and lipgloss rendering,
start with a shell script `scripts/agm-compare.sh` (~80 lines) that:

1. Creates git worktrees under `~/.agm/worktrees/{id}/{agent}`
2. Launches `agm session new` for each agent with `--detached`
3. Polls `agm status` until all sessions reach READY (or timeout)
4. Runs `git diff --stat` in each worktree
5. Outputs side-by-side summary to stdout
6. Cleans up worktrees and sessions on EXIT trap

Wrapped by a thin CLI command for discoverability:

```
agm compare --harnesses claude-code,gemini-cli,codex-cli,opencode-cli --repo . --prompt "..."
    [--timeout 30m]
```

**Supported Agents:** claude, gemini, codex, opencode (all fully integrated)

`cmd/agm/compare.go` validates inputs and execs the shell script.

#### Worktree operations

Extend existing `internal/git/git.go` with:

```go
func AddWorktree(repoPath, worktreePath, branchName string) error
func RemoveWorktree(repoPath, worktreePath string) error
```

This reuses the existing `findGitRoot()`, `IsInGitRepo()`, and error
sentinel patterns already in the git package.

#### Constraints

- Max 4 agents per comparison
- Default timeout: 30 minutes (configurable via `--timeout`)
- One active comparison at a time (enforced via lockfile at
  `~/.agm/comparisons/.lock` using `O_CREATE|O_EXCL`)
- Requires git repo (validated via `git.IsInGitRepo()`)
- On timeout or crash: EXIT trap cleans up worktrees and terminates
  sessions. Stale lockfiles (older than timeout + 5min) are auto-removed
  by subsequent `agm compare` invocations.

#### Cleanup on failure

- EXIT trap in shell script handles SIGINT, SIGTERM, and normal exit
- Terminates comparison sessions via `agm session kill`
- Removes git worktrees via `git worktree remove`
- Stale comparison detection: if lockfile exists and is older than
  `--timeout + 5min`, it is removed and associated worktrees cleaned

#### Verification

- `agm compare --harnesses mock1,mock2 ...` with test harnesses
- Worktrees created and cleaned correctly on success, timeout, and SIGINT
- `git worktree list` shows no leaked worktrees after cleanup
- Stale lockfile auto-recovery works

#### Graduation criteria

If comparison usage exceeds 1x/week for 2+ months, graduate to a Go
implementation with:
- Persistent comparison records
- `agm compare list/status/results` subcommands
- lipgloss table rendering
- Daemon-side timeout enforcement

---

## Feature 4: Dolt Migration (Embedded Mode) — DEFERRED

**Priority:** LOW — deferred pending concrete evidence of need.
**Status:** Not scheduled. Revisit when a specific debugging scenario
requires time-travel queries or unified storage.

### Rationale for deferral

The 5-persona review identified that:

1. **No concrete user pain:** No specific incident where storage
   fragmentation caused a problem. "Difficult" is an engineering concern,
   not a documented workflow pain point.
2. **Simpler alternatives exist:** `agm backup` (tarball of `~/.agm/`
   and `~/.config/agm/`) achieves unified backup in ~30 LOC. SQLite
   migration (already present for message queue) achieves unified reads
   without introducing a new embedded database engine.
3. **Disproportionate cost:** 5-week implementation, three-phase
   migration with dual-write hazard, ~30MB+ binary size increase,
   branch-per-session concurrency bugs (`DOLT_CHECKOUT` on shared
   connection pool), and Phase C rollback that is destructive without
   a tested Dolt-to-YAML export path.
4. **Instrumental convergence risk:** The feature makes the storage
   system more sophisticated to serve the storage system's own properties
   (git history, SQL, branching) rather than user outcomes.

### If revisited

Before re-scheduling, the following must be true:
- A specific debugging scenario required time-travel queries and failed
- `agm backup` and SQLite migration have been tried and proven insufficient
- The `dolthub/driver` binary size impact has been measured
- `WithSessionBranch` uses `sql.Conn` (pinned connection) not `sql.DB`
  (connection pool) to prevent the branch-checkout race condition
- Phase C rollback includes a tested Dolt-to-YAML export procedure
- A dual-write validation gate (row count + checksum comparison) blocks
  Phase B promotion until Phase A data consistency is confirmed

### Interim: `agm backup`

Add a lightweight backup command instead:

```
agm backup [--output ~/backups/agm-{date}.tar.gz]
```

Tarballs `~/.agm/` and `~/.config/agm/` into a timestamped archive.
~30 LOC, zero new dependencies.

---

## Cross-Feature Dependencies

Despite shipping independently, these dependencies exist:

| Dependency | Impact |
|---|---|
| Feature 2 reads state files introduced by Feature 1 | Feature 1 must ship first |
| State files are tmux UI artifacts, not storage artifacts | Persist regardless of future storage changes |

Timeline ordering (1 → 2 → 3) respects these dependencies.

## Critical Files to Modify

| File | Feature | Change |
|---|---|---|
| `internal/manifest/manifest.go` | 1 | Add `ERROR` state constant |
| `internal/session/state_detector.go` | 1 | Pane-dead detection, state file writer |
| `internal/tmux/tmux.go` | 2 | Status line injection in `NewSession()` |
| `internal/config/config.go` | 1, 2 | Add notify + tmux status config |
| `internal/git/git.go` | 3 | Add worktree operations |
| `cmd/agm/main.go` | 1, 2, 3 | Register new subcommands |
| `go.mod` | 1 | Add beeep |

## New Files

| File | Feature | Purpose |
|---|---|---|
| `internal/notify/manager.go` | 1 | Notification orchestration via eventbus |
| `internal/notify/backend.go` | 1 | Backend interface + beeep impl |
| `internal/notify/config.go` | 1 | Notification config |
| `cmd/agm/notify.go` | 1 | `agm notify` CLI commands |
| `cmd/agm/tmux_status.go` | 2 | `agm tmux-status` command |
| `cmd/agm/compare.go` | 3 | `agm compare` CLI (thin wrapper) |
| `cmd/agm/backup.go` | 4-interim | `agm backup` command |
| `scripts/agm-compare.sh` | 3 | Comparison shell script |

## New Dependencies

| Package | License | Purpose |
|---|---|---|
| `gen2brain/beeep` | BSD-2-Clause | Cross-platform notifications |

## Phased Timeline

| Phase | Feature | Weeks | Ships Independently |
|---|---|---|---|
| 1 | Notifications | 1–3 | Yes |
| 2 | Tmux Status Lines | 3–4 | Yes (requires Feature 1 state files) |
| 3 | A/B Comparison (shell script) | 5–6 | Yes |
| — | `agm backup` | any time | Yes |
| — | Dolt Migration | deferred | Revisit when evidence warrants |

## Review Checklist

- [x] Product manager: user pain documented, success criteria added
- [x] Tech lead: eventbus integration, pane-dead detection, connection safety
- [x] Reuse advocate: EscalationTracker reused, eventbus reused, git pkg extended
- [x] Complexity counsel: Feature 2 simplified, Feature 3 → shell script, Feature 4 deferred
- [x] DevOps engineer: failure logging, cleanup paths, benchmark requirements

## Review Disposition Log

| Reviewer | Finding | Disposition |
|---|---|---|
| PM | No success criteria | Added per-feature success criteria |
| PM | No user validation | Added "Validated by" per feature |
| PM | Problem statements describe solution not pain | Rewritten as user pain |
| PM | DND exempt for critical triggers | Added DND exemptions |
| PM | Sound should be separately configurable | Added `notify.sound` config key |
| Tech Lead | `d.sessions` doesn't exist in daemon | Switched to eventbus subscription |
| Tech Lead | `isError()` string matching → false positives | Replaced with pane-dead + hook + health check |
| Tech Lead | `WithSessionBranch` connection pool race | Feature 4 deferred; fix documented for revisit |
| Tech Lead | State file glob in status helper | Index file + direct path lookup |
| Tech Lead | Dual-write silently swallows failures | Feature 4 deferred; validation gate documented |
| Tech Lead | Feature 2 / Feature 4 Phase C breaks state files | State files declared as permanent tmux UI artifacts |
| Reuse | Cooldown duplicates EscalationTracker | Reuse astrocyte.EscalationTracker |
| Reuse | Notify bypasses eventbus | Subscribe to eventbus instead of polling |
| Reuse | Worktree duplicates git helpers | Extend internal/git/git.go |
| Reuse | Comparison status bare strings | Define typed constants |
| Complexity | Feature 2 over-engineered | Simplified: no styles, single command |
| Complexity | Feature 3 disproportionate cost | Shell script first, graduate on evidence |
| Complexity | Feature 4 fails five-whys | Deferred pending evidence |
| DevOps | Notification errors silently discarded | Added failure counter + logging |
| DevOps | No panic recovery in notify path | Added goroutine with recover + timeout |
| DevOps | Worktree cleanup on failure unspecified | EXIT trap + stale lockfile recovery |
| DevOps | Phase C rollback destructive | Feature 4 deferred; export path documented |
| DevOps | No poll loop performance metrics | Moved to eventbus (no poll loop impact) |
