# ADR Integrity Specification

<!-- Last audited at: 2026-07-19 -->

## EARS Requirements

**ADRLINT-01** When repository policy is loaded, the system shall reject unknown keys inside `adr-governance` while allowing unrelated top-level repository configuration, and shall require unique scoped paths and indexes, unique aggregate paths, and reasoned exclusions.

**ADRLINT-02** When ADR inventory is built, the system shall inspect every Git-tracked numbered record and aggregate path through the caller's context without allowing an interactive Git prompt.

**ADRLINT-03** If an ADR-shaped path, including a canonical three- or four-digit record, is neither governed nor excluded, the system shall report an ungoverned-path violation.

**ADRLINT-04** When a numbered record is validated, the system shall require matching filename and rendered H1 identities, one rendered normalized primary status, and a scope-unique identity while ignoring code examples.

**ADRLINT-05** When a scope index is validated, the system shall require an exact one-to-one match of record identity, filename, title, and primary status from rendered rows outside code examples.

**ADRLINT-06** When a record is Superseded, the system shall require a resolving repository-local link to another governed ADR and shall reject self-links, external ADR-shaped URLs, and missing targets.

**ADRLINT-07** When rendered Markdown outside code spans and code blocks in a record, aggregate, or index contains an unresolved relative link, a link that escapes the repository, or an undefined reference-style link label, the system shall report a violation.

**ADRLINT-10** When a tracked ADR-like filename in a declared scope is malformed, the system shall report it rather than silently omitting it from governance.

**ADRLINT-12** When ADR identities use different zero-padding widths within one scope, the system shall compare their numeric values and report a collision.

**ADRLINT-13** When a scope or aggregate declares a positive `max-lines` override, the system shall enforce that tighter review budget instead of the repository default.

**ADRLINT-11** When a declared scope index is not Git-tracked or contains any numeric ADR-like row outside the supported identity width or row schema, the system shall report a violation.

**ADRLINT-08** When validation completes, the system shall return path/reason violations sorted deterministically and shall keep operational errors distinct.

## Invariants

- Numeric identity is scoped to one declared directory.
- Indexes are reviewed source, never generated or rewritten by the validator.
- Number gaps preserve deleted identity and do not require renumbering.
- The validator is read-only.

## BDD Traceability

- Feature: `agm/test/bdd/features/audit_package_guardrails.feature`
