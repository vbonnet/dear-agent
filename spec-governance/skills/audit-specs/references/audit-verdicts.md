# Audit verdicts

## Verdicts

| Verdict | Required evidence | Meaning |
| --- | --- | --- |
| `merge-now` | Same actor, trigger, observable, applicability, and compatible BDD proof | One file can own the contract without a product choice |
| `extract-neutral-contract` | Shared outcome is repeated through multiple adapters or implementations, with real local deltas | Create one shared owner; retain narrow local translations |
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

## Confidence

- `confirmed`: exact source and executable behavior establish the relation.
- `likely`: multiple independent facts support the relation, with a named gap.
- `tentative`: useful lead, but ownership or behavior remains unverified.

Only `confirmed` findings should be presented as ready consolidation work.
`Likely` and `tentative` findings request research or a product decision.

## Required evidence per candidate

1. Exact revision and scope.
2. Every relevant path, line, requirement ID, and short excerpt.
3. The observable actor, trigger, result, and failure for each statement.
4. Material differences, even when the verdict is positive.
5. A current-owner set that exactly matches the cited source paths, plus a
   bounded completeness rationale describing the searches and seeds reviewed.
6. Proposed canonical owner with a separate selection rationale and exact
   pinned evidence when that owner already exists. An existing proposed owner
   must already be in the authenticated current-owner set; otherwise propose a
   new owner rather than laundering an unrelated SPEC into the topology.
7. Active-member applicability and unsupported cases.
8. Existing BDD features or lower-level tests and the migration consequence.
9. Risk, limitation, confidence, strength, and maintainer decision.

## High-value patterns

- A shared registry or matrix is repeated inside each native directory spec.
- Command and implementation packages repeat the same ownership invariant.
- Provider implementations repeat a common lifecycle contract plus real local
  mechanics.
- Several event hooks repeat shared resolution and failure semantics while
  differing only in event-specific output.
- Two commands repeat repository inventory/exclusion behavior that belongs to
  one inventory module.

## Near misses

- Same ID prefix in unrelated domains is an ID collision, not a merge.
- One feature file may exercise several distinct rules.
- Permission policy and launcher construction can share mode names but expose
  different behavior.
- Native hook declarations remain local even when hook policy is shared.
- Provider-specific filesystem isolation must not be merged wholesale into a
  generic sandbox contract.
- Compatibility specs may intentionally preserve behavior excluded from the
  active-member contract.

## Recommendation strength

Use `strong`, `moderate`, or `exploratory`. A strong recommendation needs
confirmed evidence, a defensible owner, and a bounded BDD migration. A high
text-similarity score alone is never strong.
