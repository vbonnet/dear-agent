# AGM Safety Guards Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/safety` owns guardrail checks that prevent AGM from sending input
into unsafe or unready harness panes. The package keeps harness readiness and
human-typing detection out of command handlers so delivery policy can remain
consistent across Claude Code, Codex CLI, AGY, and other tmux-backed harnesses.

## EARS Requirements

**SAFE-01** When AGM checks a session before delivery, the system shall report a violation when the selected harness has not reached an interactive or working state.

**SAFE-02** When tmux readiness text is ambiguous for Codex CLI or AGY, the system shall inspect the pane process tree before classifying the harness as uninitialized.

**SAFE-03** When Codex CLI output contains startup, model, composer, or active-work indicators, the system shall not classify the session as uninitialized solely because the final composer prompt is absent.

**SAFE-04** When AGY output contains trust, prompt, or active-work indicators, the system shall not classify the session as uninitialized solely because the final prompt marker is absent.

**SAFE-05** When human typing is detected in a target pane, the system shall return a safety violation instead of sending automated input over active human input.

**SAFE-06** When the pane process tree is inspected for harness liveness, the system shall use the shared full-descendant-tree scan from `agm/internal/tmux` so a harness running as a grandchild under a shell (crash-resume) is still detected.

**SAFE-07** When Codex CLI liveness is checked, the system shall treat either a `codex` process or a `node` wrapper process in the pane tree as evidence that the harness is running.

## BDD Traceability

- `agm/test/bdd/features/harness_parity.feature`

## Package Test Traceability

- `agm/internal/safety/safety_test.go`
