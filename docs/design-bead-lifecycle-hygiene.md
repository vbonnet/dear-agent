# Design Spike: Bead Lifecycle Hygiene Enforcement

**Bead:** ce-blct · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21

## Problem

A bead's lifecycle has two silent failure modes. **Zombie open beads**: a PR
merges (and deploys) but the bead is never closed, so agents re-investigate
solved problems or over-fix working code. **Premature close**: a bead is closed
the moment its PR merges, before the fix is deployed and verified in prod, so
regressions slip through unnoticed. Both stem from one gap — *PR merge is treated
as terminal* when it is only step 1 of 3.

**Definition of Done (already established):** a bead is DONE only when (1) PR
merged to main, (2) fix deployed (binary in prod), and (3) verified running in
prod (health check / smoke test / human confirmation). Merge ≠ done. This doc
designs the enforcement, not the policy.

The storage split is inherited from [[ce-t8kn]] §2: **metadata = query, trail =
audit**. DoD fields live in queryable `bd` metadata so detection is a `bd list`
sort; every state transition also lands a `kind`-tagged line in
`~/.agm/vroom/trail.jsonl` for the immutable record.

## 1. Zombie Detection

A zombie is a bead that is `OPEN` + has a merged PR + has *no* DoD verification
metadata, aged past a grace window. Detection is a query, run as a step in the
meta-orchestrator's grooming tick (the same `~480-tick`/daily cycle that
[[ce-h59v]] grooming and WSJF sequencing already own — no new infrastructure,
one new step):

```
bd list --status open --json \
  | filter: pr_merged_at present AND deployed_at absent AND (now - pr_merged_at) > 24h
```

`pr_merged_at` is populated by the existing merge webhook / PR-watcher writing
`bd update <id> --set-metadata lifecycle.pr_merged_at=<tick>`. The grace window
(24h) absorbs the normal merge→deploy→verify lag so a freshly-merged bead is not
flagged. Each finding emits `kind: supervisor.meta.zombie_detected` to the trail
and the supervisor either (a) confirms deployment and runs the full close
protocol (§3), or (b) escalates to a human if deploy status is unclear. Steady
state target ([[ce-blct]] success criteria): ≤5 findings per run after two
cycles.

## 2. Premature-Close Prevention

Block the close *at the point of close*, with a defense-in-depth pair:

- **`bd close` gate (`--require-deployed`).** A close hook rejects the close
  unless `lifecycle.deployed_at` and `lifecycle.verified_at` metadata are
  present on the bead. Missing → non-zero exit with a clear message:
  `cannot close ce-XXXX: DoD step 2/3 unverified (deployed_at absent)`. This is
  the cheap, local guard that catches the common "closed on merge" reflex. P0
  beads additionally require `verified_by` to name a human or overseer.
- **Grooming spot-check (backstop).** The hook can be bypassed (`--force`, or a
  close from a context without the metadata). So the meta-orchestrator's grooming
  tick also re-scans every close in the last 48h; any close missing `deployed_at`
  or `verified_at` is **reopened** with a trail note
  (`kind: supervisor.meta.premature_close_reopened`) explaining the missing step.

The gate stops most premature closes synchronously; the spot-check guarantees
eventual correction even when the gate is bypassed.

## 3. Close Protocol Extension

A DoD-complete close carries three fields, written to `bd` metadata (queryable)
and mirrored to the trail (audit), consistent with §2 of [[ce-t8kn]]:

| Field | Type | Meaning |
|---|---|---|
| `lifecycle.deployed_at` | tick/ISO | deployment pipeline confirmed binary in prod |
| `lifecycle.verified_at` | tick/ISO | prod verification (health check / smoke / human) passed |
| `lifecycle.verified_by` | string | `agent:<id>` or `human:<name>` who signed off |

Close command shape:

```
bd close ce-XXXX --require-deployed \
  --set-metadata lifecycle.deployed_at=<tick> \
  --set-metadata lifecycle.verified_at=<tick> \
  --set-metadata lifecycle.verified_by=agent:vroom-overseer \
  --reason "DoD complete: merged #NNN, deployed, verified in prod"
```

The close emits `kind: supervisor.meta.bead_closed` to the trail with the DoD
fields populated — this is the audit record zombie detection (§1) and the
spot-check (§2) read against. Metadata is the source of truth for queries; the
trail line is the immutable copy.

## 4. Worker Enforcement

Workers must be structurally unable to close on merge alone. Two changes to the
worker prompt template:

1. **Prompt language.** The existing "After completing (PR merged)" close
   instruction is rewritten to "After DoD complete (merged **and** deployed
   **and** verified)". Workers are told explicitly: *merge is step 1 of 3; do
   not run `bd close` until you have `deployed_at` and `verified_at` to supply.*
2. **Mandatory checklist step.** The worker's closeout adds a gate: before
   `bd close`, the worker must produce the three §3 fields. If the worker cannot
   verify deployment itself (most workers can't), it does **not** close — it
   hands the bead back to the orchestrator marked `awaiting-deploy-verify`, and
   the supervisor closes it once §3 is satisfiable. This makes "I merged, so I'm
   done" a non-terminal state by construction.

## 5. Orchestrator Role

The **meta-orchestrator** owns hygiene, consistent with grooming not being
delegated to workers ([[ce-h59v]]). Per grooming tick it runs three cheap steps
over the trail + `bd` metadata: zombie scan (§1), premature-close spot-check
(§2), and `awaiting-deploy-verify` drain (§4). Each step is a filter over data
already present — no new store. New trail kinds introduced:
`supervisor.meta.zombie_detected`, `supervisor.meta.premature_close_reopened`,
and `supervisor.meta.bead_closed` (DoD-tagged). The **overseer** is the
escalation target for P0 closes and for any bead whose deploy status the
meta-orchestrator cannot resolve automatically.

## 6. W0 Requirements for the Implementation Bead

1. **`bd close --require-deployed` gate** — close hook that rejects when
   `lifecycle.deployed_at`/`verified_at` are absent. *Acceptance:* a bare close
   of an undeployed bead fails with a clear message; a close with both fields
   succeeds. *Files:* bd close hook / pre-close validator.
2. **`pr_merged_at` population** — PR-watcher writes
   `lifecycle.pr_merged_at` on merge. *Acceptance:* merging a bead's PR sets the
   metadata within one watcher cycle.
3. **Grooming hygiene steps** — three filters in the meta-orchestrator tick
   (§5) emitting the new trail kinds. *Acceptance:* a seeded zombie is flagged;
   a seeded premature close is reopened.
4. **Worker prompt update** — template language + checklist gate (§4) and the
   `awaiting-deploy-verify` handback state. *Acceptance:* a worker prompt no
   longer instructs close-on-merge.
5. **Backfill/audit pass** — one-time scan acting on the Dispatch audit
   findings: close confirmed zombies with a DoD note, reopen premature closes,
   escalate unknown-deploy beads. *Acceptance:* zero open beads with merged PR
   >24h and no `deployed_at` after the pass.
6. **Success metric wiring** — grooming emits a count so "≤5 zombie findings
   per run" and "zero closes without `deployed_at`" are observable in the trail.

## Companion Beads

[[ce-t8kn]] structured spike output (metadata/trail storage split) ·
[[ce-h59v]] grooming cadence & ownership · [[ce-ynyb]] spike pattern adoption ·
WSJF sequencing (`docs/design-wsjf-bead-sequencing.md`, shares the grooming tick).
