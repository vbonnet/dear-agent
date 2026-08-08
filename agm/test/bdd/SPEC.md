# AGM Executable BDD Runner Specification

<!-- Last audited at: 2026-08-08 -->

## Requirements

**BDDR-01** When the BDD suite runs, the runner shall execute every direct-child feature file in `agm/test/bdd/features` without a tag filter.

**BDDR-02** If a feature step is undefined, then the runner shall fail the suite instead of silently skipping the scenario.

**BDDR-03** When a feature file is added, the BDD invariants shall require it to be represented in the maintained catalog.

**BDDR-04** When SPEC and feature traceability drift, the BDD invariants shall fail with the missing contract relationship.

**BDDR-05** When a lifecycle scenario claims an operation succeeded, the system shall invoke the production operation boundary and assert observable storage or runtime effects instead of setting simulated success flags.

**BDDR-06** When a BDD scenario hook overrides process-wide environment variables or creates an owned temporary directory, the system shall limit setup to applicable scenarios, restore the exact prior environment, reuse explicit shared Go caches, remove only the owned directory, and report restoration or cleanup failures.

**BDDR-07** If a `.feature` file is nested below `agm/test/bdd/features` or its direct-child basename contains characters outside the shared ASCII letter, digit, dot, underscore, and hyphen grammar, then the BDD invariants shall fail because Godog would execute input omitted by the flat parseable catalog and governance registries.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/bdd/*_test.go`
