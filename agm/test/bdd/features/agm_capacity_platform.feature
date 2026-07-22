# SPEC: agm/internal/capacity/SPEC.md

Feature: AGM capacity platform detection
  AGM capacity reporting must use the host's native memory source so the
  operator command remains usable on every supported development platform.

  Scenario: Detect capacity from native memory sources
    Given AGM is running on a supported capacity platform
    When AGM detects the host capacity resources
    Then the capacity detector should report bounded memory
