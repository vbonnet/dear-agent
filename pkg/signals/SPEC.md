# Rigor Signal Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**SIGNALS-01** When task text contains configured rigor keywords, the system shall emit deterministic keyword signals with bounded confidence.

**SIGNALS-02** When estimated effort crosses configured thresholds, the system shall emit the corresponding rigor signal.

**SIGNALS-03** When changed files match configured risk patterns, the system shall emit deterministic file signals.

**SIGNALS-04** When Beads labels or prior phase outputs indicate risk, the system shall include those signals in analysis.

**SIGNALS-05** When signals are fused, the system shall keep confidence within zero and one and select a consistent rigor level.

**SIGNALS-06** When no signals are present, the system shall return the minimal rigor decision without inventing escalation evidence.

**SIGNALS-07** When escalation is logged, the system shall preserve the computed decision and any explicit user override.

**SIGNALS-08** While context comes from any supported harness and model family, the system shall apply the same evidence weights and thresholds.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/signals`
