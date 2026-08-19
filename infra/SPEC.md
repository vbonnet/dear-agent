# Repository Infrastructure Specification

<!-- Last audited at: 2026-08-10 -->

## EARS Requirements

**INFRA-01** When dear-agent repository infrastructure is planned or applied, the system shall derive the explicitly supported zero-bypass branch-protection subset from the versioned canonical ruleset JSON through versioned Terraform inputs and shall fail closed on fields outside that subset.

**INFRA-02** If an import or apply operation cannot identify the intended resource, the system shall fail before mutating unrelated infrastructure state.

**INFRA-03** When an OpenTofu plan changes canonical ruleset policy, an independent verifier shall attest the saved plan's exact digest and declared scope before the system applies that same artifact without routine human approval; if the plan contains a destroy, replacement, state migration, irreversible change, or ambiguous effect, the system shall stop for human authorization before provider-visible mutation.

**INFRA-04** When a canonical ruleset apply completes, the system shall verify the immutable state binding, a no-drift provider refresh, canonical source parity, and effective branch enforcement before reporting reconciliation complete.

**INFRA-05** When production reconciliation uses private inventory, the system shall run only from trusted default-branch code, fail on incomplete provider evidence or duplicate and overlapping case-insensitive active and archived identities, and withhold repository identities and detailed plans from public logs, issues, comments, and artifacts.

**INFRA-06** When state import enumerates managed repositories, the system shall derive active and archived names from the same evaluated OpenTofu inventory used by the module graph and shall fail before any state or provider operation if that inventory is malformed, omits dear-agent, duplicates a case-insensitive GitHub repository identity within a set, or contains the same case-insensitive identity across both sets.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
