# SPEC: pkg/workflow/codemod/SPEC.md
# RELATED-SPEC: pkg/workflow/dev/SPEC.md
# RELATED-SPEC: pkg/workflow/roles/SPEC.md
# RELATED-SPEC: pkg/workflow/SPEC.md
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

  Scenario: Enforced workflows must declare their constitutional invariants
    Given a workflow enables constitutional enforcement without invariants
    When AGM validates and attempts to run the workflow
    Then workflow validation should fail before run recording, lifecycle hooks, or node execution

  Scenario: Definition policy failures stop workflow execution
    Given a valid workflow whose definition policy rejects it
    When AGM attempts to run the definition-rejected workflow
    Then the run should fail before node execution with a terminal definition error
