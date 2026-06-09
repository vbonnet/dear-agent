# DEAR Retro: Handoff Confidence Levels + AGM CLI Drift

**Date:** 2026-05-15
**Severity:** Low (feature shipped clean; one stale governance reference corrected)
**Status:** Resolved — feature merged; CLAUDE.md mandate fixed in the same change

## Define

**The task:** implement "handoff confidence levels" — when an agent
serializes context for a successor (the `/handoff` pattern), it must
express how complete and accurate that context is, so a cold receiver
knows how much to trust it versus re-derive it. Integrate with the DEAR
protocol and the VROOM role system.

**Premise check (before writing code).** The task was framed as
"research is done, this needs an implementation PR." That premise did
**not** hold up:

- The engram-research project `implement-structured-handoff-objects-with-validati`
  is an abandoned Wayfinder stub (status `in_progress`, started
  2025-12-13, every phase `D1..S11` unchecked, zero design/research
  content).
- No research artifact in `~/src/engram-research` covers handoff
  confidence specifically.

The feature was implemented anyway because it stands on its own
evidence, not the missing research doc:

- **Concrete code gap:** `gateway.RoutingDecision` already carries a
  `Confidence float64`, but `HandoffContext` — the object actually
  serialized for a cold pickup — had no confidence field at all.
- **ROADMAP §6.3** ("Confidence scoring per DEAR phase", HIGH) is a real
  prioritized ticket in the same problem space.

**Acceptance criteria** (from `.dear-agent.yml`): `go test ./...`,
`golangci-lint run ./...`, no regressions.

## Execute

- `agm/internal/gateway/handoff_confidence.go` — `HandoffConfidence`
  (level + 0.0–1.0 score + rationale + known gaps), score→level
  derivation with inclusive band floors (0.40 medium, 0.75 high), and
  `Validate()` enforcing level/score consistency and a non-empty
  rationale.
- `HandoffContext` gained `Confidence *HandoffConfidence` plus a
  `SetConfidence` method; nil is rendered as a loud "CONFIDENCE NOT
  ASSESSED" in every handoff template so silence never reads as trust.
- `DeserializeContext` re-validates a confidence block so a tampered or
  hand-edited handoff file is rejected, not silently trusted.
- **VROOM:** new `vroom.decision.handed_off` topic + `HandedOffPayload`
  + `EmitHandedOff`, so every handoff (especially low-confidence ones)
  lands in the append-only decision trail per ADR-020.
- **DEAR:** new declarative `handoff-confidence` acceptance criterion
  type.
- Tests added for all of the above.

## Audit

- `go test` on changed packages (gateway, vroom, acceptance): **pass**.
- `golangci-lint run ./...`: **0 issues**, whole repo.
- Full `go test ./...` showed two failures, both in `agm/internal/tmux`
  / `agm/internal/monitor/tmux` — `GOMAXPROCS=6` expectation on a
  4-core box and a tmux test needing a live Claude prompt. **Confirmed
  pre-existing** by running them on the untouched base commit
  (`9b7104d35d`) in a clean worktree: identical failures, no relation to
  the changed packages. No regression introduced.

## Retro — process gap found

**Gap: the dogfooding mandate referenced a command that does not exist.**
`.claude/CLAUDE.md` instructed agents to run `agm acceptance show` "at
the start of a task". `agm` 2.0.0-dev has no `acceptance` subcommand
(`Error: Unknown command or argument: "acceptance"`). Acceptance criteria
live in `.dear-agent.yml` and are formalized by `pkg/acceptance` — there
is no CLI surface for them. `agm session new` and `agm admin doctor`
(the other commands the mandate names) were verified to exist and work.

This is exactly the failure the mandate's own "acceptable bypass" clause
anticipates: *"the literal bootstrap case where the tool itself is broken
(in which case: file an issue or write a retro before moving on)."*

**Fix wired in this same change** (per the repo's retro-followthrough
discipline — a retro that names a doc/config fix must ship it, not just
propose it):

- `.claude/CLAUDE.md`: replaced `agm acceptance show` with "read the
  `acceptance-criteria:` block of `.dear-agent.yml`"; corrected
  `agm new` → `agm session new`.
- `AGENTS.why.md`: appended a Design Decisions Log entry recording the
  correction and its rationale (config-change convention).

**Lesson:** mandates that name specific CLI invocations rot as the CLI
evolves. The durable instruction is the *mechanism* (`.dear-agent.yml` +
`pkg/acceptance`), not a command string. Prefer pointing at the
authoritative file over a command name in governance docs.
