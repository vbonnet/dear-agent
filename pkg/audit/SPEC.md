# DEAR Audit Engine Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
- Test consequence: Deterministic unit and integration tests in
  `remediation_test.go`, `remediation_suggestion_test.go`, `store_test.go`,
  `runner_test.go`, and `verifier_test.go` cover remediation validation,
  policy defaulting, persistence round trips, and partial or failed run
  outcomes.

<!-- Last audited at: 2026-08-28 -->

## Purpose

`pkg/audit` runs configured DEAR audit checks, stores audit runs and findings,
applies severity policy, invokes verifiers and refiners, and records proposals
for remediation. The package is intentionally repository-neutral so scheduled,
CLI, and workflow-triggered audits share the same lifecycle semantics.

Remediation fields are inert suggestions. A strategy is a closed handling hint,
not proof that a command, patch, PR, or issue is applicable or authorized. The
command, patch, title, and body fields are optional operator context, so a
patchless PR suggestion can recommend investigation or PR-producing work. The
package records suggestions without dispatching them. Any side-effecting
remediation requires a separately chartered durable module with a proven live
producer and consumer, applicability, revision binding, authority, idempotency,
persistence, and reconciliation semantics.

## EARS Requirements

**AUDIT-01** When an audit runner is created, the system shall install the default check registry, logger, clock, and identifier generator.

**AUDIT-02** When an audit run starts, the system shall reject invalid plans before creating an audit run record in the configured store.

**AUDIT-03** When no severity policy is supplied, the system shall apply the package default severity policy for failing, notifying, and remediation strategy selection.

**AUDIT-04** When a check, verifier, or refiner returns findings, the system shall upsert the findings through the store and include them in the returned run report.

**AUDIT-05** When a check or verifier fails while other work can continue, the system shall mark the audit run partial and continue processing the remaining configured work.

**AUDIT-06** When a finding carries a `StrategyAuto` command, the system shall persist and return the suggestion without executing the command or causing a remediation-induced finding-state transition.

**AUDIT-07** When a remediation suggestion is stored in memory or SQLite, the system shall round-trip its strategy, command, patch, title, and body without dispatching side effects.

**AUDIT-08** When `workflow-audit show` renders a finding, the system shall expose its stored remediation suggestion to the operator.

**AUDIT-09** When a check re-emits an acknowledged finding, the system shall preserve the acknowledged finding state.

**AUDIT-10** When side-effecting remediation is required, the system shall keep dispatch outside `pkg/audit` until a separate durable module has a proven live producer and consumer.

**AUDIT-11** When a check or verifier emits a finding with an unspecified remediation strategy, the system shall apply the configured severity-policy default before validating and storing the finding.

**AUDIT-12** When a remediation suggestion uses a recognized strategy, the system shall persist and return any subset of command, patch, title, and body, including a PR suggestion with no patch, as optional operator context without representing applicability, authority, or an applicable patch.

**AUDIT-13** When a finding with an unknown remediation strategy is written through `Store.UpsertFinding` to memory or SQLite, the system shall reject the finding before mutating finding state.

**AUDIT-14** When SQLite contains an unknown remediation strategy, the system shall return an error from `GetFinding` or `ListFindings` when either path encounters the row.

**AUDIT-15** When finding re-emission or a finding-state transition encounters an unknown stored remediation strategy, the system shall reject the operation before mutating the stored finding.

**AUDIT-16** When a supplied severity policy omits a required severity, contains an unknown severity, or contains an unknown or unspecified default strategy, the system shall reject the plan before creating an audit run record.

**AUDIT-17** When a check or verifier finding fails validation or persistence, the system shall record an outcome error and let that rejected finding contribute only a partial-run signal; cancellation or a separate persisted fail-run finding may still finish the run failed.
