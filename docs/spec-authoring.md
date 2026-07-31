# Authoring behavioral specifications

A `SPEC.md` is the canonical contract for observable product behavior. It is
not a description of the current implementation and it is not a place to copy
one promise into every harness, provider, transport, or storage adapter. State
a shared outcome once so one BDD rule can hold every applicable implementation
to the same promise.

This guide owns the repository's SPEC authoring rules. The normative product
governance requirements live in
[`spec-governance/SPEC.md`](../spec-governance/SPEC.md), and the canonical
[`write-spec`](../spec-governance/skills/write-spec/SKILL.md) skill is a thin
workflow entry point that delegates here. The guide's
[contract model](../spec-governance/skills/write-spec/references/contract-model.md)
owns the classification vocabulary and its
[EARS and BDD guide](../spec-governance/skills/write-spec/references/ears-and-bdd.md)
owns the detailed authoring forms. This living guide is the repository review
router; do not copy its rules into a skill or harness-specific guide.

## Before editing a specification

1. Name the actor, trigger or state, success result, failure result, and
   applicability.
2. Search existing requirements and BDD scenarios for the same observable.
3. Classify the statement as a shared contract, capability variation, native
   adapter or projection, or internal implementation detail.
4. Select exactly one canonical owner. A harness, command, manifest, generated
   file, or provider that merely translates behavior is not the shared owner.
5. Amend the existing owner when one exists; do not create a local copy for
   discoverability.
6. Record the BDD consequence before presenting the change.

A co-located file or traceability rule does not confer semantic ownership. If
a coverage gate cannot represent an implementation that has no local
observable delta, stop, report, and track the tooling limitation separately.
Do not expand a SPEC-only change into enforcement work or invent a local
product promise merely to satisfy the gate.

Shared requirements describe stable outcomes and normally omit private code
names, harness-specific commands and flags, settings keys, directory layouts,
provider UI sequences, and one implementation's retry or storage mechanism.
An exact native path or invocation may remain in a local adapter specification
when that artifact is itself an independently observable interface. That local
requirement states only the delta and references the shared requirement; it
does not repeat the shared workflow or completion contract.

## Capability variation is part of the contract

When a shared outcome varies, give every member of the active inventory one of
the canonical dispositions: `supported`, `adapted`, `unsupported`, or
`not-applicable`, with evidence. `unknown` is research state, not a final
normative disposition. Do not promise false parity to make the prose uniform.

If applicability is a product choice, or implementations contradict the same
claimed contract, stop for a product decision instead of silently selecting
one implementation's behavior.

## Shared contract versus copied contracts

Bad — each harness promises the same product result:

```text
agm/claude/SPEC.md: When a session is archived, the system shall stop it ...
agm/codex/SPEC.md:  When a session is archived, the system shall stop it ...
agm/pi/SPEC.md:     When a session is archived, the system shall stop it ...
```

Good — one session specification owns archival success. Local specifications
name only distinct native stop or notification behavior, and one scenario
outline evaluates every applicable harness.

A legitimate local adapter requirement looks like this:

```text
**PI-SKILL-01** When Pi loads repository skills, the system shall include the
canonical skill directory through `.pi/settings.json`.
```

It can reference a shared skill-discovery requirement, but it does not become
another owner of the skill's workflow. A harness name is evidence to inspect,
not automatic implementation leakage.

## EARS and BDD review

Give each independently testable promise a stable identifier and a strict EARS
statement. Every changed requirement must identify one test consequence:

- an existing scenario already proves it;
- one scenario outline gains applicable members;
- a new observable or failure mode needs a rule or scenario;
- copied scenarios consolidate around one shared rule;
- a native translation needs an adapter-specific scenario; or
- a deterministic schema, unit, or integration test is the correct proof, with
  the reason no BDD change is warranted.

Traceability is reciprocal: the specification names the exact feature path,
and the feature names its canonical specification plus any distinct adapter
specifications. BDD observes results outside the implementation seam; private
calls and data structures belong in lower-level tests.

## Review and maintainer boundary

Before proposing a change, verify that each shared observable has one owner,
every local requirement adds a real adapter or capability delta, every parity
claim covers the active inventory, strict EARS passes, and reciprocal BDD links
agree.

Physical separation, equal identifiers, similar wording, or a shared vendor
term are candidate evidence only. They do not prove semantic equivalence. When
ownership remains unclear, use [`audit-specs`](../spec-governance/skills/audit-specs/SKILL.md)
and stop at a maintainer decision; an audit does not consolidate source on its
own.

Run the narrowest affected checks during editing, then at minimum:

```sh
make lint-specs STRICT=1
make lint-skills
```

Run affected `internal/speccoverage` and BDD tests for changed links or
behavior. Passing syntax and traceability gates does not prove semantic
ownership; review must still establish that the selected module owns the
observable invariant.

Report the proposed outcome together with source, local-test, CI, review,
merge, installation, and runtime state as separate facts. Do not use success in
one delivery state as proof of another.
