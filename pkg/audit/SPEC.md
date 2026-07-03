# DEAR Audit Engine Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`pkg/audit` runs configured DEAR audit checks, stores audit runs and findings,
applies severity policy, invokes verifiers and refiners, and records proposals
for remediation. The package is intentionally repository-neutral so scheduled,
CLI, and workflow-triggered audits share the same lifecycle semantics.

## EARS Requirements

**AUDIT-01** When an audit runner is created, the system shall install the default check registry, logger, remediation strategy, clock, and identifier generator.

**AUDIT-02** When an audit run starts, the system shall reject invalid plans before creating an audit run record in the configured store.

**AUDIT-03** When no severity policy is supplied, the system shall apply the package default severity policy for failing, notifying, and remediation strategy selection.

**AUDIT-04** When a check, verifier, or refiner returns findings, the system shall upsert the findings through the store and include them in the returned run report.

**AUDIT-05** When a check or verifier fails while other work can continue, the system shall mark the audit run partial and continue processing the remaining configured work.
