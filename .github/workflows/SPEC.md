# GitHub Workflow Configuration Specification

<!-- Last audited at: 2026-09-02 -->

## EARS Requirements

**DECL-WORKFLOW-01** When repository automation is triggered, the system shall execute the versioned CI, security, audit, release, and maintenance workflows for the declared events.

**DECL-WORKFLOW-02** If a required workflow job fails, the system shall preserve the failing conclusion and shall not report the workflow as successful.

**DECL-WORKFLOW-03** When CI runs on a pull request, push, schedule, or manual dispatch, the system shall execute the credential-free active-harness parity contracts and the isolated source-built Codex lifecycle without provider credentials.

**DECL-WORKFLOW-04** When CI runs on its schedule or by manual dispatch, the system shall execute the full credential-free AGM contract and integration test graphs while keeping provider-hosted scenarios explicit opt-in.

**DECL-WORKFLOW-05** When the AGM Codex contract job runs, the system shall enforce the versioned per-package coverage floors for critical lifecycle operations.

**DECL-WORKFLOW-06** When GitHub emits a `labeled` event that adds `full-ci` to a same-repository pull request targeting `main`, the system shall admit the Deepsec credential probe for the exact current pull-request head while rejecting fork pull requests and unrelated label events.

**DECL-WORKFLOW-07** When an eligible same-repository pull request targeting `main` and carrying `full-ci` is opened, synchronized, or reopened, the system shall admit the Deepsec credential probe for the exact current pull-request head.

**DECL-WORKFLOW-08** When the monthly cognitive-complexity audit evaluates the repository, the system shall report bounded findings from a complete trustworthy scan and shall fail without reporting clean whenever the scan is unavailable, invalid, incomplete, or violates its expected result protocol.

**DECL-WORKFLOW-09** When a pull request from this repository transitions from draft to ready for review, the system shall dispatch the agentic reviewers with the token scopes required to publish their findings, shall not require an opt-in label for that dispatch, and shall publish a review-status check run bound to the reviewed head commit.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package test: `agm/test/bdd/steps/deepsec_workflow_contract_test.go`

## Test Traceability

- Monthly cognitive-complexity scanner and workflow contract:
  `tests/bats/monthly-audit-complexity.bats`.
- Draft-to-ready agentic review dispatch, reviewer publish scopes, and the
  head-bound review-status check: `tests/bats/ready-review-dispatch.bats`.
  These are workflow-source assertions because the behaviour lives in GitHub's
  event dispatcher and in the Actions token scopes, neither of which the
  repository harness can execute.
- BDD consequence: No new BDD feature is required because the scheduled GitHub
  Actions runner and external scanner are not exposed by the repository BDD
  harness; deterministic Bats fixtures and workflow-source checks exercise the
  observable result protocol.
