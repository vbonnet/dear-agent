# SPEC: engram/internal/metacontext/SPEC.md
# RELATED-SPEC: engram/internal/agent/SPEC.md
# RELATED-SPEC: engram/internal/context/SPEC.md
# RELATED-SPEC: engram/internal/identity/SPEC.md
# RELATED-SPEC: engram/internal/memory/SPEC.md
# RELATED-SPEC: engram/internal/profile/SPEC.md
# RELATED-SPEC: engram/internal/prompt/SPEC.md
# RELATED-SPEC: engram/internal/scratchpad/SPEC.md
Feature: Engram core context guardrails
  Engram's agent, context, identity, memory, metacontext, profile, prompt, and
  scratchpad packages should carry executable SPEC traceability so durable
  context remains secure and harness-neutral as model-family parity expands.

  Scenario Outline: Engram core context packages declare SPEC coverage
    Given Engram core context package "<package>" is configured
    When AGM validates Engram core context package coverage
    Then Engram core context package "<package>" should have a co-located SPEC

    Examples:
      | package                     |
      | engram/internal/agent       |
      | engram/internal/context     |
      | engram/internal/identity    |
      | engram/internal/memory      |
      | engram/internal/metacontext |
      | engram/internal/profile     |
      | engram/internal/prompt      |
      | engram/internal/scratchpad  |
