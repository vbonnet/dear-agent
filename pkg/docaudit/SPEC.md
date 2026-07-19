# Document Audit Freshness Specification

<!-- Last audited at: 2026-07-18 -->

## EARS Requirements

**DOCAUDIT-01** When repository policy is loaded, the system shall require each declared living-document surface to have a unique name, valid glob, owner, verification command, and positive maximum age.

**DOCAUDIT-02** When repository documents are discovered, the system shall inspect every Git-tracked path matching exactly one declared surface through context-cancellable, non-interactive Git commands with diagnostic stderr.

**DOCAUDIT-03** If a tracked document matches multiple declared surfaces, the system shall return an operational error.

**DOCAUDIT-04** When a baseline revision is supplied, the system shall reject policy changes that remove governed coverage from a surviving tracked document or increase its maximum audit age.

**DOCAUDIT-04** When a declared document lacks exactly one canonical audit marker, the system shall classify the marker as missing, placeholder, malformed, or duplicate.

**DOCAUDIT-05** When a canonical audit date is impossible, future-dated, or older than the declared maximum age, the system shall classify the corresponding date finding.

**DOCAUDIT-06** When current findings differ from the checked-in baseline, the system shall report both new findings and stale baseline entries.

**DOCAUDIT-07** When a base ref has a living-document policy, the system shall compare against the baseline path declared by that base policy and report every current baseline identity absent there, including after a baseline-path rename.

**DOCAUDIT-08** When a base ref has no baseline, the system shall permit the initial reviewed bootstrap.

**DOCAUDIT-09** When the repository is intact, the system shall return a nonblocking report that still exposes all baselined findings.

## Invariants

- Finding identity is `<kind><TAB><repo-relative-path>`.
- Baseline entries are sorted, unique, and exact; the module never edits them.
- The injected as-of date is normalized to a UTC calendar day.
- Git, policy, baseline, and document failures are errors, not freshness debt.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
