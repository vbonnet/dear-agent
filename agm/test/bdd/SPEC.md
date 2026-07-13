# AGM Executable BDD Runner Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**BDDR-01** When the BDD suite runs, the runner shall execute every feature file under `agm/test/bdd/features` without a tag filter.

**BDDR-02** If a feature step is undefined, then the runner shall fail the suite instead of silently skipping the scenario.

**BDDR-03** When a feature file is added, the BDD invariants shall require it to be represented in the maintained catalog.

**BDDR-04** When SPEC and feature traceability drift, the BDD invariants shall fail with the missing contract relationship.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/bdd/*_test.go`
