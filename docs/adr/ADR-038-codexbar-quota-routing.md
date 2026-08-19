# ADR-038: CodexBar-backed quota-aware routing and cost guardrail

Status: Accepted (2026-08-11)

## Context

The router picks a model per role from a primary/secondary/tertiary chain in
`config/roles.yaml`, and falls through the chain only when a call *fails*. A
provider approaching its subscription limit therefore keeps taking traffic
until the first request is rejected. Across a day this exhausts whichever
vendor a role names first while paid capacity sits unused at the other two —
the failure the 2026-08-11 cost retro recorded (Beads epic `ce-3rmqx`).

Preserving capacity needs a meter, and the meter has to cover subscription
plans rather than metered API keys, because that is where the binding limits
sit: Claude Max, Codex Pro, and Google AI Ultra all enforce rolling and weekly
windows that no per-request accounting in this repo can observe.

CodexBar already aggregates those windows locally for every provider on the
host and publishes them programmatically. Consuming that is far cheaper than
writing and maintaining three vendor-specific log and dashboard scrapers.

The paired Engram Research security audit (engram-research #313) gated the
dependency: install only CodexBar `v0.49.0`+, preferably `v0.49.1`+ (earlier
builds have a SQLite cost-store defect on large Codex histories); start with
Codex, Claude, and Gemini only; leave browser-cookie imports, iCloud sync, and
optional telemetry-style integrations disabled pending separate review.

### The data interface

Verified against CodexBar 0.49.2 on the development host. CodexBar exposes two
JSON surfaces:

| Surface | Shape | Verdict |
| --- | --- | --- |
| `codexbar usage [--provider X] --format json` | Per-provider `usage.primary/secondary/tertiary` windows plus `extraRateWindows`, each with `usedPercent` | Rejected: `usedPercent` only, window layout differs per provider, and identity is not redactable |
| `codexbar dashboard --identity redacted` | `schemaVersion`, `generatedAt`, `staleAfterSeconds`, and a uniform `providers[].windows[]` array carrying both `usedPercent` and `remainingPercent` | **Chosen** |

`dashboard` wins on three counts: one window shape across every provider, an
explicit `--identity redacted` mode so account addresses never enter this
process, and a declared `schemaVersion` to pin the parse against.

Two properties of the real payload drove the design:

- **A provider can report an error *and* usable windows.** The host's Codex
  entry carries `"codex cost refresh timed out"` next to a good weekly window.
  Windows must outrank the error field.
- **The call is slow.** Measured 5–30 s, because it refreshes providers over
  the network. It cannot sit on the routing path.

CodexBar's loopback server exposes the same dashboard schema. The CLI is
preferred: no always-on local network dependency, no server lifecycle to
manage, and identity redaction stays explicit at the call site.

## Decision

`pkg/llm/quota` owns a provider-neutral quota meter in three layers:

- `Reader` produces a `Snapshot`. `CodexBarReader` shells out to
  `codexbar dashboard --identity redacted`, folds CodexBar's provider ids onto
  dear-agent provider families through a configurable alias table, and
  classifies every provider as `ok`, `disabled`, `auth_required`, or
  `unavailable`.
- `Evaluate` reduces a snapshot to a per-family `Decision` — healthy,
  deprioritized, avoid, or unknown — from the *most constrained* window, under
  operator-set thresholds and a maximum reading age.
- `Meter` caches one snapshot, refreshes it off the request path, and exposes
  the verdict as a candidate ordering.

`router.Options.Quota` reorders a role's candidate chain by that verdict.
`Meter.HasCapacity` satisfies `pkg/workflow/roles.CapacityChecker`, so the
workflow role resolver can take the same meter without a second mechanism.

### Ordering is by coarse band, not by exact percentage

Candidates sort by quartile of remaining quota, stably, so the configured order
survives within a band. Sorting on the raw percentage would hand all traffic to
whichever provider is one point ahead until it falls one point behind, and would
churn the per-role vendor assignment on every refresh. Quartiles spread load
across providers that are genuinely in different shape and leave `roles.yaml` in
charge otherwise. An explicit avoid or deprioritize threshold floors the band,
so tuning the thresholds still does what it says.

### Unknown always means "route as before"

Every way of not knowing — no CodexBar, a failed read, an unsupported schema, a
reading past its max age, a provider needing credentials, a provider the
operator disabled, a model the resolver cannot place — produces `ClassUnknown`,
which sorts into the top band and contributes no metadata. An unmetered
deployment routes byte-identically to one without this package.

Two consequences fall out of that rule and are load-bearing:

- **A missing reading is never exhaustion.** A signed-out provider reports
  `auth_required`, not "0% left". Treating a credential problem as a spent
  budget would route traffic away from a provider that is completely fine, and
  would do it silently.
- **No candidate is ever dropped.** The meter reorders; it does not filter. A
  role stays routable even when every provider it can reach is out of quota.
  Failing at the provider is a clearer signal than a router that refuses to
  try. `HasCapacity` is the one place a tier can be skipped, and it is opt-in
  by whoever sets `Resolver.Capacity`.

### Enablement

`workflow-run -quota` is a tri-state: `auto` (default) turns metering on only
when `codexbar` is already on `PATH`, `on` forces it, `off` disables it. dear-agent
never installs or auto-starts CodexBar — that stays an operator decision, gated
by the audit above.

### The cost guardrail

Routing preference is not a budget control. Reordering a candidate chain
changes which provider a role prefers; it cannot stop a fleet from spending a
subscription down to zero, because every candidate is still allowed. The
guardrail is the part that says no.

It gates **spawns**, not requests. An agent session spends for as long as it
lives, so refusing to start one is the control that actually bounds cost;
refusing a call inside a session already running mostly just breaks that
session. `agm/internal/circuitbreaker` already owns spawn admission for every
sanctioned path, so the guardrail is a new gate there — `provider_quota` —
rather than a second mechanism beside it.

Two signals trip it:

- **Headroom.** Remaining quota at or below the halt floor (3%).
- **Burn rate.** CodexBar's `pace` reading says the current rate will not reach
  the window's reset. Headroom alone cannot distinguish "50% left, spending
  normally" from "50% left, gone in six hours" — that difference is the whole
  point of a runaway-cost guardrail. Below the spike floor (20% headroom) an
  overspend halts; above it, there is still room to correct, so it throttles.

`pace` lives on CodexBar's `usage` surface rather than `dashboard`, so
collecting it costs a second invocation. That surface has no
`--identity redacted` mode, so the parser declares only `provider` and `pace`:
account addresses are dropped during decoding rather than ignored afterwards.

The **fail-safe direction is inverted relative to its neighbours**, and
deliberately. Every other gate in that package fails closed — an unreadable
disk or process table refuses the spawn, because those guard against the
resource fill that took the host down. This one fails open. A missing quota
reading is not evidence that a budget is spent, and halting the whole fleet
because a meter is uninstalled, signed out, or merely stale would be a far
worse failure than one spawn too many.

### The published state file

Reading the meter takes seconds, so no consumer may do it inline. One
scheduled job (`agm admin install-quota-schedule`, every 30 minutes) publishes
`~/.local/state/dear-agent/quota/latest.json` atomically; the guardrail,
`agm quota`, and the orchestrator all read that file in O(1).

The file is versioned and carries the verdicts, not just raw percentages, so
consumers do not each re-derive policy. It records `readable` separately from
`remainingPercent` so a consumer cannot mistake an unreadable provider for an
exhausted one.

The interval is half the 90-minute gating age, so one missed run still leaves
a usable reading. If the job dies, readings go stale and the guardrail stops
halting spawns — safe, but it means a dead job silently removes the guardrail.
`agm quota` prints STALE so the condition is visible rather than silent.

## Alternatives

**Per-request token accounting in this repo.** Cannot see usage from Claude
Code, the Codex CLI, or Antigravity sessions running outside the workflow
engine, which is most of the consumption on a developer host. It would also
have no idea when a rolling window resets.

**Scraping provider logs directly** (`~/.claude/projects`, `~/.codex/sessions`,
Gemini CLI state). The fallback if CodexBar regresses or is not installed. It
means owning three formats that change without notice, so it is worth doing
only behind the same `Reader` interface — the router must not learn provider
storage layouts.

**Hard-coded provider weights.** Simplest, but blind to resets, to usage from
outside dear-agent, and to per-account plan differences — which is exactly the
information that makes the meter worth having.

**Gating requests instead of spawns.** Bounds nothing useful: a session that
has already started keeps spending, and failing its calls mid-flight breaks
work rather than preventing it.

**A dollar-denominated guardrail on top of `pkg/costtrack`.** That machinery
already exists and stays as it is. It cannot see subscription plans, which is
where the binding limits are: Claude Max, Codex Pro, and Google AI Ultra
enforce rolling and weekly windows that no per-token accounting in this repo
observes, and no dollar figure is attached to them at all.

**Blocking the routing path on a fresh reading.** Correctness for latency at a
5–30 s exchange rate. Rejected; the meter refreshes in the background and
serves a bounded-age cache.

## Consequences

Routing spreads across Claude, Codex/OpenAI, and Gemini in proportion to real
remaining capacity, and CodexBar stays an optional local data source rather than
an authority over credentials or execution.

The costs are honest ones. The parse is pinned to `schemaVersion: 1`; a CodexBar
schema bump stops the reading (safely, and loudly in `quota-meter`) until the
parser is updated, and `pkg/llm/quota/testdata/codexbar-dashboard-live.json` is
the captured real payload that fails first when that happens. Classifying an
error message as an authentication failure is a keyword match, because CodexBar
tags every provider failure as kind `provider`; the match is deliberately broad,
since reporting "auth required" for a genuine outage is far cheaper than
reporting exhaustion for a signed-out account.

Quota readings are host-local. Several dear-agent processes on one machine share
the meter's view but each keep their own cache, so their reorderings agree only
to within a refresh interval. That is sufficient for load spreading and is not
sufficient for a hard budget cap — which this ADR does not attempt.
