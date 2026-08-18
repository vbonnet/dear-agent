# DEAR Audit Engine Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-08-02 -->

## Purpose

`pkg/audit` runs configured DEAR audit checks, stores audit runs and findings,
applies severity policy, invokes verifiers and refiners, and records proposals
for remediation. The package is intentionally repository-neutral so scheduled,
CLI, and workflow-triggered audits share the same lifecycle semantics.

Remediation fields are inert suggestions. `StrategyAuto` marks a command as
eligible for an external automation system; it does not authorize `Runner` to
execute the command or change finding state. The package likewise records PR
and issue proposals without dispatching them. Any side-effecting remediation
requires a separately chartered durable module with a proven live producer and
consumer, idempotency, persistence, and reconciliation semantics.

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
