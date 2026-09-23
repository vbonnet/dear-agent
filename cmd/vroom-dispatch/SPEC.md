# vroom-dispatch Specification

<!-- Last audited at: 2026-08-28 -->

## Purpose

`cmd/vroom-dispatch` supervises the persistent VROOM control-plane mesh and
bootstraps worker dispatch. It owns the recovery contract for the three
supervisors: Meta-Orchestrator, Orchestrator, and Overseer. Recovery must
preserve provider redundancy, keep detached supervisors executable without
interactive approval prompts, and avoid silently substituting a different
harness when a supervisor is stale.

## EARS Requirements

**VD-01** When `vroom-dispatch` spawns a supervisor, the system shall pass that supervisor's canonical `--harness` and `--model` to `agm session new`.

**VD-02** When a Claude supervisor is spawned and a supervisor model override is configured, the system shall apply the override only to the Claude supervisor.

**VD-03** When a non-Claude supervisor is spawned, the system shall keep the supervisor's canonical model even if a Claude model override is configured.

**VD-04** When a supervisor harness supports startup auto mode, the system shall pass `--mode=auto` to `agm session new` so the detached supervisor can execute its tick without an interactive approval prompt.

**VD-05** When the canonical Overseer is spawned on the `agy` harness, the system shall pass `--mode=auto` so AGM launches AGY with its startup skip-permissions mechanism instead of default prompt-per-command mode.

**VD-06** When a supervisor spawn fails and every AGM failed gate is the recognized recent-spawn stagger or resource-governor pause, the system shall retry within the bounded retry policy instead of permanently dropping that supervisor, waiting until the advertised earliest admission boundary for a governor pause.

**VD-07** When a supervisor spawn fails for an error other than recognized circuit-breaker backpressure, including a refusal that combines transient backpressure with any hard safety gate, the system shall surface the failure and shall not retry blindly.

**VD-08** When supervisor role metadata is present, the system shall pass the matching `--role` flag to `agm session new` so the supervisor receives its RBAC permission profile.

**VD-09** When `vroom-dispatch` invokes `agm session new` for supervisor recovery, the system shall bound the subprocess with an internal timeout so a hung spawn cannot stall the recovery loop indefinitely.

**VD-10** When ready beads remain available while no active workers are reported for the flow-liveness threshold, the system shall raise one `flow_liveness_stall` escalation until flow resumes.

**VD-11** When no ready beads remain or an active worker returns, the system shall reset the flow-liveness timer and duplicate-escalation guard.

**VD-12** When the health monitor queries AGM session health or Beads ready work, the system shall derive a timeout-bounded subprocess context from the monitor context and wrap cancellation, command, and decoding failures.

**VD-13** When `vroom-dispatch` materializes supervisor launch policy, the system shall derive session identity, role, and Primary/Tertiary peers from `pkg/vroom/supervisor` while keeping harness, model, skill, tick interval, and tick prompt policy local to the dispatcher.

**VD-14** When `vroom-dispatch` records a stale supervisor observation, the system shall name the canonical supervisor identity and identify the authoritative AGM supervisor record as the heartbeat source.

**VD-15** When supervisor instructions encounter a permission prompt, the system shall delegate approval only to the typed cross-check classifier and shall not provide a manual approval fallback.

**VD-16** When the Orchestrator selects work, the system shall dispatch directly from Beads through `vroom-dispatch-direct` and shall not consume roadmap, dispatch-ledger, deploy-ledger, or prompt-file projections.

**VD-17** When a supervisor evaluates completion, the system shall require evidence that the change is merged, deployed when applicable, and verified before closing its bead.

**VD-18** When a supervisor session is still registered but its pane shows a provider authentication failure for its configured harness, the system shall archive that broken active session before recreating it through the normal bounded restart path.

**VD-19** When the VROOM-SUP-32 authoritative heartbeat observation is missing, unreadable, or has no timestamp for a registered supervisor session, the system shall classify the supervisor as stale rather than dead or alive.

## BDD traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/stall_detection.feature`
- No BDD change, with reason: the stale-observation payload is a deterministic Dispatch adapter projection pinned by `TestStaleSupervisorTrailDetailsIdentifyAuthoritativeRecord`; no process orchestration or user-visible scenario changes.
- Package tests: `cmd/vroom-dispatch/main_test.go`, `cmd/vroom-dispatch/coverage_test.go`
