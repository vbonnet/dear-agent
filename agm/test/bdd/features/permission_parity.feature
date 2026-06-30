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

  Scenario: Permission profiles resolve across the active harness set
    Given permission profile "worker" is configured
    When AGM resolves permission policy parity
    Then every active harness should have a permission policy target
    And the resolved permission policy should include default permissions
    And the resolved permission policy should include profile permissions
