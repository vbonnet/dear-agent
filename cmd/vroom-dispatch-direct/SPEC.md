# vroom-dispatch-direct Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/vroom-dispatch-direct` dispatches worker sessions straight from `bd ready`
with no intermediate prompt-file layer. Dispatch state is derived from ground
truth (live `worker-<id>` sessions and open PRs), capacity is controlled by
real spawn backpressure, and session names are tmux-safe so a dotted bead id
can never brick a dispatch run (ce-b1zw).

## EARS Requirements

**VDD-01** When a dispatch run starts, the system shall read ready beads from `bd ready --json` and order eligible candidates by priority (P0 first) then id.

**VDD-02** The system shall not dispatch a bead that is human-gated, already has a live worker session, or is already in flight in an open PR.

**VDD-03** When a worker session is spawned for a bead, the system shall derive the session name from the tmux-safe form of the bead id (dots, colons, and spaces normalized to dashes) so agm's tmux-safety check never falls into an interactive prompt.

**VDD-04** When live-worker deduplication compares bead ids against `agm session list` output, the system shall normalize both sides of the comparison so a sanitized session name dedups its dotted bead id and a legacy dotted session name dedups the same bead.

**VDD-05** If a spawn fails with a deterministic per-bead error (unsafe or invalid session name, invalid bead, or an interactive prompt with no TTY), then the system shall log the failure, skip that bead, and continue dispatching the remaining candidates.

**VDD-06** If a spawn fails with circuit-breaker backpressure, a timeout, or an unrecognized error, then the system shall stop the dispatch run so the remaining beads retry on a later run.

**VDD-07** When a worker session spawns successfully, the system shall send the rendered work prompt, which references the original (unnormalized) bead id, to the sanitized session name.

**VDD-08** While an `agm session new` spawn is in flight, the system shall bound it with a spawn-specific 180-second timeout, keeping the tighter 60-second bound for bd, gh, and agm-list subprocess calls.

**VDD-09** When a run dispatches zero beads, the system shall exit with status 0, treating a drained or fully in-flight backlog as a normal steady state.

**VDD-10** When `-max-dispatch` is set to a positive N, the system shall dispatch at most N candidates in that run, counting only successful dispatches toward N so a deterministically-skipped bead cannot consume the budget, and shall leave the remaining eligible candidates for a later run.

**VDD-11** When `-max-dispatch` is 0 or unset, the system shall preserve unlimited dispatch (dispatch every eligible candidate, bounded only by spawn backpressure).

**VDD-12** When `-max-dispatch` is negative, the system shall exit with an error rather than treating the misconfigured cap as unlimited.

**VDD-13** When a run summary is reported, the system shall report the total eligible candidate count rather than the capped dispatch count.

**VDD-14** While determining whether a bead is human-gated, the system shall consult the shared `internal/vroomgate` list rather than a command-local copy.
