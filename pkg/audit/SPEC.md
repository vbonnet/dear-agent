# DEAR Audit Engine Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-08-01 -->

## Purpose

`pkg/audit` runs configured DEAR audit checks, stores audit runs and findings,
applies severity policy, invokes verifiers and refiners, and records proposals
for remediation. The package is intentionally repository-neutral so scheduled,
CLI, and workflow-triggered audits share the same lifecycle semantics.

The exported `Remediator`, `ApplyOutcome`, and `Runner.Remediator` shapes are a
dormant compatibility seam. `Runner` ignores outcome status and reference,
passes a note to the store only with a valid finding-state change, and therefore
does not treat the outcome as durable remediation evidence. `NewRunner` installs
a side-effect-free no-op that preserves the stored finding state. Custom
remediators remain callable for compatibility but are deprecated; `ce-1hu9.13`
owns an idempotent remediation-event persistence and migration contract before
an in-repository side-effecting adapter may be activated.

## EARS Requirements

**AUDIT-01** When an audit runner is created, the system shall install the default check registry, logger, remediation strategy, clock, and identifier generator.

**AUDIT-02** When an audit run starts, the system shall reject invalid plans before creating an audit run record in the configured store.

**AUDIT-03** When no severity policy is supplied, the system shall apply the package default severity policy for failing, notifying, and remediation strategy selection.

**AUDIT-04** When a check, verifier, or refiner returns findings, the system shall upsert the findings through the store and include them in the returned run report.

**AUDIT-05** When a check or verifier fails while other work can continue, the system shall mark the audit run partial and continue processing the remaining configured work.

**AUDIT-06** When a remediator returns an outcome, the system shall discard the outcome status and reference without treating either value as evidence.

**AUDIT-07** When a remediator returns a valid finding state that differs from the stored state, the system shall pass the returned state and note to the finding store.

**AUDIT-08** When a remediator returns an invalid or unchanged finding state, the system shall discard the returned note without updating the finding state.

**AUDIT-09** When the default no-op remediator processes a finding, the system shall preserve the finding state.

**AUDIT-10** When a remediator persists a valid finding-state change, the system shall expose the updated finding state to the run report and refiners.
