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

**DECL-WORKFLOW-09** When the daily schedule, a dependency-path push to the default branch, or a default-branch manual dispatch triggers the vulnerability workflow, the system shall use a dedicated non-pull-request job with only the required read permissions to retrieve every page of open dependency and Trivy code-scanning alerts before evaluating the repository vulnerability policy.

**DECL-WORKFLOW-10** If either provider vulnerability-alert source is unavailable, malformed, or incomplete, then the system shall fail the provider audit without reporting policy compliance.

**DECL-WORKFLOW-11** When repository vulnerability scanning runs, the system shall apply the canonical reporting-severity projection to staged SARIF and the canonical blocking-severity projection to the admission scan, while only a trusted non-pull-request publisher receives `security-events: write` to upload the staged report.

**DECL-WORKFLOW-12** When a pull request changes no dependency input and change detection succeeds, the system shall publish the required vulnerability context without executing the blocking admission scan.

**DECL-WORKFLOW-13** When a scheduled, dependency-path push, or manually dispatched provider audit evaluates vulnerability alerts, the system shall verify the deterministic policy failure and liveness canaries before evaluating live alerts.

**DECL-WORKFLOW-14** The provider vulnerability audit shall not mutate provider alerts or repository issues.

**DECL-WORKFLOW-15** While a scheduled or manually dispatched vulnerability sweep is running, the system shall use a distinct non-cancellable concurrency group that push and pull-request activity cannot cancel.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package test: `agm/test/bdd/steps/deepsec_workflow_contract_test.go`

## Test Traceability

- Monthly cognitive-complexity scanner and workflow contract:
  `tests/bats/monthly-audit-complexity.bats`.
- Vulnerability policy projection, provider-evidence, failure, and pull-request
  liveness contract:
  `cmd/vulnerability-policy/workflow_contract_test.go`.
- BDD consequence: No new BDD feature is required because the scheduled GitHub
  Actions runner, external scanner, and provider alert APIs are not exposed by
  the repository BDD harness; deterministic command fixtures, Bats fixtures,
  and workflow-source checks exercise the observable result protocols.
