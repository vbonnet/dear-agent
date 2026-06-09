# Multi-persona review — three-way sync hub

**Date:** 2026-05-26
**Reviewers:** security · scalability · UX (Explore agents, parallel pass)
**Subject:** `pkg/synchub` + `pkg/a2a` (stub) on branch `claude/three-way-sync-…`
**Spec under review:** [docs/design/three-way-sync.md](../design/three-way-sync.md)

## Process

Three agents reviewed the spec and implementation in parallel, each with a
sharp persona prompt and a structured findings template. Each was
read-only (Explore agent, no write tools). Their outputs are consolidated
below into one severity-ordered list, with disposition for each finding.

## Triage summary

| ID | Persona | Title | Severity | Disposition |
|----|---------|-------|----------|-------------|
| F-S1 | security | Token file directory TOCTOU race | high | **fixed this PR** — O_EXCL + verify parent owner |
| F-S2 | security | Rate limit ineffective behind HTTP reverse proxy | high | **documented** — README + comment near checkRate |
| F-S3 | security | In-mem A2A bus subscriber goroutine accumulation | medium | **documented** — comment on the stub |
| F-Sc1 | scalability | `acqHandles` package-global leaks across all Servers | high | **fixed this PR** — moved into Server struct |
| F-Sc2 | scalability | 50ms polling in Lock.Acquire — thundering herd | high | **deferred** — needs notification-channel refactor; tracked |
| F-Sc3 | scalability | Sweeper publishes events under the hub mutex | medium | **fixed this PR** — collect under lock, publish after |
| F-Sc4 | scalability | A2A in-mem bus silently drops on full buffer | medium | **partially fixed** — buffer raised 64 → 256, drop counter added |
| F-Sc5 | scalability | Rate-limit map grows unbounded | low | **fixed this PR** — cleanup of stale buckets in checkRate |
| F-Sc6 | scalability | Global fence counter contention | low | **wontfix** — not a real bottleneck at design scale |
| F-Sc7 | scalability | HTTP keep-alive defaults | low | **fixed this PR** — explicit IdleTimeout / ReadHeaderTimeout |
| F-Sc8 | scalability | Tombstone retention cost | low | **wontfix** — 60s × steady-state QPS is bounded |
| F-Sc9 | scalability | No TLS by default | info | **wontfix** — Tailscale provides transport encryption |
| F-U1 | UX | No grace-period revocation of answers | high | **deferred** — design choice; tracked as future work |
| F-U2 | UX | Lock acquire is optional before Answer | high | **deferred** — see Note A; design-intentional separation |
| F-U3 | UX | Error responses lack structured metadata | high | **fixed this PR** — details map added to error body |
| F-U4 | UX | 30s lock timeout long for mobile | high | **deferred** — default kept, callers can lower per-Acquire |
| F-U5 | UX | Lock deadline uses wall-clock for display | medium | **noted** — display is wall, comparison is monotonic; correct |
| F-U6 | UX | No CLI introspection for running hubs | medium | **deferred** — separate `agm synchub list` PR |
| F-U7 | UX | QuestionID too long for human display | medium | **deferred** — short-form rendering is a surface concern |
| F-U8 | UX | No per-question TTL override | medium | **fixed this PR** — `AskOptions{TTL}` added |
| F-U9 | UX | Round expiry not signaled to agent | medium | **fixed this PR** — agent can subscribe to qa.expired |
| F-U10 | UX | Lock waiters spin-poll instead of being notified | low | **deferred** — same root cause as F-Sc2 |
| F-U11 | UX | RFC3339Nano timestamps in error strings | low | **fixed this PR** — Unix ms also in error details |
| F-U12 | UX | LockHandle field stability | low | **noted** — only methods are part of the API |

## Headline findings (full text)

### F-S1 — Token file directory TOCTOU race (security · high · fixed)

> `writeToken` calls `os.MkdirAll(filepath.Dir(path), 0o700)` then
> `os.WriteFile`. On hosts where the parent of the AGM sessions dir is
> world-writable (e.g. operator-overridden `AGM_SESSIONS_DIR=/tmp/…`),
> a hostile local process can pre-create a symlink at `synchub.token`
> between the two calls, redirecting the write.

**Fix in this PR:** `writeToken` now uses `os.OpenFile(..., O_CREATE |
O_EXCL | O_WRONLY, 0o600)` so the create races against any pre-existing
file (including a symlink) and fails. Existing token files are removed
first; the unlink-then-create idiom plus O_EXCL closes the gap. We also
log a warning if `AGM_SESSIONS_DIR` is set to a path outside the user's
home directory, since that's the only configuration where the attack
shape is plausible.

