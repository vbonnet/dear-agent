# Built-in DEAR Audit Checks Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`pkg/audit/checks` ships the built-in audit checks (build, test, lint,
govulncheck, complexity, OTel spans, spec coverage/conformance/staleness) and
the v1 lint-gap refiner. Each check implements `audit.Check`, registers itself
with `audit.Default` at package init, and ships offline-replayable tests.

## EARS Requirements

**CHECKS-01** When the package is initialized, the system shall register every built-in check and the lint-gap refiner with the default audit registry.

**CHECKS-02** When a check wraps an external CLI tool, the system shall capture stdout and stderr separately, cap each stream at 1 MiB, and treat a non-zero exit code as findings rather than a tool failure.

**CHECKS-03** When a wrapped tool binary cannot start, the system shall report the invocation error instead of fabricating findings.

**CHECKS-04** When the lint-gap refiner observes a single linter rule with at least the configured recurrence threshold of findings in one run, the system shall emit one informational proposal for that linter with a suggested configuration diff.

**CHECKS-05** When the lint-gap refiner threshold is unset or non-positive, the system shall apply the default recurrence threshold of five findings.
