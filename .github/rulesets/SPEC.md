# GitHub Ruleset Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-RULESET-01** When branch protection is provisioned, the system shall apply the versioned main-branch ruleset.

**DECL-RULESET-02** When merge policy is evaluated, the system shall preserve required checks and squash-only protected-branch behavior.

**DECL-RULESET-03** When Markdown documentation changes, the main-branch ruleset shall require the `Header block format` check before merge.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
