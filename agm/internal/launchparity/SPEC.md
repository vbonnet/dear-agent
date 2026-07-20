# Harness Launch Parity Specification

<!-- Last audited at: 2026-07-20 -->

## EARS Requirements

**LAUNCH-PARITY-01** When AGM resolves an active harness launch contract, the system shall return a native interactive startup token for Claude Code, Codex, AGY, and OpenCode.

**LAUNCH-PARITY-02** When automatic or plan mode has a native startup flag, the system shall resolve that flag before launching the harness.

**LAUNCH-PARITY-03** When a persistent session is requested, the system shall omit the pane-shell exit suffix for every active harness.

**LAUNCH-PARITY-04** When Codex starts in automatic mode, the system shall use the supported `-a never` approval policy with a workspace-write sandbox and shall not emit removed convenience flags.

**LAUNCH-PARITY-05** When Codex starts in plan mode, the system shall use a read-only sandbox with the supported `-a untrusted` approval policy.

**LAUNCH-PARITY-06** When post-create startup or final process-tree liveness verification fails, the system shall remove the newly created tmux session, manifest directory, Dolt row, and sandbox rather than reporting an active session.

**LAUNCH-PARITY-07** When startup reaches its final verification gate, the system shall require both a live tmux session and an active harness process before reporting success.

**LAUNCH-PARITY-08** When AGM starts AGY interactively, the system shall invoke bare `agy` and shall not emit `--prompt-interactive` or `--print` unless it also supplies the prompt argument required by those string-valued flags.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
