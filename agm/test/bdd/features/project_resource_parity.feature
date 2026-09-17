# SPEC: agm/internal/projectresourceparity/SPEC.md
Feature: Project resource surface governance
  Active harnesses should declare how they consume repository instructions,
  skills, configuration, and hooks without asserting false cross-harness
  equivalence or taking ownership from the behavioral modules.

  Scenario Outline: Active harnesses declare final project resource dispositions
    Given project resource harness "<harness>" is active
    When AGM validates project resource surface coverage
    Then harness "<harness>" should declare instruction "<instructions>", skill "<skills>", configuration "<configuration>", and hook "<hooks>" dispositions

    Examples:
      | harness      | instructions | skills    | configuration | hooks     |
      | claude-code  | adapted      | supported | supported     | supported |
      | codex-cli    | supported    | adapted   | supported     | adapted   |
      | agy          | adapted      | adapted   | supported     | supported |
      | opencode-cli | adapted      | adapted   | supported     | adapted   |
      | pi-cli       | supported    | adapted   | supported     | adapted   |

  Scenario: The Pi repository projection declares the shared instruction entrypoint
    Given the repository Pi instruction surface
    When AGM validates the Pi instruction projection
    Then the repository should expose root AGENTS.md without a divergent Pi-only copy
