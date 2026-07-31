---
name: write-spec
description: "Write or revise SPEC.md as an observable, implementation- and harness-neutral contract. Use when behavior needs normative EARS requirements, cross-harness consistency, an explicit capability variation, or BDD traceability. Do not use for behavior-preserving refactors or non-normative prose edits."
---

# Write a specification

A `SPEC.md` defines observable product behavior, not a copy of the current
implementation. State a shared outcome once. Keep native commands, paths, and
discovery mechanisms in local adapter contracts only when those details are
themselves observable.

## Workflow

1. Read the nearest `AGENTS.md`, `ARCHITECTURE.md`, affected specifications,
   linked BDD features, and the source that owns the active implementation or
   harness inventory. Do not infer current behavior from a stale plan or PR.
2. State the proposed observable behavior: actor, trigger or state, successful
   outcome, failure outcome, and applicability. Separate product choices from
   facts already established by source or tests.
3. Read [the contract model](references/contract-model.md) and classify every
   statement as a shared contract, capability variation, native adapter or
   projection, or internal implementation detail.
4. For a shared contract, identify exactly one canonical owner before drafting.
   Search existing requirements and BDD scenarios for that outcome. Amend the
   owner; do not create a second owner beside each harness or implementation.
5. For a capability variation, give every active member a `shared`, `adapted`,
   `unsupported`, or `not-applicable` disposition. Never promise false parity
   merely to make the requirement uniform.
6. For an adapter or projection, describe only the local observable delta and
   reference the shared requirement. A wrapper, generated file, manifest, or
   symlink is a distribution mechanism, not a new owner of the workflow it
   exposes.
7. Read [EARS and BDD traceability](references/ears-and-bdd.md). Draft stable
   IDs and strict EARS requirements. Describe observable results and failures;
   move replaceable mechanisms into architecture, code, or adapter contracts.
8. Map each changed requirement to its feature, rule, and scenario impact.
   Prefer one scenario outline across implementations over copied scenarios.
   Cover failure behavior when the outcome can fail.
9. Run the narrowest relevant deterministic checks, including `make lint-specs`
   and affected reciprocal SPEC/BDD coverage or feature tests. Lint success
   proves syntax and links, not semantic ownership.
10. Report one outcome: `spec updated`, `adapter-only specification updated`,
    `no normative behavior changed`, or `needs product decision`.

## Stop conditions

Stop with `needs product decision` when evidence cannot establish whether two
promises describe the same behavior, when implementations contradict the same
claimed contract, or when applicability is a product choice. Do not resolve
those cases by duplicating, deleting, or silently choosing one implementation.

## Verify

Confirm all of the following before presenting a proposed change:

- each shared observable has one owner;
- every local requirement adds a real adapter or capability delta;
- every active member has a disposition where parity is claimed;
- EARS statements pass strict lint and avoid implementation leakage;
- BDD observes outcomes rather than private mechanics; and
- source, local tests, CI, review, merge, installation, and runtime state are
  reported separately.

## References

- [Contract model](references/contract-model.md)
- [EARS and BDD traceability](references/ears-and-bdd.md)
- Repository `AGENTS.md` and `docs/policies/harness-hygiene.ai.md`
- Repository `cmd/ears-lint` and `internal/speccoverage`
