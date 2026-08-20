# GitHub Ruleset Configuration Specification

<!-- Last audited at: 2026-08-19 -->

## EARS Requirements

**DECL-RULESET-01** When dear-agent branch protection is provisioned, the system shall derive the provider ruleset from the versioned main-branch ruleset JSON.

**DECL-RULESET-02** When merge policy is evaluated, the system shall preserve required checks and squash-only protected-branch behavior.

**DECL-RULESET-03** When Markdown documentation changes, the main-branch ruleset shall require the `Header block format` check before merge.

**DECL-RULESET-04** When the canonical main ruleset is reconciled, the system shall preserve the explicitly supported zero-bypass branch-protection subset: branch target and ref conditions, name, active enforcement, no bypass actors, deletion and force-push prevention, linear history, squash-only pull requests, the declared supported pull-request fields, strict status checks with the declared create-enforcement field, and each dear-agent required check bound to GitHub Actions integration ID 15368; an omitted generic-repository check integration ID shall normalize as an explicit context-only identity, while any additional, omitted-required, null-required, or incorrectly typed owned policy field shall fail closed until the subset is deliberately extended.

**DECL-RULESET-06** When managed repository branch protection is audited, the system shall require exactly one active default-branch ruleset whose complete supported policy matches the canonical declaration and shall report any legacy classic protection, competing authority, weakened parameter, or mismatched required-check identity.

**DECL-RULESET-07** When branch-protection auditing uses the private managed inventory, the system shall require complete cross-repository evidence, evaluate every active ruleset applying to the concrete default branch including explicit ref patterns, verify each required check's context and declared GitHub App identity from check runs no older than thirty days across the default branch and at most the twenty most recently updated pull-request heads targeting that branch, and expose only aggregate findings on public workflow surfaces.

**DECL-RULESET-05** When canonical ruleset policy changes, an independent verifier shall attest the saved OpenTofu plan's exact digest and declared scope before the system applies that same artifact without routine human approval; if the plan contains a destroy, replacement, state migration, irreversible change, or ambiguous effect, the system shall stop for human authorization before provider-visible mutation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
