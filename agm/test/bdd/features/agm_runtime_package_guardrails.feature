# SPEC: agm/internal/artifacts/SPEC.md
# RELATED-SPEC: agm/internal/backup/SPEC.md
# RELATED-SPEC: agm/internal/capacity/SPEC.md
# RELATED-SPEC: agm/internal/compaction/SPEC.md
# RELATED-SPEC: agm/internal/deadlock/SPEC.md
# RELATED-SPEC: agm/internal/freshness/SPEC.md
# RELATED-SPEC: agm/internal/lock/SPEC.md
# RELATED-SPEC: agm/internal/monitoring/SPEC.md
# RELATED-SPEC: agm/internal/ops/SPEC.md
# RELATED-SPEC: agm/internal/reservation/SPEC.md
# RELATED-SPEC: agm/internal/session/SPEC.md
# RELATED-SPEC: agm/internal/state/SPEC.md
# RELATED-SPEC: agm/internal/tracking/SPEC.md
# RELATED-SPEC: agm/internal/tmux/SPEC.md
Feature: AGM runtime package guardrails
  AGM runtime support packages must keep executable SPEC traceability because
  harness parity depends on stable compaction delivery, accounting, observation,
  verification, capacity, lock, state, monitoring, backup, reservation,
  artifact, and failure-tracking behavior.

  Scenario Outline: AGM runtime packages declare SPEC coverage
    Given AGM runtime package "<package>" is configured
    When AGM validates AGM runtime package coverage
    Then AGM runtime package "<package>" should have a co-located SPEC

    Examples:
      | package                  |
      | agm/internal/artifacts   |
      | agm/internal/backup      |
      | agm/internal/capacity    |
      | agm/internal/compaction  |
      | agm/internal/deadlock    |
      | agm/internal/freshness   |
      | agm/internal/lock        |
      | agm/internal/monitoring  |
      | agm/internal/reservation |
      | agm/internal/state       |
      | agm/internal/tracking    |

  Scenario: Session observations preserve provenance for readiness decisions
    When AGM runs the compaction observation provenance regressions
    Then non-live session observations should remain distinguishable

  Scenario: Delivery authorization rejects ambiguous evidence even under force
    When AGM runs the compaction delivery authorization regressions
    Then compaction delivery should require positive live-ready evidence

  Scenario: Preserve state is bound to durable session identity
    When AGM runs the compaction state ownership regressions
    Then reused display-name state should require exact stable ownership

  Scenario: Registered compaction delivery is durably accounted before submission
    When AGM runs the durable compaction accounting regressions
    Then compaction attempts should be keyed by stable identity and counted conservatively

  Scenario: Positive completion requires an active transition and stable readiness
    When AGM runs the positive compaction verification regressions
    Then compaction completion should require active then stable ready evidence

  Scenario: Lost evidence and timeout remain unverified
    When AGM runs the unverified compaction regressions
    Then ambiguous compaction outcomes should return non-success

  Scenario: Command wording distinguishes sent from completed
    When AGM runs the compaction command reporting regressions
    Then command output should claim completion only after positive proof

  Scenario: Uncertain or incompletely accounted delivery forbids replay
    When AGM runs the compaction delivery outcome error regressions
    Then compaction delivery errors should prohibit automatic retry and completion wording
