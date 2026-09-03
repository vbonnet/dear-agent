# GitHub Workflow Configuration Specification

<!-- Last audited at: 2026-08-27 -->

## EARS Requirements

**DECL-WORKFLOW-01** When repository automation is triggered, the system shall execute the versioned CI, security, audit, release, and maintenance workflows for the declared events.

**DECL-WORKFLOW-02** If a required workflow job fails, the system shall preserve the failing conclusion and shall not report the workflow as successful.

**DECL-WORKFLOW-03** When CI runs on a pull request, push, schedule, or manual dispatch, the system shall execute the credential-free active-harness parity contracts and the isolated source-built Codex lifecycle without provider credentials.

**DECL-WORKFLOW-04** When CI runs on its schedule or by manual dispatch, the system shall execute the full credential-free AGM contract and integration test graphs while keeping provider-hosted scenarios explicit opt-in.

**DECL-WORKFLOW-05** When the AGM Codex contract job runs, the system shall enforce the versioned per-package coverage floors for critical lifecycle operations.

**DECL-WORKFLOW-06** When GitHub emits a `labeled` event that adds `full-ci` to a same-repository pull request targeting `main`, the system shall admit the Deepsec credential probe for the exact current pull-request head while rejecting fork pull requests and unrelated label events.

**DECL-WORKFLOW-07** When an eligible same-repository pull request targeting `main` and carrying `full-ci` is opened, synchronized, or reopened, the system shall admit the Deepsec credential probe for the exact current pull-request head.

**DECL-WORKFLOW-08** When the monthly cognitive-complexity audit evaluates the repository, the system shall report bounded findings from a complete trustworthy scan and shall fail without reporting clean whenever the scan is unavailable, invalid, incomplete, or violates its expected result protocol.

**DECL-WORKFLOW-09** When a reviewer family workflow runs, the system shall publish that family's started label before invoking its model, and shall publish a posted label plus an approved, changes-requested, or error label once the reviewer concludes.

**DECL-WORKFLOW-10** When a pull request head is pushed, the system shall remove every agentic review label before the gate evaluates, and shall fail the gate rather than evaluate if that removal did not succeed.

**DECL-WORKFLOW-11** When the agentic review gate evaluates, the system shall invoke no model and shall publish the `agentic-review/gate` commit status against the evaluated head.

**DECL-WORKFLOW-12** When the scheduled sweep runs, the system shall refresh the gate status for every open non-draft pull request so a reviewer family that fell silent past its deadline is resolved without a further GitHub event.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package test: `agm/test/bdd/steps/deepsec_workflow_contract_test.go`

## Test Traceability

- Monthly cognitive-complexity scanner and workflow contract:
  `tests/bats/monthly-audit-complexity.bats`.
- BDD consequence: No new BDD feature is required because the scheduled GitHub
  Actions runner and external scanner are not exposed by the repository BDD
  harness; deterministic Bats fixtures and workflow-source checks exercise the
  observable result protocol.
