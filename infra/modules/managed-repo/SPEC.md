# Managed Repository Module Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**MANAGED-REPO-01** When the managed-repository module is instantiated, the system shall configure declared repository settings, rulesets, and automation resources.

**MANAGED-REPO-02** When merge policy is configured, the system shall preserve squash-only protected-branch behavior.

**MANAGED-REPO-03** When required status checks are configured, the system shall require a pull-request branch to be up to date with the base branch before merge unless the caller's declared ruleset explicitly opts out.

**MANAGED-REPO-04** When a managed ruleset is configured, the system shall render the explicitly supported zero-bypass branch-protection subset, preserve its declared reviewer and required-check identities, reject fields outside the subset as well as non-active or non-zero-bypass policy, and require the ruleset to target the default branch without excluding it.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
