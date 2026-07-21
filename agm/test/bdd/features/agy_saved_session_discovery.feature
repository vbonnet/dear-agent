# SPEC: agm/internal/agysession/SPEC.md

Feature: AGY saved-session discovery
  AGM must recover native Antigravity conversation metadata without allowing
  a large or stale provider log directory to become an unbounded lifecycle path.

  Scenario: AGY cache misses use deterministic bounded log discovery
    Given AGY saved-session metadata requires log fallback
    When AGM validates bounded AGY log discovery
    Then AGY cache hits should bypass log discovery
    And AGY log fallback should prefer the newest modification time
    And AGY log fallback should enforce its candidate-file budget
    And AGY log fallback should enforce its per-file byte budget
    And AGY oversized log lines should fail explicitly
