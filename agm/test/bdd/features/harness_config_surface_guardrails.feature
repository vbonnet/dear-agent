# SPEC: agm/internal/agent/SPEC.md
# RELATED-SPEC: .agents/SPEC.md
# RELATED-SPEC: .claude/SPEC.md
# RELATED-SPEC: .codex/SPEC.md
# RELATED-SPEC: .gemini/SPEC.md
# RELATED-SPEC: .opencode/SPEC.md
# RELATED-SPEC: .dear-agent/SPEC.md
# RELATED-SPEC: .claude-plugin/SPEC.md
Feature: Harness configuration surface guardrails
  Repo-local harness configuration and marketplace surface directories should
  own co-located SPEC coverage so parity tests can enforce the supported
  surface contract directly.

  Scenario Outline: Harness configuration surfaces declare SPEC coverage
    Given harness configuration surface "<surface>" is configured
    When AGM validates harness configuration surface coverage
    Then harness configuration surface "<surface>" should have a co-located SPEC

    Examples:
      | surface        |
      | .agents        |
      | .claude        |
      | .codex         |
      | .gemini        |
      | .opencode      |
      | .dear-agent    |
      | .claude-plugin |
