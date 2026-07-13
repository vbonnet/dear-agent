# Benchmark Baseline Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-BASELINE-01** When performance regression checks run, the system shall compare measurements with the versioned baseline data.

**FIX-BASELINE-02** If a measurement exceeds an enforced regression threshold, the system shall report the regression rather than rewrite the baseline implicitly.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
