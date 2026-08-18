# AGM Sentinel Daemon Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`

<!-- Last audited at: 2026-07-19 -->

## Purpose

`agm/internal/sentinel/daemon` monitors long-running AGM sessions and applies
bounded recovery when a session appears stuck. Recovery must prefer
non-destructive interruption, avoid fighting human operators, and keep repeated
automation inside explicit circuit-breaker limits.

## EARS Requirements

**SENTD-01** When a stuck-session symptom is classified, the system shall choose the least disruptive recovery strategy defined for that symptom.

**SENTD-02** When automated recovery would act on a session with a human present, the system shall downgrade the attempt to manual recovery without sending tmux keys.

**SENTD-03** When recovering a cursor-frozen session, the system shall attempt a flag-based interrupt before escalating to tmux control-key injection.

**SENTD-04** When recovery attempts reach the configured maximum, the system shall block additional attempts until the cooldown permits the history to reset.

**SENTD-05** When a recovery attempt is recorded, the system shall persist the strategy, success value, reason, timestamp, and total-attempt count in the recovery history.

**SENTD-06** When monitor shutdown is requested, the system shall signal the active monitoring run exactly once and wait no longer than five seconds for completion so an unresponsive external probe cannot block daemon shutdown indefinitely.

**SENTD-07** When a tmux socket is explicitly configured, the system shall restrict sentinel discovery and direct recovery actions to that exact socket and shall pass it as `AGM_TMUX_SOCKET` to nested AGM recovery commands without auto-discovering or targeting AGM, legacy, or system tmux sockets.
