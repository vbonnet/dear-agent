# SPEC: agm/internal/conversation/SPEC.md
# RELATED-SPEC: agm/internal/claude/SPEC.md
# RELATED-SPEC: agm/internal/claudeui/SPEC.md
# RELATED-SPEC: agm/internal/detection/SPEC.md
# RELATED-SPEC: agm/internal/fuzzy/SPEC.md
# RELATED-SPEC: agm/internal/history/SPEC.md
# RELATED-SPEC: agm/internal/importer/SPEC.md
# RELATED-SPEC: agm/internal/search/SPEC.md
# RELATED-SPEC: agm/internal/transcript/SPEC.md
# RELATED-SPEC: agm/internal/uuid/SPEC.md
Feature: AGM conversation and discovery package guardrails
  Conversation persistence and discovery packages must keep executable SPEC
  traceability so harness-neutral formats stay separate from harness-specific
  filesystem adapters and imported sessions preserve canonical metadata.

  Scenario Outline: AGM conversation and discovery packages declare SPEC coverage
    Given AGM conversation package "<package>" is configured
    When AGM validates conversation package coverage
    Then AGM conversation package "<package>" should have a co-located SPEC

    Examples:
      | package                   |
      | agm/internal/claude       |
      | agm/internal/claudeui     |
      | agm/internal/conversation |
      | agm/internal/detection    |
      | agm/internal/fuzzy        |
      | agm/internal/history      |
      | agm/internal/importer     |
      | agm/internal/search       |
      | agm/internal/transcript   |
      | agm/internal/uuid         |
