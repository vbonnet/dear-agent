# SPEC: pkg/override/SPEC.md
# RELATED-SPEC: cmd/override-ledger-append/SPEC.md
# RELATED-SPEC: agm/cmd/override-audit-launchdaemon-installer/SPEC.md
# RELATED-SPEC: agm/cmd/override-audit-systemd-installer/SPEC.md
# RELATED-SPEC: agm/internal/launchdaudit/SPEC.md
# RELATED-SPEC: agm/internal/systemdaudit/SPEC.md
Feature: Dangerous override governance
  Every unattended launch escape hatch that switches off a launch-time safety
  control travels one contract: a stated reason, an expiring human approval, a
  ledger entry, and a recurring audit. The package carries that contract for
  all launch override kinds, so a new kind cannot be added on the side without
  adopting the same gates.

  Scenario Outline: Dangerous override packages declare SPEC coverage
    Given dangerous override package "<package>" is configured
    When AGM validates dangerous override package coverage
    Then dangerous override package "<package>" should have a co-located SPEC

    Examples:
      | package                                         |
      | agm/cmd/override-audit-launchdaemon-installer  |
      | agm/cmd/override-audit-systemd-installer       |
      | agm/internal/launchdaudit                       |
      | agm/internal/systemdaudit                       |
      | cmd/override-ledger-append                      |
      | pkg/override                                    |
