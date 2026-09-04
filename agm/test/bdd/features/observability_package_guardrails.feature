# SPEC: internal/telemetry/agent/SPEC.md
# RELATED-SPEC: internal/telemetry/analysis/SPEC.md
# RELATED-SPEC: internal/telemetry/enrichment/SPEC.md
# RELATED-SPEC: internal/telemetry/errors/SPEC.md
# RELATED-SPEC: internal/metrics/SPEC.md
# RELATED-SPEC: pkg/otelsetup/SPEC.md
# RELATED-SPEC: cmd/otel-local/SPEC.md
# RELATED-SPEC: cmd/jaeger-health/SPEC.md
# RELATED-SPEC: pkg/absencealarm/SPEC.md
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
      | cmd/jaeger-health             |
      | pkg/absencealarm              |
      | cmd/otel-local                |
      | internal/metrics              |
      | internal/telemetry/agent      |
      | internal/telemetry/analysis   |
      | internal/telemetry/enrichment |
      | internal/telemetry/errors     |
      | pkg/otelsetup                 |

  Scenario Outline: Observability specifications define cancellation and timeout edges
    Given observability package "<package>" is configured
    When AGM validates observability package coverage
    Then observability package SPEC should declare requirement "<requirement>" containing "<contract>"

    Examples:
      | package                     | requirement    | contract                |
      | pkg/absencealarm            | AA-05          | cannot be evaluated     |
      | internal/telemetry/analysis | TEL-ANALYSIS-01 | context is canceled     |
      | pkg/otelsetup               | OTELSETUP-07   | revision is shorter     |
      | cmd/jaeger-health           | JAEGER-HEALTH-02 | times out              |
