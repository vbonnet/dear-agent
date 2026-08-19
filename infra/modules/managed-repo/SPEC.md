# Managed Repository Module Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**MANAGED-REPO-01** When the managed-repository module is instantiated, the system shall configure declared repository settings, rulesets, and automation resources.

**MANAGED-REPO-02** When merge policy is configured, the system shall preserve squash-only protected-branch behavior.

**MANAGED-REPO-03** When required status checks are configured, the system shall require a pull-request branch to be up to date with the base branch before merge unless the caller explicitly opts out.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
