# DEAR Retro: Backlog Suggestion System

**Date:** 2026-05-15
**Severity:** Low (greenfield feature; the gap it closes was latent, not an incident)
**Status:** Resolved — `pkg/backlog` + `cmd/backlog-suggest` shipped, format gap fixed in the same change

This is the **Audit + Retro** half of the DEAR loop for the
framework-improvement item "backlog suggestion system" (BACKLOG.md
DEAR-X.9, P2). The **Define** half is
[ADR-022](../adrs/ADR-022-backlog-suggestion-system.md).

## Define

**The invariant:** the rule stated at the top of `BACKLOG.md` — *"find the
lowest-numbered `pending` ticket in the active phase whose `Dep` column is
satisfied (all listed tickets are `done`)"* — SHOULD be executable and
audited, not an unenforced human convention.

**Today's snapshot, before this session:** the rule had governed the entire
6-phase workflow-engine project as prose only. Nothing parsed the backlog;
ticket pickup produced no `vroom.decision.dispatched` row, so the
Overseer/Meta-Orchestrator could not see *which* item was chosen or *why* —
directly contradicting the dogfooding/audit mandate in CLAUDE.md.

## Enforce

**What broke (process, not code):**

Two gaps, both latent because the project had enough humans-in-the-loop to
paper over them:

1. **Unenforced selection rule.** "Pick the next eligible pending ticket"
   was judgment-dependent. Same failure family as the 2026-05-09/05-10 CI
   retros: a deterministic rule left as a list of instructions drifts.
2. **Format gap surfaced by the new tool.** Running `backlog-suggest`
   against the *real* `BACKLOG.md` immediately exposed that the cross-phase
   table (`| # | Title | Notes |`) has **no Status column**, so genuinely
   pending future work (DEAR-X.7 `spec.staleness`, DEAR-X.8
   `spec.conformance`) parsed as `unknown` and was invisible. The tool
   found a real defect in its own input on first contact — the point of
   dogfooding.

## Audit

**Actions executed this session (in order):**

1. Explored the architecture (VROOM roles, aggregator/recommendation-MCP,
   markdown backlog formats, CLI conventions, agm ADR-023 Orchestrator
   dispatch contract).
2. Wrote ADR-022 (Define) before code.
3. Shipped `pkg/backlog`: `Item` model, header-aware `MarkdownSource`
   (reads both the 7-column BACKLOG.md and 4-column ROADMAP.md layouts),
   `Ranker` (eligibility + priority + leverage + effort, mirroring agm
   ADR-023 § Task Dispatch), `Suggester`, `OrchestratorNotifier`
   (synchronous `vroom.decision.dispatched`, not the goroutine `Emitter` —
   a fire-once CLI would `os.Exit` before the async publish flushed).
4. Shipped `cmd/backlog-suggest` (`list` / `suggest`, `--emit-vroom`).
5. 31 tests, `pkg/backlog` 94.9% / CLI 81.4% coverage; `golangci-lint`
   clean; full `go build ./...` green.
6. **Wired the format fix in the same change** (not deferred): added a
   `Status` column to the cross-phase table so X.1–X.4/X.7/X.8 are now
   machine-visible as `pending`. Dogfood re-run confirms the tool now
   surfaces them.

## Retro

**What went right — Define before Execute.** Writing ADR-022 first forced
the realization that this is the *task-driven* twin of the *metric-driven*
ADR-015/016 surface, and that ranking should *reproduce* the existing agm
ADR-023 Orchestrator dispatch rules rather than invent a new scheme. The
ranker is therefore a faithful pre-dispatch decision, not a parallel
heuristic.

**The forcing function held.** Memory `dear-agent-retro-followthrough.md`
says a retro proposing a fix must ship the patch in the same change. The
cross-phase Status-column gap could have been written up as a TODO; instead
it shipped here (one-line-per-row doc edit) and was verified by re-running
the tool. Design + wiring in one change.

**Meta-pattern:** dogfooding caught the input-format defect on first
contact. A tool that consumes the project's own conventions is also an
auditor of them — "punted/deferred" had been conflated with a *status* when
it is a *priority* signal.

## Action items (status)

| # | Action | Status |
|---|--------|--------|
| 1 | `pkg/backlog` + `cmd/backlog-suggest` (executable selection rule) | ✅ shipped (this change) |
| 2 | `vroom.decision.dispatched` on pickup (decision trail) | ✅ shipped (this change) |
| 3 | Cross-phase table Status column (format-gap fix) | ✅ shipped (this change) |
| 4 | Wire `backlog-suggest` into the recommendation MCP / Orchestrator scan loop | TODO — follow-up; the CLI + library seam is ready |
| 5 | Add a second `Source` (Asana / issue tracker) when a driver exists | deferred — interface in place, no adapter until needed (YAGNI) |
