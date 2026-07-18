# ADR Integrity Specification

<!-- Last audited at: 2026-07-18 -->

## EARS Requirements

**ADRLINT-01** When repository policy is loaded, the system shall require a positive line budget, unique scoped paths and indexes, unique aggregate paths, and reasoned exclusions.

**ADRLINT-02** When ADR inventory is built, the system shall inspect every Git-tracked numbered record and aggregate path.

**ADRLINT-03** If an ADR-shaped path is neither governed nor excluded, the system shall report an ungoverned-path violation.

**ADRLINT-04** When a numbered record is validated, the system shall require matching filename and H1 identities, one normalized primary status, and a scope-unique identity.

**ADRLINT-05** When a scope index is validated, the system shall require an exact one-to-one match of record identity, filename, title, and primary status.

**ADRLINT-06** When a record is Superseded, the system shall require a link to another ADR.

**ADRLINT-07** When a record, aggregate, or index exceeds the configured line budget or contains an unresolved relative link, the system shall report a violation.

**ADRLINT-08** When validation completes, the system shall return path/reason violations sorted deterministically and shall keep operational errors distinct.

## Invariants

- Numeric identity is scoped to one declared directory.
- Indexes are reviewed source, never generated or rewritten by the validator.
- Number gaps preserve deleted identity and do not require renumbering.
- The validator is read-only.

## BDD Traceability

- Feature: `agm/test/bdd/features/audit_package_guardrails.feature`
