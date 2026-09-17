# SPEC: agm/internal/ops/SPEC.md
Feature: Sandbox launch authority
  AGM must bind managed sandbox launches to the runtime authority selected by
  the loaded configuration. The configured parent selects the storage volume,
  while only the stable AGM session's exact child workspace becomes writable.

  Scenario Outline: Create and cold resume select the stable AGM session workspace
    When AGM validates sandbox launch selection for "<lifecycle>" with "<harness>"
    Then the configured parent and stable AGM session child should reach the launch boundary
    And persisted or effective launch paths outside that child should be refused
    And APFS merged-directory symlinks within that child should remain valid for create and cold resume

    Examples:
      | lifecycle  | harness     |
      | create     | claude-code |
      | create     | codex-cli   |
      | cold-resume | claude-code |
      | cold-resume | codex-cli   |

  Scenario: Provider output is validated before onboarding or permission writes
    When AGM validates sandbox provider output containment
    Then malicious provider paths should be refused before outside onboarding or permission writes and cleaned by stable AGM session ID
    And unmaterialized provider paths should be refused before host directories are created
    And later create failures should clean through the original provider instance with an uncanceled context
    And contained provider symlinks should preserve the exact durable cleanup boundary while supplying a physical live and onboarding path

  Scenario: Fresh and cold-resume containment is checked before launch side effects
    When AGM validates fresh and cold-resume sandbox containment
    Then fresh creation should refuse an outside prepared working directory before remote or tmux mutation
    And cold resume should refuse persisted path drift or missing effective directories before tmux creation
