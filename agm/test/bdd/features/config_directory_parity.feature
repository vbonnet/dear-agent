# SPEC: agm/internal/configdirparity/SPEC.md
# RELATED-SPEC: agm/internal/config/SPEC.md
# RELATED-SPEC: agm/internal/a2a/config/SPEC.md
# RELATED-SPEC: .agents/SPEC.md
# RELATED-SPEC: .claude/SPEC.md
# RELATED-SPEC: .codex/SPEC.md
# RELATED-SPEC: .gemini/SPEC.md
# RELATED-SPEC: .opencode/SPEC.md
# RELATED-SPEC: .pi/SPEC.md
Feature: Harness configuration directory parity
  AGM should keep repo-local dot-directory configuration surfaces for every
  active harness, with Gemini retained as deprecated compatibility.

  Scenario Outline: Active harnesses have configuration directories
    Given harness "<harness>" is configured
    When AGM validates configuration directory parity
    Then harness "<harness>" should have configuration directory "<directory>"

    Examples:
      | harness      | directory |
      | claude-code  | .claude   |
      | codex-cli    | .codex    |
      | agy          | .agents   |
      | opencode-cli | .opencode |
      | pi-cli       | .pi       |

  Scenario: Gemini configuration directory is deprecated compatibility
    Given harness "gemini-cli" is configured
    When AGM validates deprecated configuration directory parity
    Then harness "gemini-cli" should have configuration directory ".gemini"
    And harness "gemini-cli" should be deprecated
