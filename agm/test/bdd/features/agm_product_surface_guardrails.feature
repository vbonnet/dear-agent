# SPEC: agm/internal/gateway/SPEC.md
# RELATED-SPEC: agm/agm-plugin/commands/SPEC.md
# RELATED-SPEC: agm/cmd/plugin-hash/SPEC.md
# RELATED-SPEC: agm/cmd/fix-uuid-manual/SPEC.md
# RELATED-SPEC: agm/internal/pluginhash/SPEC.md
# RELATED-SPEC: agm/internal/surface/cmd/generate/SPEC.md
# RELATED-SPEC: agm/internal/tui/SPEC.md
# RELATED-SPEC: agm/internal/ui/SPEC.md
# RELATED-SPEC: agm/workflowbus/SPEC.md
Feature: AGM product surface guardrails
  AGM's remaining gateway, plugin, generated, workflow-bus, and operator UI
  surfaces must keep executable SPEC traceability and explicit accessibility,
  safety, and shared-registry contracts.

  Scenario Outline: AGM product surfaces declare SPEC coverage
    Given AGM product surface "<package>" is configured
    When AGM validates product surface coverage
    Then AGM product surface "<package>" should have a co-located SPEC

    Examples:
      | package                           |
      | agm/agm-plugin/commands           |
      | agm/cmd/plugin-hash               |
      | agm/cmd/fix-uuid-manual           |
      | agm/internal/gateway              |
      | agm/internal/pluginhash           |
      | agm/internal/surface/cmd/generate |
      | agm/internal/tui                  |
      | agm/internal/ui                   |
      | agm/workflowbus                   |
