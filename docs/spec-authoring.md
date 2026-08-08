# Authoring `SPEC.md` contracts

Use this page as the repository entry point for behavioral specifications.
The complete authored workflow is the canonical
[`write-spec` skill](../spec-governance/skills/write-spec/SKILL.md). Do not
copy its workflow into a harness directory, a package-local note, or another
skill.

## Contract ownership

A `SPEC.md` records an observable product contract, not current wiring. One
shared observable has one product or domain owner that is neutral to harness
and implementation placement. A native command, path, or provider result that
is itself observable remains in that same owner as an explicitly
applicability-scoped requirement. Translation, storage, invocation, and other
replaceable mechanics belong in architecture, code, or lower-level tests.

When the evidence cannot establish a shared outcome, compatibility boundary,
or member applicability, stop for a product decision. Existing
harness- or implementation-local `SPEC.md` files are audit candidates, not
permission to delete or merge anything.

## Implementation projections

An implementation directory that only adapts a contract owned elsewhere must
not create a second `SPEC.md` for co-location convenience. Add a `SPEC.owner`
file containing exactly one canonical repository-relative path to the neutral
product or domain `SPEC.md`, for example:

```text
internal/hookparity/SPEC.md
```

The target cannot live in a dotted or bare harness configuration,
registration, plugin, or grouped harness root, and the implementation directory
cannot declare both `SPEC.md` and `SPEC.owner`. Repository coverage applies the
target's strict EARS and reciprocal BDD checks through the pointer. Use this
only when the implementation adds no distinct observable contract; a new
observable requires an ownership decision in the neutral product domain.

## Start here

1. Read the canonical [`write-spec` workflow](../spec-governance/skills/write-spec/SKILL.md).
2. Use the [contract model](../spec-governance/skills/write-spec/references/contract-model.md)
   to classify the behavior before choosing an owner.
3. Draft from [`docs/templates/SPEC.md.tmpl`](templates/SPEC.md.tmpl), then
   follow the canonical [EARS and BDD guidance](../spec-governance/skills/write-spec/references/ears-and-bdd.md).
4. Run `make lint-specs STRICT=1` and the affected reciprocal SPEC/BDD checks.
   Passing checks establish syntax and declared links, not semantic ownership
   or runtime parity.

For a read-only corpus review, use the canonical
[`audit-specs` skill](../spec-governance/skills/audit-specs/SKILL.md). Its
inventories and HTML reports are maintainer-review evidence only; they do not
authorize a product SPEC migration.

## Evidence boundary

Report source, local-test, CI, review, merge, install/discovery, and runtime
facts separately. Repository projections and catalogs do not prove that a
harness discovers or invokes a skill; runtime behavior remains `UNVERIFIED`
until separately observed.
