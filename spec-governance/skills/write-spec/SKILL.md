---
name: write-spec
description: "Write or revise SPEC.md as an observable, implementation- and harness-neutral contract. Use when behavior needs normative EARS requirements, cross-harness consistency, an explicit capability variation, or BDD traceability. Do not use for behavior-preserving refactors or non-normative prose edits."
---

# Write a specification

A `SPEC.md` defines observable product behavior, not a copy of the current
implementation. State an outcome once in a harness- and implementation-neutral
owner. A native command, path, or provider outcome belongs in that owner only
as an explicit applicability-scoped observable; replaceable wiring belongs in
architecture, code, or tests, never a harness-local `SPEC.md`.

The neutral owner is the product or domain contract that owns the observable,
not one repository-wide specification. Native names may identify finite
applicability members or evidence, but they do not become additional normative
owners.

This `SKILL.md` is the one canonical authored workflow for `write-spec`.
It is source guidance, not a harness registration or proof that any provider
discovers or invokes the skill.

## Workflow

1. Read the nearest `AGENTS.md`, `ARCHITECTURE.md`, affected specifications,
   linked BDD features, and the source that owns the active implementation or
   harness inventory. Do not infer current behavior from a stale plan or PR.
2. State the proposed observable behavior: actor, trigger or state, successful
   outcome, failure outcome, and applicability. Separate product choices from
   facts already established by source or tests.
3. Read [the contract model](references/contract-model.md) and classify every
   statement as a shared contract, capability variation, observable native
   variation, or internal implementation detail.
4. For a shared contract, identify exactly one harness-neutral product
   `SPEC.md` owner before drafting. Search existing requirements and BDD
   scenarios for that outcome. Amend the owner; do not create a second owner
   beside each harness or implementation.
5. For a capability variation, give every active member a `supported`, `adapted`,
   `unsupported`, or `not-applicable` disposition. Never promise false parity
   merely to make the requirement uniform.
6. For an observable native variation, add an applicability-scoped requirement
   to the same harness-neutral product owner. Do not create a local `SPEC.md`
   beside a harness or implementation. Put translation and wiring mechanics in
   architecture, code, or tests.
7. Read [EARS and BDD traceability](references/ears-and-bdd.md). Draft stable
   IDs and strict EARS requirements. Describe observable results and failures;
   move replaceable mechanisms into architecture, code, or tests.
8. Map each changed requirement to its feature, rule, and scenario impact.
   Require one shared scenario or scenario outline across implementations for a
   shared contract; copied per-harness scenarios require an explicit
   applicability-only justification and must not create another normative
   owner.
   Cover failure behavior when the outcome can fail.
9. Run the narrowest relevant deterministic checks, including
   `make lint-specs STRICT=1` (optionally with `PATHS=...`) and affected
   reciprocal SPEC/BDD coverage or feature tests. Lint success proves syntax
   and links, not semantic ownership.
10. Report one outcome: `spec updated`, `no normative behavior changed`, or
    `needs product decision`. Separate source, local test, CI, review, merge,
    and runtime evidence. Keep runtime behavior `UNVERIFIED` until current live
    evidence proves it.

## Stop conditions

Stop with `needs product decision` when evidence cannot establish whether two
promises describe the same behavior, when implementations contradict the same
claimed contract, or when applicability is a product choice. Do not resolve
those cases by duplicating, deleting, or silently choosing one implementation.

Treat existing harness- or implementation-local specifications as audit and
migration candidates, not as permission to delete them immediately. Preserve
stable IDs, reciprocal BDD links, member applicability, and source or test
evidence until a maintainer approves a neutral owner and the migration is
complete.

## Verify

Confirm all of the following before presenting a proposed change:

- each shared observable has one owner;
- every applicability-scoped requirement remains under one harness-neutral
  owner rather than a harness- or implementation-local `SPEC.md`;
- every active member has a disposition where parity is claimed;
- EARS statements pass strict lint and avoid implementation leakage;
- BDD observes outcomes rather than private mechanics; and
- source, local tests, CI, review, merge, and runtime state are reported
  separately, with unobserved runtime state marked `UNVERIFIED`.

## References

- [Contract model](references/contract-model.md)
- [EARS and BDD traceability](references/ears-and-bdd.md)
- Repository `AGENTS.md` and `docs/policies/harness-hygiene.ai.md`
- Repository `cmd/ears-lint` and `internal/speccoverage`
