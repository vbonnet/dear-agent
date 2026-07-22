# Harness Launch Parity Specification

<!-- Last audited at: 2026-07-21 -->

## EARS Requirements

**LAUNCH-PARITY-01** When AGM resolves an active harness launch contract, the system shall return a native interactive startup token for Claude Code, Codex, AGY, OpenCode, and Pi.

**LAUNCH-PARITY-02** When automatic or plan mode has a native startup flag, the system shall resolve that flag before launching the harness.

**LAUNCH-PARITY-03** When a persistent session is requested, the system shall omit the pane-shell exit suffix for every active harness.

**LAUNCH-PARITY-04** When Codex starts in automatic mode, the system shall use the supported `-a never` approval policy with a workspace-write sandbox and shall not emit removed convenience flags.

**LAUNCH-PARITY-05** When Codex starts in plan mode, the system shall use a read-only sandbox with the supported `-a untrusted` approval policy.

**LAUNCH-PARITY-06** When post-create startup or final process-tree liveness verification fails, the system shall remove the newly created tmux session, manifest directory, Dolt row, and sandbox rather than reporting an active session.

**LAUNCH-PARITY-07** When startup reaches its final verification gate, the system shall require both a live tmux session and an active harness process before reporting success.

**LAUNCH-PARITY-08** When AGM starts AGY interactively, the system shall invoke bare `agy` and shall not emit `--prompt-interactive` or `--print` unless it also supplies the prompt argument required by those string-valued flags.

**LAUNCH-PARITY-09** When an AGY launch specification has no resolved model, the system shall omit `--model` rather than emitting an empty value, allowing a saved conversation to retain its native selection.

**LAUNCH-PARITY-10** When AGM starts Pi, the system shall pass exact session identity, a private session directory, the managed authorization extension, an AGM-private permission-policy file, explicit project approval, active tools, mode policy, and optional model through one canonical builder without placing allowlist JSON in terminal input.

**LAUNCH-PARITY-11** When AGM resumes Pi, the system shall reuse that canonical builder with the persisted native identity and shall not discover or select a different transcript by modification time.

**LAUNCH-PARITY-12** When AGM constructs a Pi create or resume command, the system shall pass a unique launch ID to the managed extension without reusing the persistent native session ID as process-readiness identity.

**LAUNCH-PARITY-13** When AGM constructs a Pi create or cold-resume command, the canonical builder shall unset any inherited `PI_CODING_AGENT_DIR` from the tmux server and, when the session has an explicit validated coding-agent directory, shall then safely quote and assign that persisted path to the Pi process; when the directory is absent, the cleared environment shall preserve Pi's native default discovery.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
