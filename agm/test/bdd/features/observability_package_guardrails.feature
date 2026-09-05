# SPEC: internal/telemetry/agent/SPEC.md
# RELATED-SPEC: internal/telemetry/analysis/SPEC.md
# RELATED-SPEC: internal/telemetry/enrichment/SPEC.md
# RELATED-SPEC: internal/telemetry/errors/SPEC.md
# RELATED-SPEC: internal/metrics/SPEC.md
# RELATED-SPEC: pkg/otelsetup/SPEC.md
# RELATED-SPEC: cmd/otel-local/SPEC.md
# RELATED-SPEC: cmd/jaeger-health/SPEC.md
# RELATED-SPEC: pkg/absencealarm/SPEC.md
# RELATED-SPEC: cmd/merge-health/SPEC.md
# RELATED-SPEC: pkg/gatehealth/SPEC.md
# RELATED-SPEC: cmd/gate-health/SPEC.md
# RELATED-SPEC: cmd/bead-health/SPEC.md
Feature: Observability package guardrails
  Observability packages must keep executable SPEC traceability because quota,
  drift, trace, and agent-runtime monitoring need the same contracts across
  harnesses and model families.

  Scenario Outline: Observability packages declare SPEC coverage
    Given observability package "<package>" is configured
    When AGM validates observability package coverage
    Then observability package "<package>" should have a co-located SPEC

    Examples:
      | package                       |
      | cmd/gate-health               |
      | cmd/bead-health               |
      | cmd/jaeger-health             |
      | pkg/absencealarm              |
      | cmd/merge-health              |
      | cmd/otel-local                |
      | internal/metrics              |
      | internal/telemetry/agent      |
      | internal/telemetry/analysis   |
      | internal/telemetry/enrichment |
      | internal/telemetry/errors     |
      | pkg/gatehealth                |
      | pkg/otelsetup                 |

  Scenario Outline: Observability specifications define cancellation and timeout edges
    Given observability package "<package>" is configured
    When AGM validates observability package coverage
    Then observability package SPEC should declare requirement "<requirement>" containing "<contract>"

    Examples:
      | package                     | requirement    | contract                |
      | pkg/absencealarm            | AA-05          | cannot be evaluated     |
      | cmd/merge-health            | MH-05          | in the future           |
      | internal/telemetry/analysis | TEL-ANALYSIS-01 | context is canceled     |
      | pkg/otelsetup               | OTELSETUP-07   | revision is shorter     |
      | cmd/jaeger-health           | JAEGER-HEALTH-02 | times out              |
      | pkg/gatehealth              | GH-05          | exclude that pull request |
      | cmd/gate-health             | GHC-03         | exit 2                  |
      | cmd/bead-health             | BH-05          | in the future           |
