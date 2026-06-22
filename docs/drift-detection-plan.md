# Deployment Drift Detection — Wayfinder Project Plan

> **Routing note.** dear-agent normally routes plans to the engram-research
> knowledge base (`.dear-agent.yml`, CLAUDE.md §Documentation & Artifact
> Routing). This plan is kept in-repo by explicit request because it documents
> an in-repo subsystem (`cmd/drift-check`, `internal/drift`) and is maintained
> in lockstep with that code — i.e. it behaves as living documentation. The
> per-tool living doc is `cmd/drift-check/README.md`.

## W0 — Problem statement

"Merged to main" does not mean "deployed on the host." Several dear-agent
artifacts ship by being *copied or rendered onto the machine*, not by being
imported as Go packages:

| Artifact class            | Deployed to                                   | Source of truth                                  | Deploy command                       |
|---------------------------|-----------------------------------------------|--------------------------------------------------|--------------------------------------|
| Claude Code hooks         | `~/.claude/hooks/*`                            | `agm/cmd/agm/hooks/*` (embedded in agm)          | `agm admin install-hooks`            |
| launchd plists            | `~/Library/LaunchAgents/com.dear-agent.*`      | `cmd/*/templates/*.plist`, `agm/.../schedules/*` | `make install-*-launchagent`, `agm admin install-sweep-schedule` |
| Claude Code settings      | `~/.claude/settings.json`                      | chezmoi source tree                              | `chezmoi-deploy`                     |
| Other chezmoi-managed     | `~/.config/**`, shell rc, git config           | chezmoi source tree                              | `chezmoi-deploy`                     |

When a PR changes one of these but the redeploy step is skipped, the fix lives
in git yet never reaches the machine. This has now happened three times; the
most recent and costly:

