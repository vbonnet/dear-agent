# AGM Session Recovery Specification

<!-- Last audited at: 2026-07-10 -->

## Purpose

`agm/internal/recovery` verifies that soft recovery changes the target session's
process state. It prevents pane readability or ready-looking terminal chrome
from being reported as recovery while a wedged harness child remains alive.

## EARS Requirements

**RECOVERY-01** When soft recovery begins, the system shall snapshot the target tmux pane's full descendant process tree.

**RECOVERY-02** When a work-process leaf existed before a recovery keypress, the system shall report recovery only after at least one pre-existing work-process PID exits.

**RECOVERY-03** When no work-process leaf existed before a recovery keypress, the system shall require a verified ready prompt before reporting recovery.

**RECOVERY-04** When pane capture succeeds without a work-process exit or verified ready prompt, the system shall report recovery as unconfirmed.

**RECOVERY-05** When AGY does not forward terminal recovery keys, the system shall fall back to sending SIGINT only to non-harness leaf processes in that session's pane subtree.

**RECOVERY-06** When process-level fallback runs, the system shall not signal the tmux pane root, harness runtime, intermediate shell, or another session's processes.

**RECOVERY-07** When Claude Code, Codex CLI, or OpenCode recovery remains unconfirmed, the system shall avoid process-level escalation unless that harness has an explicitly verified fallback.

**RECOVERY-08** When recovery waits between signals or before process-state confirmation, the system shall return promptly if the command context is canceled.

**RECOVERY-09** When AGY process-level fallback signals work leaves, the system shall use the operating system process API and report each signal failure without signaling invalid process identifiers.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/recovery/recovery_test.go`
