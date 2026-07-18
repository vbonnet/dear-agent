# Three-Way Sync — Terminal · Desktop · Discord

**Status:** Draft
**Date:** 2026-05-26
**Author:** claude/three-way-sync session
**Scope:** `pkg/synchub`, `pkg/a2a` (stub), `agm/agm-plugin/channels/*`

## Problem

A user driving Claude Code can interact through three surfaces simultaneously:

- **Terminal** — `claude` CLI running in a tmux pane managed by AGM.
- **Desktop / mobile** — claude.ai surfaces with Claude Code's *Remote Control* feature.
- **Discord** — bot messages routed through agm-bus channels (AGM ADR-028).

These surfaces are bridged today by ad-hoc forwarding: tmux output is mirrored
to Discord, Desktop "remote control" is enabled per-session, AGM drives the
tmux side. There is no shared model of:

- Which surface currently *holds the floor* (so the user does not answer the
  same question twice on two devices and create racing state).
- Whose answer counts when several surfaces respond.
- How long a surface can hold the floor before yielding it back.

Without that model, the three surfaces are concurrent writers to a single
agent input stream — a classic last-writer-wins disaster.

## Goals & Non-Goals

**Goals.**
1. A single source of truth — the *hub* — for each Claude Code session,
   reachable from all three surfaces.
2. First-come-first-serve answer semantics with deterministic ordering: the
   first answer to a Q&A round wins; later answers for the same round are
   rejected with a clear reason.
3. Auto-releasing soft locks that never deadlock and never depend on wall
   clocks.
4. Push-based supervisor↔session traffic via A2A — no polling.
5. Localhost-only listener by default, with optional Tailnet binding. No
   public internet exposure under any configuration.

**Non-goals.**
- End-to-end encryption between surfaces. We rely on Tailscale's transport
  encryption when a session leaves the box.
- Cross-tenant isolation. Single-user installations only.
- Replacing Claude Code's *Remote Control* protocol — we sit *underneath*
  it as a coordinating hub, not on top of it.
- Replicating session history across machines. Each session has one home.

## Mental model

```
                       ┌─────────────────────────┐
                       │  Claude Code session N  │  ← the agent
                       └──────────┬──────────────┘
                                  │ stdin/stdout
                       ┌──────────▼──────────────┐
                       │   synchub (per session) │
                       │  ┌────────┐ ┌────────┐  │
                       │  │ Q&A    │ │ Locks  │  │
                       │  └────────┘ └────────┘  │
                       │   A2A inbox / outbox    │
                       └─┬────────┬────────┬─────┘
                         │        │        │
                  ┌──────▼──┐ ┌───▼───┐ ┌──▼──────┐
                  │ Terminal│ │Desktop│ │ Discord │
                  │  (AGM)  │ │  RC   │ │ agm-bus │
                  └─────────┘ └───────┘ └─────────┘
```

The hub is **per-session**, in-process for now (callable from AGM's session
process), and exposed to remote surfaces over a localhost or Tailnet socket.
There is no central broker — each session owns its own hub.

## Q&A protocol

A *Q&A round* is the unit of coordination. It begins when the agent emits a
question and ends when one surface answers, the round expires, or the agent
cancels it.

### Identifiers

Each round has a unique `QuestionID` of the form
`q-<session>-<monotonic-seq>-<rand6>`:

- `session` — the AGM session ID (already unique per machine).
- `monotonic-seq` — a per-session counter incremented under a mutex.
  Survives no restarts, but the random suffix and session ID together make
  collisions across restarts impossible in practice.
- `rand6` — six bytes of `crypto/rand` base32-encoded, so IDs cannot be
  guessed by a hostile client on the same Tailnet.

A new question — even from the same agent, on the same topic, immediately
after the previous one — gets a fresh `QuestionID`. This is what makes the
"second question is a new Q&A session" rule actually enforceable: surfaces
key their answers by `QuestionID`, not by question text.

### Answer arbitration

`Answer(qid, surface, payload)` is the only mutator. Pseudocode:

```
lock(round[qid])
  if round[qid].state != Open: return ErrClosed{reason}
  round[qid].state = Closed
  round[qid].winner = surface
  round[qid].answer = payload
  round[qid].closedAt = now() // monotonic
unlock(round[qid])
publish(A2A, AnswerWonEvent{qid, surface})
```

Only the first caller to acquire the per-round mutex with the round still
`Open` wins. All subsequent calls return `ErrClosed{Winner: …, At: …}`
without mutating state. This is deterministic — Go's `sync.Mutex` admits
exactly one holder at a time — and does not depend on clock comparisons.

### Expiry

Rounds expire after `roundTTL` (default 5 min). Expiry is enforced lazily
on every read and proactively by a single per-hub sweeper goroutine. The
sweeper uses monotonic clocks (`time.Since(round.openedAt)` where
`openedAt` was captured with `time.Now()` but compared via `time.Since`,
which is monotonic-safe).

Expired rounds are kept in memory for 60s as tombstones so that a late
answer gets a meaningful `ErrExpired{}` rather than `ErrNotFound{}`.

## Lock model

Locks coordinate *who can speak right now* — distinct from Q&A, which is
*who is the agent listening to right now*. A surface acquires a lock
before sending unsolicited input (e.g. a `/command`) so the other two
surfaces can show a "user is typing from Discord…" indicator and reject
collisions cleanly.

### Properties

