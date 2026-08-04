# EARS and BDD traceability

## Strict EARS forms

Use one observable requirement per stable ID. The repository accepts these
forms:

```text
When <trigger>, the system shall <observable response>.
While <state>, the system shall <observable behavior>.
Where <feature applies>, the system shall <observable behavior>.
If <condition>, then the system shall <observable behavior>.
The system shall <ubiquitous behavior>.
The system shall not <prohibited behavior>.
```

Name a more precise system actor only when the repository linter accepts it and
the actor is the stable contract owner. Avoid pronouns whose referent can drift.

## Requirement quality

A requirement should establish:

- one actor or stable system boundary;
- one trigger, state, option, or prohibited condition;
- an externally observable success or failure result;
- explicit applicability when the claim is not universal; and
- terminology defined by the owning domain rather than a current code symbol.

Split independently testable outcomes. Conjunctions are acceptable only when
they form one atomic observation or one invariant record.

For example, an archival success gate can be one atomic requirement:

```text
When an active session is archived, the system shall stop its runtime and
persist its durable archived state before reporting successful archival.
```

The ordering and success gate are one observable promise. By contrast, a
member-specific notification should be a separate, applicability-scoped
requirement in the same canonical contract, with an explicit decision about
whether notification failure affects archival success.

## Implementation-neutral language

Shared requirements normally avoid:

- harness-specific commands, flags, settings keys, and directory names;
- private function, package, or struct names;
- an implementation-specific UI sequence;
- one implementation's retry, storage, or transport mechanism; and
- present-tense file layout used as a substitute for ownership.

If an exact native artifact is observable and stable, describe that outcome in
an applicability-scoped requirement under the canonical harness-neutral
contract. Do not create a harness- or implementation-specific specification;
keep replaceable translation and wiring details in architecture, code, or
lower-level tests.

## BDD mapping

Map every changed requirement to one of:

| Consequence | Use when |
| --- | --- |
| Existing scenario still proves it | Behavior and applicability are unchanged |
| Extend scenario outline examples | One rule applies across more implementations |
| Add rule or scenario | A new observable or failure mode exists |
| Consolidate scenarios | Copied scenarios assert one shared rule |
| Applicability-specific scenario | One member exposes a distinct observable outcome under the canonical contract |
| No BDD change, with reason | A deterministic unit or schema test is the correct proof |

Feature prose describes business rules and observable outcomes. Put technical
mechanics in step definitions or lower-level tests. A `Then` step should assert
an output visible outside the implementation boundary, not an internal call.

## Reciprocal traceability

Follow the repository's current convention:

1. The specification names the exact feature path under its traceability
   section.
2. The feature names each canonical harness-neutral `SPEC` whose requirement
   it exercises; native examples identify applicable members without naming
   local normative owners.
3. Requirement IDs remain stable when wording is clarified without changing
   the observable contract.
4. When ownership moves, update both directions and preserve migration evidence
   in the focused consolidation change.

Run `make lint-specs STRICT=1` (optionally with `PATHS=...`) and the affected
`internal/speccoverage` or BDD tests. Those checks validate syntax and declared
links; review must still prove that the scenario actually observes the
requirement.
