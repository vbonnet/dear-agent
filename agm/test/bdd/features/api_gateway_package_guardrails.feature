# SPEC: pkg/api/SPEC.md
# RELATED-SPEC: pkg/gateway/SPEC.md
# RELATED-SPEC: pkg/gateway/adapters/cli/SPEC.md
# RELATED-SPEC: pkg/gateway/adapters/http/SPEC.md
# RELATED-SPEC: cmd/dear-agent-api/SPEC.md
Feature: API and gateway package guardrails
  API and gateway packages must keep executable SPEC traceability because they
  expose workflow, HITL, audit, and run-triggering control surfaces across
  loopback, Tailscale, CLI, and future adapters.

  Scenario Outline: API and gateway packages declare SPEC coverage
    Given API and gateway package "<package>" is configured
    When AGM validates API and gateway package coverage
    Then API and gateway package "<package>" should have a co-located SPEC

    Examples:
      | package                   |
      | cmd/dear-agent-api        |
      | pkg/api                   |
      | pkg/gateway               |
      | pkg/gateway/adapters/cli  |
      | pkg/gateway/adapters/http |
