# Critical Lifecycle Coverage Policy Specification

<!-- Last audited at: 2026-07-19 -->

## Requirements

**COVPOL-01** When CI evaluates critical AGM lifecycle coverage, the system shall enforce a versioned minimum statement-coverage floor for backend capability forwarding, shared operations, state detection, and delivery safety.

**COVPOL-02** When a coverage floor changes, the system shall preserve the package identity and explicit percentage in the reviewed policy rather than infer a repository-wide completeness claim.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Policy: `agm/test/coverage/critical-lifecycle.json`
- Enforcer: `cmd/coverage-ratchet`
