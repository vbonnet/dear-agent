# Variant Benchmark Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**BENCHMARK-01** When a benchmark task is registered, the system shall make it discoverable by stable name.

**BENCHMARK-02** When a variant is run repeatedly, the system shall preserve each run result and propagate context or execution errors.

**BENCHMARK-03** When metrics are extracted, the system shall return the selected duration, token, cost, or quality observations.

**BENCHMARK-04** When variants are compared, the system shall compute statistical significance and effect size using the shared statistics package.

**BENCHMARK-05** When comparison evidence is inconclusive, the system shall decline to select a winner.

**BENCHMARK-06** When one variant has clear supported evidence, the system shall select it and preserve the comparison rationale.

**BENCHMARK-07** When orchestration receives an unknown task or fewer than two variants, the system shall return an explicit error.

**BENCHMARK-08** While variants originate from any supported harness and model family, the system shall apply identical metrics and decision thresholds.

## BDD Traceability

- Feature: `agm/test/bdd/features/evaluation_control_parity.feature`

## Test Traceability

- Unit package: `pkg/benchmark`
