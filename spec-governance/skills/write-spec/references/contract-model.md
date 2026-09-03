# Contract model

Use this taxonomy before creating, moving, or consolidating a normative
requirement. A harness name is evidence to inspect, not an automatic defect.
"One neutral owner" means the product or domain contract that owns the
observable; it does not mean one repository-wide mega-specification.

## Classes

| Class | Canonical home | Native names allowed? | Test consequence |
| --- | --- | --- | --- |
| Shared observable contract | One `SPEC.md` beside the shared owner | Only capability or active-member predicates | One cross-implementation feature or scenario outline |
| Capability variation | Shared contract plus an applicability matrix or registry | Yes, as members of a finite inventory | Supported, adapted, unsupported, and failure examples |
| Observable native variation | The same harness-neutral `SPEC.md`, scoped by applicability | Yes, only as a finite member or capability predicate | Applicability-specific example plus shared conformance |
| Internal implementation detail | No normative requirement unless externally observable | Not applicable | Unit or integration test, architecture note if consequential |

## Ownership test

For each statement, answer in order:

1. What can a user, caller, operator, or external system observe?
2. Would the promise remain true if the harness, storage engine, CLI
   flag, or directory layout changed?
3. Which module owns the invariant rather than merely translating it?
4. Does an existing requirement already promise the same trigger and outcome?
5. Which active members support, adapt, omit, or cannot represent it?

If the behavior survives a replaceable mechanism, the mechanism is not its
canonical owner. If a native path or invocation is itself the promised output,
record it as an applicability-scoped requirement in the harness-neutral owner.
Do not create a `SPEC.md` beside a harness or implementation merely because it
translates or exposes the behavior.

## Implementation projections

When repository coverage requires an implementation directory to declare its
contract owner, use `SPEC.owner` with one canonical repository-relative path
to the neutral product or domain `SPEC.md`, for example:

```text
internal/hookparity/SPEC.md
```

The pointer is traceability, not a second normative contract: it cannot target
a dotted or bare harness configuration, registration, plugin, or grouped
harness root, cannot coexist with a local `SPEC.md`, and its target must retain
strict EARS and reciprocal BDD coverage. Use it only when the implementation
adds no distinct observable contract; a new observable requires an ownership
decision in the neutral product or domain.

## Shared contract pattern

State the outcome once, then make implementations examples:

```text
**SESSION-IDENTITY-01** When a supported harness creates a session, the system
shall persist one durable identity that later reads return unchanged.
```

The capability matrix can state that one harness adapts a native ID while
another persists an AGM ID. Their invocation details do not belong in the
shared requirement.

## Observable native variation pattern

When one member exposes a distinct stable outcome, keep it in the canonical
contract and make the narrower applicability explicit:

```text
**SESSION-NOTIFY-01** Where provider-native notifications are supported, when
the system archives a session, the system shall emit one notification after
durable archival succeeds.
```

The applicability matrix identifies the supporting member and its evidence.
The native invocation, path, envelope, and wiring remain architecture or test
facts unless one of them is itself the stable user-visible outcome. Even then,
the canonical contract remains harness- and implementation-neutral.

Native names may appear as finite applicability members or evidence references.
They must not become second normative owners for the same product promise.

## Migration boundary

An existing harness- or implementation-local `SPEC.md` is evidence to audit,
not deletion authority. Before retiring it, select the neutral product or
domain owner, preserve or deliberately migrate stable requirement IDs, update
reciprocal BDD links, record every active member's applicability, and retain
the source or test evidence for native conformance. Stop for maintainer review
when ownership or observable semantics are uncertain.

The deterministic source guard admits a same-change relocation or complete
retirement to that review only when the selected immutable snapshot has no
surviving reciprocal BDD or implementation ownership edge to the deleted path
and every replacement passes the ordinary strict checks. That is structural
graph evidence, not semantic deletion authority; the changed paths and reviewed
diff must still demonstrate the stable-ID preservation or deliberate retirement
decision.

If implementation source survives, deleting its `SPEC.owner` is blocked unless
the same directory gains a permitted local `SPEC.md` replacement that passes
strict neutrality and contract validation.

To prevent ownership from being erased by a relocation, the deterministic
guard conservatively treats any added implementation blob whose Git object ID
matches a deleted implementation blob from the directory whose `SPEC.owner`
edge was deleted as a possible relocation target. Immutable Git snapshots
encode content identity, not move intent, so the guard also classifies an
independently added object-identical blob as a possible relocation target;
modifications and additions with different object IDs do not create that
content-identity edge.

## Capability variation pattern

Do not flatten a missing capability into false parity. Record a disposition:

| Member | Disposition | Evidence |
| --- | --- | --- |
| A | supported | Native behavior directly satisfies the contract |
| B | adapted | Adapter supplies the observable outcome |
| C | unsupported | The implementation cannot represent the capability |
| D | not-applicable | Contract is outside this member's declared role |

An `unknown` observation is research state, not a final normative disposition.
Use `unsupported` when the contract belongs to the member's role but the member
cannot provide it. Use `not-applicable` when the contract is outside that
member's role altogether; absence of an implementation-only side effect is
normally `not-applicable`, not an unsupported shared capability.

When an adapter adds a member-specific observable side effect, decide explicitly
whether failure of that side effect fails the shared operation, delays shared
success, or is advisory. Record that decision in the canonical contract; do not
let implementation placement decide it implicitly.

## Boundaries that remain separate

- Permission persistence and command construction may share security
  vocabulary while having different triggers and observables.
- Config-directory and hook declarations are architecture or conformance
  evidence when they implement a shared hook policy; they are not local
  normative owners.
- A fixture or migration may intentionally reproduce requirement-shaped text;
  it is not a second product promise unless production consumes it as one.
- A security-sensitive native invocation may be an applicability-scoped
  observable in the canonical contract when the exact launched artifact is
  externally verifiable.
- Deprecated compatibility behavior is not part of active-member parity unless
  the product explicitly makes that promise.

## Unsafe shortcuts

- Do not merge solely because bodies or IDs match.
- Do not keep copies solely because paths differ.
- Do not create a harness- or implementation-local `SPEC.md` for discovery,
  co-location, or traceability convenience.
- Do not ban every vendor name from every specification.
- Do not choose the most central-looking directory without tracing ownership.
- Do not treat a green linter or one passing implementation as parity proof.
