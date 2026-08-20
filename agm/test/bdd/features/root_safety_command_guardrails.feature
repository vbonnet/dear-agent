# SPEC: cmd/drift-check/SPEC.md
# RELATED-SPEC: cmd/configure-claude-settings/SPEC.md
# RELATED-SPEC: cmd/codex-hook-json/SPEC.md
# RELATED-SPEC: cmd/credential-monitor/SPEC.md
# RELATED-SPEC: cmd/gopls-watchdog/SPEC.md
# RELATED-SPEC: cmd/infra-plan-policy/SPEC.md
# RELATED-SPEC: cmd/log-rotate/SPEC.md
# RELATED-SPEC: cmd/pretool-bash-write-guard/SPEC.md
# RELATED-SPEC: cmd/pretool-fs-write-guard/SPEC.md
# RELATED-SPEC: cmd/token-refresher/SPEC.md
Feature: Root safety command guardrails
  Host safety commands and native harness adapters should keep executable SPEC
  traceability without promoting Claude-specific storage into shared policy.

  Scenario Outline: Safety command packages declare SPEC coverage
    Given safety command package "<package>" is configured
    When AGM validates safety command package coverage
    Then safety command package "<package>" should have a co-located SPEC

    Examples:
      | package                          |
      | cmd/configure-claude-settings    |
      | cmd/codex-hook-json              |
      | cmd/credential-monitor           |
      | cmd/drift-check                  |
      | cmd/gopls-watchdog               |
      | cmd/infra-plan-policy            |
      | cmd/log-rotate                   |
      | cmd/pretool-bash-write-guard     |
      | cmd/pretool-fs-write-guard       |
      | cmd/token-refresher              |