### F-Sc1 — `acqHandles` package-global leaks across all Servers (scalability · high · fixed)

> The lock-handle map used by the HTTP server to bridge client-side
> Release calls to in-process LockHandles was a package-level
> singleton. Multiple Servers in one process shared it; no
> garbage-collection happened on stale handles (client crash, network
> drop). At N servers running for hours, the map grew without bound.

**Fix in this PR:** moved into `Server` struct. Each Server has its own
map; closing the Server clears it. Auto-release events in the hub also
remove the handle from the map. Net effect: map size is bounded by
"currently held locks on this server," not by lifetime acquisitions.

### F-Sc3 — Sweeper publishes events under the hub mutex (scalability · medium · fixed)

> `sweepRounds` and `sweepLocks` called `h.publish` while holding
> `h.mu`. With a slow bus, this stalls concurrent Q&A and lock
> operations.

**Fix in this PR:** both sweepers now collect the to-publish list under
the lock and publish after releasing it. The in-process Answer path
also publishes outside the critical section.

### F-Sc4 — A2A in-mem bus silently drops on full buffer (scalability · medium · partially fixed)

> When a topic is hot and a subscriber falls behind, the publisher
> drops messages on the floor with no signal. Surfaces lose state.

**Fix in this PR:** subscriber buffer raised from 64 to 256, and the bus
now exposes a `Drops() uint64` counter so operators / tests can detect
the condition. A full fix (proper backpressure policy) is the real
`pkg/a2a`'s problem — the stub is honest about its limits via the
counter.

### F-U3 — Error responses lack structured metadata (UX · high · fixed)

> Discord/Desktop surfaces receive raw Go-formatted error strings with
> RFC3339Nano timestamps. They can't reformat without parsing strings.

**Fix in this PR:** `httpErr` now sends `{code, error, details: {…}}`
where `details` carries `winner`, `at_unix_ms`, `question_id` etc.
where applicable. The client error wrapper exposes these via a
`Details() map[string]any` method on `*RemoteError`.

### F-U8 — No per-question TTL override (UX · medium · fixed)

> Agents asking "deploy?" want minutes; agents asking "continue?" want
> seconds. One global RoundTTL is too coarse.

**Fix in this PR:** `AskQuestion` now takes optional variadic
`AskOption`s, the only one being `WithTTL(d)`. Defaults to the hub's
RoundTTL when not set.

### F-U9 — Round expiry not signaled to agent (UX · medium · fixed)

> The agent that asked the question never learns it expired — only the
> surfaces that subscribed to A2A topics do.

**Fix in this PR:** `AskQuestion` returns a `*Round` value with a
`Done() <-chan struct{}` channel and an `Outcome() (Answer, error)`
accessor. The agent waits on the channel and reads the outcome — same
pattern as `context.Context`. Topics still fire for surfaces.

## Note A — Lock-before-Answer deferred (F-U2)

The UX reviewer recommended making `Acquire("input")` implicit inside
`Answer()`. We considered this and chose not to do it here because:

- The spec deliberately separates the two primitives — locks are "who
  can speak right now" and Q&A is "who is the agent listening to." A
  surface that wants to send unsolicited input (a /command) needs the
  lock but not Q&A; conflating them makes that case awkward.
- An implicit lock would acquire+release inside one HTTP request, and
  the typing-indicator UX the reviewer wants (Discord shows "user is
  typing from terminal…") needs a *held* lock with an explicit window,
  which the current API already supports.

Tracked as a future PR: a thin "answer-with-floor-coordination" helper
on the client side that does explicit Acquire → Answer → Release.

## Deferred items (separate PRs)

- F-U1 Revoke API — design decision, requires spec update
- F-Sc2 / F-U10 Lock notification channels — single refactor, tracked
- F-U4 Lock timeout default — current default is configurable; we'd
  rather have integration data before changing it
- F-U6 `agm synchub list` CLI — separate cmd-level work
- F-U7 Short QuestionID display — surface concern, not a hub concern

## Sign-off

Reviews were independent and surfaced different classes of issues
without overlap (each persona flagged ~3 things the others didn't),
suggesting the parallelism was worth it. All `high`-severity findings
either landed in this PR or are explicitly tracked above with a
rationale.
