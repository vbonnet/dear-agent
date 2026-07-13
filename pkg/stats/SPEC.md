# Statistical Analysis Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**STATS-01** When a non-empty sample is created, the system shall calculate count, mean, and sample standard deviation.

**STATS-02** When an empty sample is created, the system shall return zero-valued descriptive statistics without failure.

**STATS-03** When Welch's t-test has sufficient samples, the system shall calculate a two-sided significance result without assuming equal variance.

**STATS-04** When Welch's t-test has insufficient samples or zero standard error, the system shall return a non-significant result.

**STATS-05** When effect size is requested, the system shall calculate the standardized mean difference from pooled variation.

**STATS-06** When bootstrap confidence intervals are requested, the system shall return ordered bounds and handle empty samples explicitly.

**STATS-07** When minimum sample size inputs are invalid, the system shall return zero instead of a misleading estimate.

**STATS-08** While samples originate from any supported harness and model family, the system shall apply identical statistical methods.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/stats`
