# ADR-029: Ralph Wiggum — host-tick merge loop, not GHA/webhook/Monitor

Status: Accepted (2026-06-15)

- **Epic:** ce-sbnd
- **Relates:** ce-cd14, ce-m3ya, ce-qpg9

PRs kept stalling: CI fails or the branch goes stale, the agent reports "unable
to merge," a human says "just go fix it." Fixing CI, resolving conflicts, and
clicking merge are 100% agent work; the only legitimate human escalation is a
*policy* block (security, product, money).

**Decision.** The persistent merge driver is a host-side Go orchestrator
(`cmd/mergeloop`) run by launchd as an idempotent `tick`, with a `run` daemon
mode for foreground use. It performs merge *mechanics* in-process through the
vetted `safe-*` packages (`safegit.SafeMerge`, `safe-rebase`) and *spawns AGM
agent sessions* for code-requiring states (CI failing, conflicts). The
deterministic, irreversible merge stays in a vetted wrapper (AGENTS.md
"Guarded delivery": never bypass a safety wrapper); only code edits run in a
sandboxed agent worktree.

**Rejected.** GitHub Actions (Option A) can't run our host-bound agent
toolchain to edit code — kept only as a future mechanical fast-path. Cowork
scheduling (Option B) can't start host code tasks (ce-m3ya) — so Option B is
re-homed on the host via launchd. A Monitor-tool session (Option C) isn't
durable and lets an LLM do merge mechanics — kept as an optional operator
overlay. Webhooks (Option D) need public ingress we don't have — deferred as a
latency optimization.

"Never stop" (persistent loop) and the two-retry maximum (AGENTS.md: after the
same approach fails twice, switch approaches or report the block) reconcile via
*abandonment*: after N failed agent attempts on the same failure a PR is
escalated with a concrete diagnosis, while the loop keeps driving every other
PR. Verified by OTel: `time_to_merge`, `pr.stall`, `human_escalation`.
