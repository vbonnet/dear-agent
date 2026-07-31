# Harness-neutral SPEC governance specification

<!-- Last audited at: 2026-07-31 -->

**Status:** Active
**Scope:** Canonical authoring, auditing, evidence, reporting, and distribution
behavior for repository specifications.

## EARS Requirements

**SPEC-GOV-01** When an agent writes or revises a normative specification, the system shall classify each statement as a shared contract, capability variation, native adapter or projection, or internal implementation detail before selecting its owner.

**SPEC-GOV-02** When an observable behavioral invariant applies to more than one implementation or harness, the system shall designate exactly one canonical `SPEC.md` owner for that invariant.

**SPEC-GOV-03** When a shared contract does not apply uniformly, the system shall record a supported, adapted, unsupported, or not-applicable disposition for every active member.

**SPEC-GOV-04** When a local specification describes a native adapter or projection, the system shall state only the local observable delta and shall reference the canonical shared contract when one exists.

**SPEC-GOV-05** When an audit begins, the system shall pin the Git revision, inventory every tracked `SPEC.md` from Git objects, and account for every discovered file exactly once.

**SPEC-GOV-06** When deterministic collection finds equal text, repeated identifiers, shared BDD references, or harness terminology, the system shall treat those facts as candidate seeds and shall not treat them as semantic consolidation verdicts.

**SPEC-GOV-07** When an audit classifies a candidate, the system shall record exact source evidence, shared outcomes, material differences, ownership, applicability, BDD consequence, confidence, limitations, and the maintainer decision required.

**SPEC-GOV-08** When an audit reports a candidate, the system shall use `merge-now`, `extract-neutral-contract`, `keep-separate`, `resolve-product-divergence`, or `insufficient-evidence` as its bounded disposition.

**SPEC-GOV-09** When an audit runs, the system shall not modify product specifications, BDD features, implementation files, issue state, or delivery state.

**SPEC-GOV-10** When an audit report is rendered, the system shall produce self-contained offline HTML from a schema-validated structured finding artifact.

**SPEC-GOV-11** While a candidate and its canonical owner remain unselected by a maintainer, the system shall not consolidate or delete requirements on the candidate's behalf.

**SPEC-GOV-12** If required source, BDD, revision, or ownership evidence is unavailable, then the system shall label the affected result `insufficient-evidence` and shall not make a positive consolidation recommendation.

**SPEC-GOV-13** When a canonical governance skill is exposed through multiple discovery surfaces, the system shall retain one authored workflow body and shall use contained aliases or deterministic generated projections for native entrypoints.

**SPEC-GOV-14** When skill behavior is validated, the system shall treat a false consolidation or false canonical-owner selection as a release-blocking defect.

**SPEC-GOV-15** When an audit makes a positive recommendation, the system shall require the current owners to exactly match authenticated source-evidence paths, record bounded ownership-completeness and selection rationale, and select an existing proposed owner only from that authenticated owner set.

## BDD Traceability

- Feature: `agm/test/bdd/features/marketplace_parity.feature`
- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
