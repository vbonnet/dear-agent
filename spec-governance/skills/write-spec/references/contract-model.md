# Contract model

Use this taxonomy before creating, moving, or consolidating a normative
requirement. A harness name is evidence to inspect, not an automatic defect.

## Classes

| Class | Canonical home | Native names allowed? | Test consequence |
| --- | --- | --- | --- |
| Shared observable contract | One `SPEC.md` beside the shared owner | Only capability or active-member predicates | One cross-implementation feature or scenario outline |
| Capability variation | Shared contract plus an applicability matrix or registry | Yes, as members of a finite inventory | Supported, adapted, unsupported, and failure examples |
| Native adapter or projection | Local `SPEC.md` beside the native artifact | Yes, when the native artifact is observable | Adapter discovery, translation, or wiring test plus shared conformance |
| Internal implementation detail | No normative requirement unless externally observable | Not applicable | Unit or integration test, architecture note if consequential |

## Ownership test

For each statement, answer in order:

1. What can a user, caller, operator, or external system observe?
2. Would the promise remain true if the harness, provider, storage engine, CLI
   flag, or directory layout changed?
3. Which module owns the invariant rather than merely translating it?
4. Does an existing requirement already promise the same trigger and outcome?
5. Which active members support, adapt, omit, or cannot represent it?

If the behavior survives a replaceable mechanism, the mechanism is not its
canonical owner. If a native path or invocation is itself the promised output,
it may remain a local contract.

## Shared contract pattern

State the outcome once, then make implementations examples:

```text
**SESSION-IDENTITY-01** When a supported harness creates a session, the system
shall persist one durable identity that later discovery returns unchanged.
```

The capability matrix can state that one harness adapts a provider-native ID
while another persists an AGM ID. Their launcher flags do not belong in the
shared requirement.

## Adapter pattern

A local adapter requirement names only the translation it owns:

```text
**PI-SKILL-01** When Pi loads repository skills, the system shall include the
canonical skill directory through `.pi/settings.json`.
```

This may reference a shared skill-discovery requirement. It must not repeat the
skill's workflow, completion contract, or every invariant the skill enforces.

## Capability variation pattern

Do not flatten a missing capability into false parity. Record a disposition:

| Member | Disposition | Evidence |
| --- | --- | --- |
| A | supported | Native behavior directly satisfies the contract |
| B | adapted | Adapter supplies the observable outcome |
| C | unsupported | Provider cannot represent the capability |
| D | not-applicable | Contract is outside this member's declared role |

An `unknown` observation is research state, not a final normative disposition.
Use `unsupported` when the contract belongs to the member's role but the member
cannot provide it. Use `not-applicable` when the contract is outside that
member's role altogether; absence of a provider-only side effect is normally
`not-applicable`, not an unsupported shared capability.

When an adapter adds a provider-only side effect, decide explicitly whether
failure of that side effect fails the shared operation, delays shared success,
or is advisory. Do not let adapter placement decide those semantics implicitly.

## Boundaries that remain separate

- Permission persistence and launcher command construction may share security
  vocabulary while having different triggers and observables.
- Config-directory and hook declarations remain adapter contracts even when a
  shared hook policy is canonical elsewhere.
- One workflow and several plugin manifests or skill roots are one behavioral
  contract plus separate discovery contracts.
- A fixture or migration may intentionally reproduce requirement-shaped text;
  it is not a second product promise unless production consumes it as one.
- A security-sensitive native invocation may be normative when the exact
  launched artifact is externally verifiable.
- Deprecated compatibility behavior is not part of active-member parity unless
  the product explicitly makes that promise.

## Unsafe shortcuts

- Do not merge solely because bodies or IDs match.
- Do not keep copies solely because paths differ.
- Do not ban every vendor name from every specification.
- Do not choose the most central-looking directory without tracing ownership.
- Do not treat a green linter or one passing implementation as parity proof.
