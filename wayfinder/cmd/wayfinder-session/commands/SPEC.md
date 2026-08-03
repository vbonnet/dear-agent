# Wayfinder command requirements specification

<!-- Last audited at: 2026-07-17 -->

**Status:** Active
**Scope:** Operator-facing Cobra commands under `wayfinder session`.

## EARS requirements

**WFCMD-01** When the session command tree is built, the system shall register start, status, next-phase, start-phase, complete-phase, rewind-to, end, lifecycle, task, coordination, and sandbox commands.

**WFCMD-02** When a command receives a phase, the system shall accept only a named canonical phase.

**WFCMD-03** When a command receives an invalid outcome, project type, risk, task state, or lifecycle state, the system shall reject it before persistence.

**WFCMD-04** When `start --force` is requested, the system shall require a non-empty reason.

**WFCMD-05** When a command changes lifecycle state, the system shall use canonical status parsing and atomic persistence.

**WFCMD-06** When help is rendered, the system shall describe only registered commands, flags, and the named phase sequence.

**WFCMD-07** When AI-facing README or skill shell examples are committed, the repository shall parse each Wayfinder invocation against the active Cobra command and flag tree.

**WFCMD-08** When retired compatibility commands are requested, the system shall report that they are unknown.

**WFCMD-09** When SETUP starts, or when BUILD starts after SETUP was skipped, the system shall create a tracking bead if none exists and shall preserve the phase transition when tracker creation is unavailable.

**WFCMD-10** When required archive, trace, or Git commit persistence for a rewind fails, the system shall return an explicit error and shall not report the rewind operation as successful.

**WFCMD-11** When `session start` receives a project directory outside a Git work tree, the system shall reject the request before creating any lifecycle artifact.

**WFCMD-12** When a deterministic pre-rewind archive input is invalid, the system shall reject the rewind before mutating canonical status or appending rewind trace evidence.

**WFCMD-13** While a rewind transition is in progress for a project, the system shall reject a concurrent rewind before parsing or mutating that project's lifecycle state.

## Traceability

- Command tests: `wayfinder/cmd/wayfinder-session/commands/*_test.go`
- Registration tests: `wayfinder/cmd/wayfinder/cmd/session_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