| Property | Choice | Why |
|----------|--------|-----|
| **Timeout** | configurable, default 30s | matches AGM's per-turn budget; surfaces a hung surface within one human attention span |
| **Clock** | monotonic only | wall clocks jump under NTP and sleep/wake; a lock that "expires in the past" is a bug magnet |
| **Nesting** | forbidden | same goroutine acquiring the same lock twice returns `ErrAlreadyHeld`; nested holders across surfaces are impossible by construction |
| **Release** | explicit OR timeout, whichever first | no infinite holds; no leaks if a surface dies |
| **Fairness** | FIFO via channel-based queue | starvation-free; predictable ordering surfaces can communicate to humans |

### Algorithm

```go
type Lock struct {
    holder    surfaceID         // "" when free
    acquired  time.Time         // wall (display only)
    deadline  time.Duration     // since acquired, monotonic
    fence     uint64            // monotonic token
    waiters   chan request      // FIFO queue
}
```

Acquire:
1. Generate fence token `f := atomic.AddUint64(&fenceCounter, 1)`.
2. Push request onto `waiters`.
3. When dequeued, atomically claim if free; otherwise re-queue once after
   the current holder's deadline.
4. Return `{fence: f, release: func() { … }}`.

Release:
1. Compare-and-swap holder to empty using the fence token. A stale release
   (after auto-timeout) is a no-op.
2. Signal the next waiter.

Auto-release:
1. Single sweeper goroutine per hub, ticks every `min(timeout)/4`.
2. On tick: for each held lock where `time.Since(acquired) > deadline`,
   call the same CAS-by-fence release path. The original holder's later
   release becomes a no-op — same code path, no special case.

### Why no nested locks

We considered allowing a holder to re-enter its own lock for nested
sub-operations. Rejected because:

- Q&A and lock are distinct primitives; if a flow needs both, take them in
  *fixed order* (lock before Q&A) and the deadlock case disappears.
- Re-entrancy hides the *intent* of "this code runs while I hold X" — the
  callee can no longer tell whether it owns the lock or inherited it.
- Auto-release with reentrancy is unsound: which level's deadline applies?

So `Acquire(lock, holder)` while `holder` already owns `lock` returns
`ErrAlreadyHeld{}` immediately. The caller is expected to structure flows
to not need it.

## A2A integration

Supervisor↔session traffic moves to A2A so we can stop polling. The A2A
package is being built in parallel (see `a2a-spike` worktree). We commit a
*stub* interface here so this PR compiles independently and the real
implementation can drop in without churn.

```go
// pkg/a2a/a2a.go (stub)
package a2a

type Message struct {
    From, To string
    Topic    string
    Body     []byte
}

type Bus interface {
    Publish(ctx context.Context, m Message) error
    Subscribe(ctx context.Context, topic string) (<-chan Message, error)
    Close() error
}
```

The synchub uses A2A for:

- `synchub.<session>.qa.opened` — round opened
- `synchub.<session>.qa.answered` — round closed with winner
- `synchub.<session>.qa.expired` — round expired with no winner
- `synchub.<session>.lock.acquired` / `.released` / `.timed-out`

Surfaces subscribe to the topics they care about and react. This replaces
the current polling loop in agm-bus where the Discord channel polls the
session output every N ms.

Until the real `pkg/a2a` lands, the synchub ships with an in-memory bus
that satisfies the interface — good for tests and single-process use.

## Security envelope

| Concern | Mitigation |
|---------|------------|
| **Network exposure** | Bind defaults to `127.0.0.1`. Tailnet binding requires an explicit `--listen tailnet` flag; we resolve Tailscale's interface address rather than `0.0.0.0` so a fat-fingered config can't expose the listener publicly. |
| **Authentication** | Token-based, token written to `~/.agm/sessions/<id>/synchub.token` with mode 0600 at session start. Every RPC must carry the token in a header. |
| **Replay / spoofing** | Tokens are session-scoped and rotated at session start. A leaked token only exposes the one session it belongs to. |
| **Authorization** | Single-user model: a valid token has full access to that session's hub. We do not subdivide by surface — if someone has the token, they have all three surfaces' rights. |
| **DoS** | Per-IP request rate-limit (configurable, default 50 rps) on the listener. The Q&A and lock paths are O(1). |
| **Data at rest** | Hub state is in-memory only. Round history is logged to the existing AGM session journal for replay; no separate persistence. |

## Failure modes & tests

| Scenario | Expected behavior | Test |
|----------|-------------------|------|
| Two surfaces answer one Q&A simultaneously | First wins by mutex order; second gets `ErrClosed{Winner: first}` | `TestQA_FirstWriterWins` |
| Surface re-answers same round | `ErrClosed{}` returned, state unchanged | `TestQA_DoubleAnswerRejected` |
| Agent asks Q1 then Q2 immediately | Two distinct rounds, both accept answers | `TestQA_NewQuestionNewRound` |
| Surface acquires lock, dies | Lock auto-releases at deadline; next waiter gets it | `TestLock_AutoRelease` |
| Holder releases after auto-release | Release is a no-op (fence mismatch) | `TestLock_StaleReleaseNoop` |
| Goroutine acquires same lock twice | Second call returns `ErrAlreadyHeld` immediately | `TestLock_NoReentry` |
| Round expires before any answer | All later answers get `ErrExpired` | `TestQA_Expiry` |
| 50 concurrent answers on 1 round | Exactly 1 winner | `TestQA_HighConcurrency` |
| Clock jumps backward 1h | Locks still expire on schedule (monotonic) | `TestLock_MonotonicClock` |

## Open questions

- Should the lock model expose *priorities* for the supervisor (e.g.
  VROOM Overseer can preempt)? Current answer: no — keep the primitive
  flat, build preemption as a higher-level intent later.
- Does Desktop's *Remote Control* expose a hook we can register with so
  the hub learns about Desktop-side input *before* it hits the session?
  Needs investigation against the actual RC protocol once we have it.
