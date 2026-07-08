# SPEC: pkg/workflow/codemod/SPEC.md
# RELATED-SPEC: pkg/workflow/dev/SPEC.md
# RELATED-SPEC: pkg/workflow/roles/SPEC.md
# RELATED-SPEC: agm/internal/workflow/SPEC.md
# RELATED-SPEC: agm/internal/workflow/deepresearch/SPEC.md
Feature: Workflow package guardrails
  Workflow implementation packages should carry executable SPEC traceability
  because command parity depends on stable workflow registries, model-role
  resolution, codemods, dev loops, and specialized workflows.

  Scenario Outline: Workflow packages declare SPEC coverage
    Given workflow package "<package>" is configured
    When AGM validates workflow package coverage
    Then workflow package "<package>" should have a co-located SPEC

    Examples:
      | package                            |
      | agm/internal/workflow              |
      | agm/internal/workflow/deepresearch |
      | pkg/workflow/codemod               |
      | pkg/workflow/dev                   |
      | pkg/workflow/roles                 |
