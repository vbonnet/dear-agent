# ADR-013: Tailscale-Integrated HTTP API

**Status**: Proposed
**Date**: 2026-05-03
**Context**: Builds on
[ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
and
[ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md).
Phase 5.5 added `workflow-inspector`, an HTML viewer over `runs.db`, but
read-only and loopback-only. This ADR covers the *control* surface — the
HTTP API humans need to approve gates, trigger runs, and triage findings
from off-host (in particular, from a phone over Tailscale).

---

## Context

dear-agent already exposes three classes of state through CLIs:

| Surface | Read | Write |
|---|---|---|
| Workflow runs | `workflow-list`, `workflow-status`, `workflow-logs` | `workflow-run`, `workflow-cancel` |
| HITL gates | `workflow-approve list` | `workflow-approve approve\|reject` |
| Audit findings | `workflow-audit list\|show` | `workflow-audit ack\|resolve` |

These work fine when the operator is sitting at the host running the
substrate. They do not help when the operator is away from the box and
a long-running workflow is parked at an `awaiting_hitl` node, or an
audit run flagged a P0 finding that wants triage now. The operator
must SSH home, navigate to the worktree, and run a CLI — every time.

The Phase 5.5 inspector solved the "what's happening?" question for
read-only browsing on the same host. It binds to `127.0.0.1:8080`
deliberately: it has no auth and no write paths, and exposing it to
the internet was never on the table.

This ADR adds the missing control plane:

1. A small **HTTP API** that wraps the existing `pkg/workflow` and
   `pkg/audit` Go APIs in JSON over HTTP. No new state, no new
   database — the CLIs and the API write to the same SQLite tables
   through the same library functions, so the audit trail is uniform.
2. A **Tailscale-native transport** so the API is reachable from the
   operator's phone or laptop without exposing it to the public
   internet, and so the *caller's identity* is established by the
   Tailscale fabric rather than by a homegrown auth scheme.

The operator's box already runs Tailscale; the goal is to add the
endpoint, not to invent the network.

---

## Decision

Add `cmd/dear-agent-api`, a new binary that:

1. **Listens on a Tailscale-internal address** via the
   `tailscale.com/tsnet` library. A `tsnet.Server` joins the operator's
   tailnet as a fresh node (`dear-agent` by default), and the HTTP
   server binds to its listener. The node never has a route into or
   out of the public internet beyond what Tailscale itself provides.
2. **Authenticates via Tailscale identity**. Each connection is mapped
   to its calling tailnet user via `tsnet.Server.WhoIs(remoteAddr)`.
   The user's `LoginName` is recorded as the actor on every write, and
   appears in the workflow audit log alongside CLI-driven decisions.
   No bearer tokens, no OAuth, no passwords — the tailnet is the
   identity boundary.
3. **Exposes JSON endpoints** that mirror the existing CLI surface:

   | Endpoint | Method | Wraps |
   |---|---|---|
   | `/status` | GET | health + version |
   | `/workflows` | GET | `workflow.List` |
   | `/workflows/{run_id}` | GET | `workflow.Status` + `workflow.Logs` |
   | `/gates` | GET | `workflow.ListPendingHITLRequests` |
   | `/gates/{approval_id}/approve` | POST | `workflow.RecordHITLDecision(approve)` |
   | `/gates/{approval_id}/reject` | POST | `workflow.RecordHITLDecision(reject)` |
   | `/audit/findings` | GET | `audit.Store.ListFindings` |
   | `/run` | POST | enqueue a workflow run (out of process) |

4. **Does not embed the runner**. `POST /run` writes a "queued run"
   record and shells out to `workflow-run` in a child process. The API
   binary stays a thin HTTP adapter; it does not own the LLM provider,
   the budget controller, or any other runner-side dependency. This
   keeps the binary's blast radius tiny — if it crashes mid-day,
   pending runs continue under the existing supervisor.

5. **Is split across two packages** to keep the wire layer testable
   without standing up a tailnet:

   - `pkg/api` — HTTP handlers. Depends on `pkg/workflow` and
     `pkg/audit`, takes an `Identifier` interface so tests can inject
     a stub. No `tsnet` import.
   - `cmd/dear-agent-api` — wiring: opens the SQLite databases,
     constructs a `tsnet.Server`, adapts its `WhoIs` to the
     `Identifier` interface, and serves `pkg/api`'s handler.

   Tests use `httptest.NewServer` against `pkg/api` with a stub
   identifier. The `tsnet` integration is exercised by hand on the
   operator's box; unit-testing tsnet would require either a real
   tailnet or mocking the tailnet daemon, neither of which buys us
   much over the manual smoke test.

---

## Consequences

### Positive

- **Off-host triage works**. The operator approves gates and triggers
  re-runs from a phone with `https://dear-agent.<tailnet>.ts.net/...`
  bookmarked. No port forwards, no VPN config, no exposed origin.
- **Identity is uniform**. Every write the API performs lands in
  `audit_events.actor` with the caller's tailnet `LoginName`. Operators
  can `git blame` a workflow decision the same way they `git blame`
  a CLI decision.
- **Read paths are cheap**. JSON `GET` responses are direct projections
  of the existing query helpers. No duplicate SQL, no new schema.
- **Failure isolation**. The API binary owns only HTTP plumbing and
  the tsnet listener. A crash takes down the API; it does not lose
  in-flight runs, drop SQLite handles held by the runner, or corrupt
  the audit DB.

### Negative / Open

- **One more daemon**. The operator's box now runs the runner *and*
  the API. We will add a launchd / systemd unit alongside the runner
  in a follow-up.
- **`tsnet` carries dependencies**. The Tailscale module pulls in a
  meaningful chunk of code (mostly its own networking stack). We pin
  to `tailscale.com@v1.84.0` to stay compatible with the project's
  current `go 1.25` toolchain; later upgrades will need to track
  Tailscale's Go-version cadence. CI continues to build with Go 1.25.
- **`POST /run` is fire-and-forget**. Returning the queued `run_id`
  is enough for the operator to follow progress via `GET
  /workflows/{id}`, but the API does not block on completion. Async
  runs are the right shape for a phone client anyway.
- **Loopback fallback**. For offline use (the tailnet is unavailable,
  or the operator is testing locally) the binary supports a
  `--loopback ADDR` flag that skips tsnet and binds directly. This
  mode has no auth — it is for development on the same host only, and
  is documented as such.

### Out of scope

- **Web UI**. The endpoints are JSON; a UI is a separate ADR. The
  existing `workflow-inspector` HTML can be ported on top of this API
  once we want a graphical phone client.
- **Multi-tenant tailnets**. We assume the operator is the only user
  in their tailnet, or that they trust everyone in it. ACLs on the
  Tailscale side already provide finer control if needed; the API
  does not implement role checks beyond `approver_role` matching,
  which is delegated to `workflow.RecordHITLDecision`.
- **Webhooks / push**. Notifications about new HITL gates remain the
  existing Discord backend's job. The HTTP API is pull-only.

---

## Alternatives considered

- **Tailscale Serve as an external proxy** in front of `workflow-inspector`
  bound to loopback. This was the original sketch and is the simplest
  thing that could work, but it bolts identity onto the wrong layer:
  the inspector has no concept of a caller, so writes would have to
  trust the proxy header, and there is no audit-log path. Embedding
  `tsnet` and reading `WhoIs` directly is barely more code and gives
  us identity for free.
- **Reverse proxy + bearer token**. Adding token auth to the existing
  inspector gets us writes off-host but invents a new credential to
  rotate, store on the phone, and protect. tsnet replaces all of that
  with the tailnet identity already present.
- **Embed the runner in the API process**. Tempting because it removes
  the shell-out for `POST /run`, but it doubles the API binary's
  blast radius and introduces a second LLM-provider lifecycle. We
  prefer the boring child-process model.

---

## References

- [tsnet docs](https://tailscale.com/kb/1244/tsnet/)
- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md)
- `cmd/workflow-inspector` — the read-only precursor to this API
- `cmd/workflow-approve` — the CLI write path that this API mirrors
