# AGM Safety Guards Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`agm/internal/safety` owns guardrail checks that prevent AGM from sending input
into unsafe or unready harness panes. The package keeps harness readiness and
human-typing detection out of command handlers so delivery policy can remain
consistent across Claude Code, Codex CLI, AGY, OpenCode, and Pi.

## EARS Requirements

**SAFE-01** When AGM checks a session before delivery, the system shall report a violation when the selected harness has not reached an interactive or working state.

**SAFE-02** When tmux readiness text is ambiguous for Codex CLI, AGY, OpenCode, or Pi, the system shall inspect the pane process tree before classifying the harness as uninitialized.

**SAFE-03** When Codex CLI output contains startup, model, composer, or active-work indicators, the system shall not classify the session as uninitialized solely because the final composer prompt is absent.

**SAFE-04** When AGY output contains trust, prompt, or active-work indicators, the system shall not classify the session as uninitialized solely because the final prompt marker is absent.

**SAFE-05** When human typing is detected in a target pane, the system shall report a non-blocking advisory, preserve the composer through the harness-specific stash path, and continue delivery without treating the heuristic as a safety violation.

**SAFE-06** When the pane process tree is inspected for harness liveness, the system shall use the shared full-descendant-tree scan from `agm/internal/tmux` so a harness running as a grandchild under a shell (crash-resume) is still detected.

**SAFE-07** When Codex CLI liveness is checked, the system shall treat either a `codex` process or a `node` wrapper process in the pane tree as evidence that the harness is running.

**SAFE-08** When Pi delivery safety is checked, the system shall require Pi-specific process identity, including the canonical npm Node entrypoint without accepting generic Node, in the pane tree and the latest AGM-managed Pi state to be `ready` or `working` rather than requiring Claude process or prompt evidence.

**SAFE-09** When OpenCode delivery safety is checked, the system shall require an exact `opencode` process and OpenCode composer, product, or active-work evidence rather than requiring Claude process or prompt evidence.

## BDD Traceability

- `agm/test/bdd/features/harness_parity.feature`

## Package Test Traceability

- `agm/internal/safety/safety_test.go`
