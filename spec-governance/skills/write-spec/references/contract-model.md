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

When the target repository's discovered governance contract declares an
implementation-owner projection such as `SPEC.owner`, follow that repository's
normative ownership and coverage contracts for the exact declaration syntax,
target constraints, and inherited checks. Do not copy those repository-specific
rules into this portable model.

The projection is traceability, not a second normative contract. Run each
projection and canonical-target check exactly where that repository's contract
assigns it, and record every absent evidence layer as unavailable. Use a
projection only when the implementation adds no distinct observable contract;
a new observable requires an ownership decision in the neutral product or
domain.

Do not invent `SPEC.owner` in a target repository that does not declare this
capability. Record the projection layer as unavailable, follow the repository's
native ownership mechanism when one exists, or stop for a product decision
when neutral ownership cannot be represented.

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

When the target repository declares a deterministic source guard for contract
migration or retirement, follow that guard's admission contract before asking
for semantic review. Passing structural checks is not semantic deletion
authority; the changed paths and reviewed diff must still demonstrate stable-ID
preservation or a deliberate retirement decision.

When the target repository declares `SPEC.owner` projection support and
implementation source survives, preserve its ownership projection or supply an
allowed local replacement as required by that repository's guard. If either the
projection or guard capability is absent, record that evidence layer as
unavailable and follow the repository's native migration checks; do not invent
guard evidence or import dear-agent's blocking rules.

When a repository's guard classifies an added directory as a possible
relocation target, authors must preserve a valid ownership projection or an
allowed local replacement there before semantic review. That is an
author-facing admission consequence, not proof of move intent. The exact
relocation evidence and conservative false-positive boundary belong only in
the target repository's normative guard contract; discover and follow that
contract instead of copying its algorithm into this portable model.

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
