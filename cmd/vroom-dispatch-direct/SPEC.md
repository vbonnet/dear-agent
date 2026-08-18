# vroom-dispatch-direct Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/vroom-dispatch-direct` dispatches worker sessions straight from `bd ready`
with no intermediate prompt-file layer. Dispatch state is derived from ground
truth (occupied non-archived `worker-<id>` names and open PRs), capacity is
controlled by real spawn backpressure, and session names are tmux-safe so a
dotted bead id can never brick a dispatch run (ce-b1zw).

## EARS Requirements

**VDD-01** When a dispatch run starts, the system shall read ready beads from `bd ready --json` and order eligible candidates by priority (P0 first) then id.

**VDD-02** The system shall not dispatch a bead that is human-gated, already owns a non-archived worker session name, or is already in flight in an open PR.

**VDD-03** When a worker session is spawned for a bead, the system shall derive the session name from the tmux-safe form of the bead id (dots, colons, and spaces normalized to dashes) so agm's tmux-safety check never falls into an interactive prompt.

**VDD-04** When worker-name deduplication compares bead ids against `agm session list` output, the system shall normalize both sides of the comparison so a sanitized session name dedups its dotted bead id and a legacy dotted session name dedups the same bead.

**VDD-05** If a spawn fails with a deterministic per-bead error (unsafe or invalid session name, invalid bead, or an interactive prompt with no TTY), then the system shall log the failure, skip that bead, and continue dispatching the remaining candidates.

**VDD-06** If a spawn fails with circuit-breaker backpressure, a timeout, or an unrecognized error, then the system shall stop the dispatch run so the remaining beads retry on a later run.

**VDD-07** When a worker session spawns successfully, the system shall send the rendered work prompt, which references the original (unnormalized) bead id, to the sanitized session name.

**VDD-08** While an `agm session new` spawn is in flight, the system shall bound it with a spawn-specific 180-second timeout, keeping the tighter 60-second bound for bd, gh, and agm-list subprocess calls.

**VDD-09** When a run dispatches zero beads, the system shall exit with status 0, treating a drained or fully in-flight backlog as a normal steady state.

**VDD-10** When an operator selects a worker harness, model, mode, or workspace, the system shall pass that configuration to `agm session new` without changing candidate selection or the worker completion contract.

**VDD-11** When the system renders a worker prompt, the prompt shall remain harness-neutral and shall route the worker through the canonical Wayfinder V2 lifecycle.

**VDD-12** When the system renders a worker completion contract, the prompt shall require evidence that the change is merged, deployed when applicable, and verified, and shall require the worker to record exactly one terminal outcome (DONE, DONE_WITH_CONCERNS, or FAILED) as the first token of its final bead note.

**VDD-13** If a fail-closed ground-truth query (`bd ready`, `agm session list`, `gh pr list`) fails, then the system shall retry it with backoff before treating the failure as fatal.

**VDD-14** When a fail-closed query is fatal, the system shall persist the failure to a heartbeat file (`-heartbeat-file`, default `~/.agm/vroom/heartbeat/dispatch-direct.json`) recording the consecutive-failure streak and the last error, and shall reset that streak to zero on the next run that completes without a fail-closed halt.

**VDD-15** When the persisted consecutive-failure streak reaches the alert threshold, the system shall escalate loudly — an unmissable stderr banner plus a best-effort desktop notification — rather than continuing to log a terse one-line failure indistinguishable from a healthy tick.

**VDD-16** When a dispatch run starts, the system shall load a persistent dispatch ledger recording every bead a prior run has dispatched a worker for, so a bead this tool never dispatched is never reconciled.

**VDD-17** When a ready bead the ledger shows was previously dispatched currently has no live worker session, the system shall determine bead closure deterministically rather than relying on the worker having run any command, closing the bead as done when a merged pull request mentioning it is found via `gh pr list --state merged` regardless of the bead's notes.

**VDD-18** If no open or merged pull request exists for a previously-dispatched, worker-less bead, the system shall read the bead's notes for the mandated terminal-outcome token: DONE or DONE_WITH_CONCERNS shall close the bead as a no-op ("already satisfied"); FAILED shall move the bead to bd's `blocked` status so it is excluded from `bd ready` until a human clears it.

**VDD-19** If a previously-dispatched, worker-less bead has no open or merged pull request and no recognizable terminal-outcome token in its notes, the system shall record a no-progress strike, and once a bead accumulates a configured number of consecutive no-progress strikes with no open pull request in between, the system shall move it to bd's `blocked` status instead of leaving it eligible for further redispatch.

**VDD-20** When an open pull request exists for a bead with accumulated no-progress strikes, the system shall reset its strike count to zero, treating an open PR as evidence of real progress.

**VDD-21** When a dispatch run reconciles (closes or blocks) a bead, the system shall exclude it from that same run's candidate selection, and in dry-run mode the system shall report what reconciliation would do without closing, blocking, or persisting any ledger mutation.

**VDD-22** When worker-name deduplication reads session state, the system shall treat active, running, zombie, and stopped worker sessions as occupied because AGM rejects duplicate non-archived names; archived sessions shall not suppress dispatch.

**VDD-23** When session inventory exceeds one AGM list page, the system shall retrieve every page before candidate selection so an older occupied worker name cannot fall outside deduplication.

**VDD-24** When a worker is selected for dispatch, the trusted host shall create task-owned bare Git state and linked dear-agent and engram-research worktrees, borrow only immutable base objects from the read-only source repositories, and grant the worker only those task-owned paths plus the canonical Beads database.

**VDD-25** When the host creates task-owned Git state, the trusted host shall retain the source repository's origin URL and commit identity, seed the source origin/main commit as the worker base, and keep source repository Git control files outside the worker grant.

**VDD-26** When VROOM launches AGM for a prepared worker, the dispatcher shall bind the exact add-directory payload and, for Codex, the system-managed worker guard path to that session name through a one-launch trusted handoff.

**VDD-27** When an operator supplies `-prepare-worker`, the dispatcher shall prepare that bead's same production workspace without dispatching and print the session name, add directories, and applicable managed guard path as JSON for recovery of an existing session.

**VDD-28** When `-max-dispatch` is set to a positive N, the system shall dispatch at most N candidates in that run, counting only successful dispatches toward N so a deterministically-skipped bead cannot consume the budget, shall leave the remaining eligible candidates for a later run, and shall report the total eligible count rather than the capped count in the run summary.

**VDD-29** When `-max-dispatch` is 0 or unset, the system shall preserve unlimited dispatch (every eligible candidate, bounded only by spawn backpressure), and when it is negative the system shall exit with an error rather than treating the misconfigured cap as unlimited.

**VDD-30** While determining whether a bead is human-gated, the system shall consult the shared `internal/vroomgate` list rather than a command-local copy.
