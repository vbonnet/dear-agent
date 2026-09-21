# SPEC: engram/internal/scratchpad/SPEC.md
Feature: Engram scratchpad bind visibility
  Scratchpad construction should prove that the Docker daemon observes the
  intended host workspace before any interpreter receives probe code.

  Scenario: Scratchpad construction verifies bind identity before execution
    When AGM runs the deterministic scratchpad bind visibility regressions
    Then scratchpad construction should require unchanged bind contents and roll back failed creation
