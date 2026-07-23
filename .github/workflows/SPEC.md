# GitHub Workflow Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-WORKFLOW-01** When repository automation is triggered, the system shall execute the versioned CI, security, audit, release, and maintenance workflows for the declared events.

**DECL-WORKFLOW-02** If a required workflow job fails, the system shall preserve the failing conclusion and shall not report the workflow as successful.

**DECL-WORKFLOW-03** When CI runs on a pull request, push, schedule, or manual dispatch, the system shall execute the credential-free active-harness parity contracts and the isolated source-built Codex lifecycle without provider credentials.

**DECL-WORKFLOW-04** When CI runs on its schedule or by manual dispatch, the system shall execute the full credential-free AGM contract and integration test graphs while keeping provider-hosted scenarios explicit opt-in.

**DECL-WORKFLOW-05** When the AGM Codex contract job runs, the system shall enforce the versioned per-package coverage floors for critical lifecycle operations.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
