# Repository Script Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**REPO-SCRIPT-01** When a repository maintenance script runs, the system shall validate its prerequisites and operate only on the declared repository scope.

**REPO-SCRIPT-02** If a build, synchronization, installation, or verification step fails, the system shall preserve the failure and avoid a false success message.

**REPO-SCRIPT-03** When local preflight runs repository tests, the system shall use the same explicit test timeout as required CI.

**REPO-SCRIPT-04** When document freshness runs locally or in GitHub Actions, the system shall resolve the comparison commit from the explicit target, actual pull-request base, or mutation event rather than assuming the main branch.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`
