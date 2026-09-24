# SPEC: engram/retrieval/SPEC.md
# RELATED-SPEC: engram/internal/document/SPEC.md
# RELATED-SPEC: engram/internal/corpus/SPEC.md
Feature: Engram knowledge guardrails
  Engram's retrieval, document, and corpus-callosum knowledge packages should
  carry executable SPEC traceability so knowledge recall and sharing do not
  regress as harness and model-family parity expands.

  Scenario Outline: Engram knowledge packages declare SPEC coverage
    Given Engram knowledge package "<package>" is configured
    When AGM validates Engram knowledge package coverage
    Then Engram knowledge package "<package>" should have a co-located SPEC

    Examples:
      | package                   |
      | engram/retrieval          |
      | engram/internal/document  |
      | engram/internal/corpus    |
