# GitHub Ruleset Configuration Specification

<!-- Last audited at: 2026-08-19 -->

## EARS Requirements

**DECL-RULESET-01** When dear-agent branch protection is provisioned, the system shall derive the provider ruleset from the versioned main-branch ruleset JSON.

**DECL-RULESET-02** When merge policy is evaluated, the system shall preserve required checks and squash-only protected-branch behavior.

**DECL-RULESET-03** When Markdown documentation changes, the main-branch ruleset shall require the `Header block format` check before merge.

**DECL-RULESET-04** When the canonical main ruleset is reconciled, the system shall preserve the explicitly supported zero-bypass branch-protection subset: branch target and ref conditions, name, active enforcement, no bypass actors, deletion and force-push prevention, linear history, squash-only pull requests, the declared supported pull-request fields, strict status checks with the declared create-enforcement field, and each dear-agent required check bound to GitHub Actions integration ID 15368; an omitted generic-repository check integration ID shall normalize as an explicit context-only identity, while any additional, omitted-required, null-required, or incorrectly typed owned policy field shall fail closed until the subset is deliberately extended.

**DECL-RULESET-05** When canonical ruleset policy changes, an independent verifier shall attest the saved OpenTofu plan's exact digest and declared scope before the system applies that same artifact without routine human approval; if the plan contains a destroy, replacement, state migration, irreversible change, or ambiguous effect, the system shall stop for human authorization before provider-visible mutation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
