# SPEC: agm/internal/permissionparity/SPEC.md
# RELATED-SPEC: agm/internal/manifest/SPEC.md
# RELATED-SPEC: agm/internal/rbac/SPEC.md
# RELATED-SPEC: agm/internal/permissionparity/piadapter/SPEC.md
# RELATED-SPEC: .pi/SPEC.md
Feature: Agent permission parity
  AGM should resolve one agent permission policy and carry it through every
  active harness. Harnesses may enforce the policy through different native
  controls, but none of the active harnesses should be undocumented or missing
  a permission surface.

  Scenario Outline: Active harnesses publish permission policy surfaces
    Given harness "<harness>" is configured
    When AGM validates permission parity support
    Then harness "<harness>" should have a permission policy target
    And harness "<harness>" should have a startup permission surface

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario Outline: Pi permission modes enforce native tool-call decisions
    Given Pi permission mode "<mode>" with policy "<policy>"
    When Pi requests tool "<tool>" with input "<input>" in an interactive session
    Then the Pi permission decision should be "<decision>"

    Examples:
      | mode    | policy          | tool  | input      | decision |
      | plan    | Bash(git status)| bash  | git status | block    |
      | default | Bash(git status)| bash  | git status | allow    |
      | default |                  | write | /tmp/x     | ask      |
      | auto    |                  | write | /tmp/x     | allow    |

  Scenario: Permission profiles resolve across the active harness set
    Given permission profile "worker" is configured
    When AGM resolves permission policy parity
    Then every active harness should have a permission policy target
    And the resolved permission policy should include default permissions
    And the resolved permission policy should include profile permissions
