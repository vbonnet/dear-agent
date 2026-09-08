# SPEC: agm/internal/artifacts/SPEC.md
# RELATED-SPEC: agm/internal/backup/SPEC.md
# RELATED-SPEC: agm/internal/capacity/SPEC.md
# RELATED-SPEC: agm/internal/compaction/SPEC.md
# RELATED-SPEC: agm/internal/deadlock/SPEC.md
# RELATED-SPEC: agm/internal/freshness/SPEC.md
# RELATED-SPEC: agm/internal/lock/SPEC.md
# RELATED-SPEC: agm/internal/monitoring/SPEC.md
# RELATED-SPEC: agm/internal/reservation/SPEC.md
# RELATED-SPEC: agm/internal/state/SPEC.md
# RELATED-SPEC: agm/internal/tracking/SPEC.md
# RELATED-SPEC: agm/internal/tmux/SPEC.md
Feature: AGM runtime package guardrails
  AGM runtime support packages must keep executable SPEC traceability because
  harness parity depends on stable compaction, capacity, lock, state,
  monitoring, backup, reservation, artifact, and failure-tracking behavior.

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
