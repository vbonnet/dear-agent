# AGM Performance Tests Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**PERF-01** When event-bus performance is measured, the suite shall report throughput, latency distribution, errors, and connection behavior for the named workload.

**PERF-02** When burst, sustained, filtered, or churn workloads execute, the suite shall use bounded durations and deterministic completion criteria.

**PERF-03** If the host cannot provide a dedicated performance environment, then the suite shall skip timing-sensitive tests instead of asserting unstable thresholds.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Performance tests: `agm/test/performance/*_test.go`
