# SPEC: agm/internal/contracts/SPEC.md
# RELATED-SPEC: agm/internal/benchmark/SPEC.md
# RELATED-SPEC: agm/internal/debug/SPEC.md
# RELATED-SPEC: agm/internal/fileutil/SPEC.md
# RELATED-SPEC: agm/internal/fix/SPEC.md
# RELATED-SPEC: agm/internal/gclog/SPEC.md
# RELATED-SPEC: agm/internal/git/SPEC.md
# RELATED-SPEC: agm/internal/logging/SPEC.md
# RELATED-SPEC: agm/internal/logs/SPEC.md
# RELATED-SPEC: agm/internal/quality/SPEC.md
# RELATED-SPEC: agm/internal/trace/SPEC.md
# RELATED-SPEC: agm/internal/verify/SPEC.md
Feature: AGM diagnostics package guardrails
  Operational support packages must keep executable SPEC traceability so SLOs,
  evidence, repository safety, logging, quality gates, and completion checks do
  not drift independently of the behavior they govern.

  Scenario Outline: AGM diagnostics packages declare SPEC coverage
    Given AGM diagnostics package "<package>" is configured
    When AGM validates diagnostics package coverage
    Then AGM diagnostics package "<package>" should have a co-located SPEC

    Examples:
      | package                |
      | agm/internal/benchmark |
      | agm/internal/contracts |
      | agm/internal/debug     |
      | agm/internal/fileutil  |
      | agm/internal/fix       |
      | agm/internal/gclog     |
      | agm/internal/git       |
      | agm/internal/logging   |
      | agm/internal/logs      |
      | agm/internal/quality   |
      | agm/internal/trace     |
      | agm/internal/verify    |
