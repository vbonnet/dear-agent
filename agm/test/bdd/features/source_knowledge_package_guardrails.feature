# SPEC: pkg/source/SPEC.md
# RELATED-SPEC: pkg/source/contract/SPEC.md
# RELATED-SPEC: pkg/source/registry/SPEC.md
# RELATED-SPEC: pkg/source/sqlite/SPEC.md
# RELATED-SPEC: pkg/source/obsidian/SPEC.md
# RELATED-SPEC: pkg/source/llmwiki/SPEC.md
# RELATED-SPEC: pkg/source/openviking/SPEC.md
# RELATED-SPEC: pkg/source/workflowbridge/SPEC.md
# RELATED-SPEC: pkg/papersearch/SPEC.md
# RELATED-SPEC: pkg/wikibrain/SPEC.md
Feature: Source and knowledge package guardrails
  Source adapters, paper search, and wikibrain knowledge tooling must keep
  executable SPEC traceability because they form the shared knowledge substrate
  for workflow durability, search, and engram support.

  Scenario Outline: Source and knowledge packages declare SPEC coverage
    Given source and knowledge package "<package>" is configured
    When AGM validates source and knowledge package coverage
    Then source and knowledge package "<package>" should have a co-located SPEC

    Examples:
      | package                   |
      | pkg/papersearch           |
      | pkg/source                |
      | pkg/source/contract       |
      | pkg/source/llmwiki        |
      | pkg/source/obsidian       |
      | pkg/source/openviking     |
      | pkg/source/registry       |
      | pkg/source/sqlite         |
      | pkg/source/workflowbridge |
      | pkg/wikibrain             |
