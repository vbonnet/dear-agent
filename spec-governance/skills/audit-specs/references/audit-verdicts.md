# Audit verdicts

## Verdicts

| Verdict | Required evidence | Meaning |
| --- | --- | --- |
| `merge-now` | Same actor, trigger, observable, applicability, and compatible BDD proof | One file can own the contract without a product choice |
| `extract-neutral-contract` | Shared outcome is repeated through multiple adapters or implementations, with real applicability differences | Create one neutral owner; move native mechanics to architecture or tests and retain observable differences only as applicability-scoped requirements |
| `keep-separate` | Different actor, trigger, observable, lifecycle, security boundary, capability, or compatibility promise | Similar text is not duplicated behavior |
| `resolve-product-divergence` | Contradictory promises for the same declared behavior | Maintainer must select or redefine the contract before migration |
| `insufficient-evidence` | Missing source, BDD, revision, applicability, or ownership proof | No positive consolidation recommendation is safe |

## Semantic relationships

Record one relationship independently of the verdict:

- `same-observable`
- `overlapping-observables`
- `contradictory-observables`
- `same-vocabulary-only`
- `fixture-or-generated-copy`

Exact bodies can still be fixtures or incorrectly copied implementation rules.
Different wording can still express the same observable contract.

These relationships and verdicts are semantic judgments authored from pinned
evidence. Validation checks cited Git-resolved source, BDD topology, shape, and
bounded completeness fields; it cannot prove semantic equivalence, attest the
target Git store independently, or choose a canonical owner. A maintainer must
review every positive recommendation before it becomes authorized product work.

A neutral owner is the product or domain contract that owns the observable,
not a repository-wide mega-specification. Native names may remain finite
applicability members or evidence references, but never a second normative
owner for the same promise. Retiring a current local owner also requires an
explicit migration of stable IDs, reciprocal BDD links, applicability, and
source or test evidence.

## Confidence

- `confirmed`: exact pinned source, reciprocal BDD, and ownership evidence
  establish the relation within the source audit's stated trust boundary.
- `likely`: multiple independent facts support the relation, with a named gap.
- `tentative`: useful lead, but ownership or behavior remains unverified.

`Confirmed` does not mean runtime verified. Keep runtime consistency
`UNVERIFIED` unless separate current evidence proves it. Present only confirmed
findings as ready for maintainer review; never present them as authorized
consolidation work. `Likely` and `tentative` findings request research or a
product decision.

## Required evidence per candidate

1. Exact revision and scope.
2. Every relevant path, line, requirement ID, and short excerpt.
3. The observable actor, trigger, result, and failure for each statement.
4. Material differences, even when the verdict is positive.
5. A current-owner set that exactly matches the cited source paths, plus a
   bounded completeness rationale describing the searches and seeds reviewed.
6. Proposed canonical owner with separate selection and neutrality rationales,
   outside every known harness surface and not below any current-owner
   directory when it is new, plus exact
   pinned evidence when that owner already exists. An existing proposed owner
   must already be in the pinned current-owner set; otherwise propose a new
   owner rather than laundering an unrelated SPEC into the topology.
7. Active-member applicability and unsupported cases.
8. Existing BDD features or lower-level tests and the migration consequence.
   A positive shared-contract finding selects one reciprocal shared feature
   that names every current owner; narrower selected features may remain for
   their own coverage.
9. A complete ownership plan with exactly one retain or
   retire-normative-ownership entry for each current owner, pending maintainer
   approval, and structured requirement, BDD, and applicability preservation
   for every retirement.
10. Risk, limitation, confidence, strength, and maintainer decision.

## High-value patterns

- A shared registry or matrix is repeated inside each native directory spec.
- Command and implementation packages repeat the same ownership invariant.
- Native implementations repeat a common lifecycle contract plus real local
  mechanics.
- Several event hooks repeat shared resolution and failure semantics while
  differing only in event-specific output.
- Two commands repeat repository inventory/exclusion behavior that belongs to
  one inventory module.

## Near misses

- Same ID prefix in unrelated domains is an ID collision, not a merge.
- One feature file may exercise several distinct rules.
- Permission policy and command construction can share mode names but expose
  different behavior.
- Native hook declarations are implementation evidence, not local normative
  owners, even when each harness uses a different declaration mechanism.
- Implementation-specific filesystem isolation mechanics are not a second
  normative contract merely because they differ; do not merge them wholesale
  into a generic sandbox contract.
- Compatibility specs may intentionally preserve behavior excluded from the
  active-member contract.

## Recommendation strength

Use `strong`, `moderate`, or `exploratory`. A strong recommendation needs
confirmed evidence, a defensible owner, and a bounded BDD migration. A high
text-similarity score alone is never strong.
