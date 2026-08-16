# ADR-013: Tailscale-Integrated HTTP API

Status: Accepted (2026-05-03; verified 2026-07-17)

The workflow CLIs and `workflow-inspector` cover read-only browsing from the
host. They do not help when the operator is away from the box and a
long-running workflow is parked at a HITL gate, or an audit run flagged a P0
finding that wants triage now. SSHing home to run `workflow-approve` does not
scale to a phone.

Add `cmd/dear-agent-api`: a small HTTP API wrapping `pkg/workflow` and
`pkg/audit` over [`tsnet`](https://tailscale.com/kb/1244/tsnet/). Endpoints
mirror the existing CLI surface (`/workflows`, `/gates`, `/audit/findings`,
`/run`). No new state, no new database — same library functions as the CLIs,
so the audit trail stays uniform.

- **Identity is the tailnet.** `tsnet.Server.WhoIs(remoteAddr)` resolves the
  caller; `LoginName` lands in `audit_events.actor` alongside CLI-driven
  decisions. No bearer tokens, no OAuth — Tailscale is the auth boundary.
- **The API does not embed the runner.** `POST /run` validates the request,
  spawns `workflow-run` as a child process, and returns its PID; the child
  registers the run row in `runs.db`. If the API crashes mid-day, in-flight
  runs continue under the existing supervisor.
- **Split: `pkg/api` (handlers, takes an `Identifier` interface) +
  `cmd/dear-agent-api` (wires `tsnet`).** Tests use `httptest` against the
  package; the tsnet integration is exercised by hand on the operator's box.

The alternative — Tailscale Serve as a proxy in front of
`workflow-inspector` — was the obvious first sketch, but bolts identity onto
the wrong layer: the inspector has no concept of a caller, so writes would
trust a proxy header and there is no audit-log path. Embedding `tsnet` and
reading `WhoIs` is barely more code and gives us actor identity for free.

A `--loopback ADDR` flag skips tsnet for offline dev and is documented as
auth-free. Webhooks, a web UI, and multi-tenant tailnets are out of scope.