- **PR [#456](https://github.com/vbonnet/dear-agent/pull/456)** merged a gopls
  reaper into the stop hook (`stop-agm-resource-cleanup`). The hook was never
  redeployed via `agm admin install-hooks`. For two days the host kept leaking
  gopls processes (ce-710r) — the deployed hook on disk was 800 bytes while the
  merged source was 2619 bytes. The fix existed in main the entire time.

The failure mode is invisible: CI is green, the bead is closed, the PR is
merged — and the machine is still broken. There is no signal anywhere that the
deployed copy diverged from main.

### Goal

A cheap, fast, build-free check that detects when a deployed artifact no longer
matches its source of truth in main, surfaces it loudly, and (over later
phases) prevents a fix-bead from closing until the fix is provably *deployed*
and *running*.

### Non-goals

- Not a general configuration-management system (chezmoi already owns dotfile
  deployment). drift-check *detects* drift; it does not *own* deployment.
- Phase 0–2 do not prove a code path executed at runtime — that is the Phase 3
  "verified" gate, which needs OTel.

## Three-gate lifecycle model

The root cause is that our Definition of Done stops at **merged**. The fix is to
extend the lifecycle of any bead that ships via a deployed artifact to a
three-gate chain — each gate strictly stronger than the last:

```
  merged          deployed                 verified
  ───────         ────────                 ────────
  PR is MERGED →  artifact hash on host  → the fixed code path is observed
  to main        == source hash in main    running in production (log/trace)
  (today's        (drift-check: Phase 0-2)  (OTel: Phase 3)
   bead-close-
   guard)
```

A bead for a deployed-artifact fix MUST NOT close until **all three** gates
pass. The existing `bead-close-guard` enforces only `merged`; Phases 2 and 3
add `deployed` and `verified`.

## Phased delivery

### Phase 0 — Manual drift-check script *(this PR)*

A Go tool `cmd/drift-check` that hashes each deployed artifact against its
source of truth and reports DRIFT on mismatch.

- `internal/drift` — pure, testable core: `Check(ctx, Config, Options) Report`.
  Hashes (SHA-256) the deployed file and the source file; reports `ok`,
  `drift`, `missing_deployed`, `missing_source`, or `error` per target. Token
  substitution renders templated sources (plists) into their installed form so
  the compare is meaningful. Optional `--git-ref` compares against committed
  `origin/main` via `git show` rather than the working tree.
- `cmd/drift-check` — thin CLI. Text output for humans; `--json` for
  OTel/monitoring; `--audit` appends to the JSONL audit log. Exit 2 on drift.
- `internal/driftaudit` — JSONL audit at
  `~/.local/state/dear-agent/drift-check.log`, same durable-trail contract as
  safe-pr / src-recovery.
- Default deploy targets embedded as `cmd/drift-check/targets.yaml`.
- `make drift-check`, `make build-drift-check`, `make install-drift-check`.

**Immediate value:** running it today flags the still-stale `#456` stop hook.

**Acceptance:** `go test ./internal/drift/... ./internal/driftaudit/...` green;
`drift-check --json` emits schema `drift-check/v1`; exit 2 when a deployed file
differs from source.

### Phase 1 — Integrate into the daily ops audit

Wire drift-check into the daily scheduled audit so drift is surfaced once a day
without anyone remembering to run it.

- Add a `drift` check to the `audits.schedule.daily` set in `.dear-agent.yml`
  (or invoke `drift-check --json --audit` from the daily launchd job).
- Install drift-check as its own launchd job
  (`com.dear-agent.drift-check.plist`, mirroring bumblebee) for a standalone
  daily run that writes the JSONL audit and exits non-zero on drift.
- The daily-ops summary reads the latest `drift-check.log` line and includes a
  one-line drift status (`✓ no drift` / `⚠ 2 artifacts drifted`).

**Acceptance:** a drifted artifact appears in the daily audit output and in
`drift-check.log` within 24h with no manual action.

### Phase 2 — Bead-close-guard "deployed" gate *(delivered)*

`bead-close-guard` now runs a second gate after `merged`: a bead whose merged
change touched a deployed artifact cannot close until that artifact is current
on the host.

- **No manual marker needed.** The gate reads each merged PR's changed files
  (`gh pr view --json files`) and maps them to deploy targets by matching the
  changed path against `targets.yaml`'s `source`. A bead "ships" a target
  exactly when its merged code touched that target's source — automatic, so the
  enforcement does not depend on anyone remembering to label the bead.
- On `bd close`, after the `merged` check, the guard runs `drift.Check` for the
  matched target(s). Any `drift` (stale on host) or required `missing` (never
  installed) → block close (exit 2) with a message naming the remediation
  command (principle 2: teach the fix). Optional targets a machine has not
  installed do not block.
- Reuses `internal/drift` directly — no shelling out — so the guard stays one
  binary with no new runtime deps. The target list is read from the repo's
  single `cmd/drift-check/targets.yaml`, so drift-check and the gate never
  diverge.
- Best-effort on its own infrastructure: when no dear-agent checkout /
  deploy-target config is reachable where the close runs (resolved from the git
  toplevel of cwd, or `--repo-root`), the gate records *why it skipped* and
  allows the close rather than blocking it. A genuine drift always blocks.
- `--abandon-reason` still overrides both gates for genuinely abandoned beads,
  audit-logged.

**Acceptance:** closing a bead whose merged change left a deployed artifact
stale (or never installed) is blocked with exit 2 and a remediation hint;
closing it after redeploy succeeds. *(Met: see
`cmd/bead-close-guard/deploygate_test.go`.)*

### Phase 3 — Runtime "verified" gate (needs OTel)

`deployed == source` proves the bytes reached disk, not that the fixed code
*ran*. The verified gate confirms execution.

- The fixed code path emits an OTel span/log marker (dear-agent already has
  `pkg/otelsetup` + `internal/telemetry`; see the OTel-state memory). For the
  stop hook, the reaper emits a span when it reaps.
- A `drift-check verify --since <ts> --marker <name>` subcommand queries the
  trace/log backend (Jaeger/JSONL) for that marker after the deploy timestamp.
- bead-close-guard's third gate calls it: no observed execution → bead stays
  `in_progress`.

**Acceptance:** a deployed-but-never-exercised fix does not satisfy the
verified gate; once the code path is observed in a trace, it does.

### Phase 4 — Auto-remediation

Close the loop: when drift is detected for an artifact with a known, safe
remediation command, run it.

- Each target already carries a `remediation` command. Phase 4 adds an opt-in
  `drift-check --remediate` that runs the command for drifted targets, re-checks,
  and audits the before/after.
- Guarded by the atomic-wrapper discipline (CLAUDE.md principle 9): only
  whitelisted, idempotent install commands; never a raw destructive step.
  Remediation that touches `~/src` or chezmoi routes through the existing
  sanctioned wrappers (`src-recovery`, `chezmoi-deploy`).
- Runs in attended mode first; unattended auto-remediation is gated behind an
  explicit per-target `auto_remediate: true` and the Defer-Don't-Block protocol.

**Acceptance:** `drift-check --remediate` redeploys a drifted hook and the
follow-up check reports clean; the action is in the audit log.

## Notification mechanism (spans Phases 1–2)

When drift is detected:

1. **Bead** — file/update a drift bead in context-engine
   (`bd --db ~/beads/context-engine/.beads create`) naming the artifact and its
   remediation command, so the work is tracked (CLAUDE.md principle 8). One
   bead per artifact, updated (not duplicated) on subsequent detections.
2. **Audit JSONL** — every run appends to
   `~/.local/state/dear-agent/drift-check.log` (Phase 0, `--audit`), the durable
   trail monitoring and the daily summary read.
3. **Daily ops audit** — the daily summary includes the current drift status
   (Phase 1).

Phase 0 ships (2) and the `remediation`-bearing JSON that (1) and (3) consume;
(1) and (3) land in Phase 1 alongside the scheduled run.

## Risks & mitigations

- **Templated artifacts cause false positives.** Plists embed `__USER_HOME__`
  etc. → mitigated by per-target `tokens` that render the source before hashing.
- **Binary hooks** (e.g. `agm-pretool-test-session-guard`, a compiled binary)
  drift on every rebuild and are not byte-stable. Excluded from the default
  config; a build-aware comparison (compare the embedded source, or version
  stamp) is deferred past Phase 0.
- **Source-of-truth outside the repo** (chezmoi-managed settings.json). The
  default config ships these commented with guidance to point `source` at the
  chezmoi checkout; full chezmoi coverage is a Phase 1 follow-up.
- **`git show` vs working tree.** Default compares the working tree (correct
  when run from the golden `~/src/dear-agent` main checkout); `--git-ref
  origin/main` compares committed main explicitly for other contexts.

## Tracking

File the epic and per-phase beads in context-engine
(`bd --db ~/beads/context-engine/.beads`). Phase 0 closes only when its PR is
**merged** to main (CLAUDE.md §6). Drift detected against the host (e.g. the
`#456` stop hook) gets its own remediation bead — it is not in scope of the
Phase 0 PR.
