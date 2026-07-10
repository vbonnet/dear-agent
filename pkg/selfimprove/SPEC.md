# Self-Improvement Loop Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**SELFIMPROVE-01** When a loop starts, the system shall require benchmark, proposer, applier, suite, and mode dependencies with a matching benchmark suite.

**SELFIMPROVE-02** When no notifier is configured, the system shall use a no-op notifier without changing loop behavior.

**SELFIMPROVE-03** When proposer models are configured, the system shall request hypotheses from each model identifier without restricting the model family.

**SELFIMPROVE-04** When no proposer model list is configured, the system shall use the benchmark model as the proposal route.

**SELFIMPROVE-05** When the budget or cycle cap is reached, the system shall stop before starting additional improvement work.

**SELFIMPROVE-06** When a post-change benchmark or comparison fails, the system shall attempt to revert every applied patch.

**SELFIMPROVE-07** When an enabled regression gate detects lower solve rate, the system shall revert patches and stop further cycles.

**SELFIMPROVE-08** When a cycle improves or preserves the gated result, the system shall accept it and use its post-results as the next baseline.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/selfimprove`
