# SPEC: pkg/llm/auth/SPEC.md
# RELATED-SPEC: pkg/llm/config/SPEC.md
# RELATED-SPEC: pkg/llm/delegation/SPEC.md
# RELATED-SPEC: pkg/llm/quota/SPEC.md
# RELATED-SPEC: pkg/llm/router/SPEC.md
Feature: LLM runtime guardrails
  LLM auth, config, delegation, quota, and routing packages should carry
  executable SPEC traceability so model-family support has stable
  lower-level runtime contracts.

  Scenario Outline: LLM runtime packages declare SPEC coverage
    Given LLM runtime package "<package>" is configured
    When AGM validates LLM runtime package coverage
    Then LLM runtime package "<package>" should have a co-located SPEC

    Examples:
      | package            |
      | pkg/llm/auth       |
      | pkg/llm/config     |
      | pkg/llm/delegation |
      | pkg/llm/quota      |
      | pkg/llm/router     |
