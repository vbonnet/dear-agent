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

A harness-registration `SPEC.md` may appear as a factual current owner because
the pinned source really makes that claim. It is never a neutral proposed owner
and may not retain the selected normative contract. Include `.agents/`,
`.claude/`, `.claude-plugin/`, `.codex/`, `.gemini/`, `.opencode/`, and `.pi/`
at any directory depth in that registration classification. Do not classify the
neutral `.dear-agent/` catalog as a harness registration merely because it is a
hidden directory.

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
   outside every known harness surface. When it is new, its directory must be
   neither equal to nor a path-component descendant of any current owner's
   directory. A root-directory current owner therefore blocks every nested new
   owner. Sibling directories and deceptive lexical prefixes such as `hook` and
   `hook-shared` remain distinct and eligible. Require exact pinned evidence when
   the owner already exists. An existing proposed owner must already be in the
   pinned current-owner set; otherwise propose a new owner rather than laundering
   an unrelated SPEC into the topology.
7. Active-member applicability and unsupported cases.
8. Existing BDD features or lower-level tests and the migration consequence.
   Each positive current owner has exactly one current or planned traceability
   state. Current coverage requires a selected reciprocal feature. A planned
   transfer is permitted only for an uncovered owner, must target the existing
   proposed owner through a selected feature already reciprocal with that owner,
   and must cite exact behavioral rows in that feature. Matching path topology,
   titles, tags, or shared vocabulary alone cannot establish behavioral
   equivalence; the validator resolves the citation but cannot prove its
   semantics.
9. A complete ownership plan pending maintainer approval with exactly one closed
   action for each current owner. Only an existing proposed owner may
   `retain-distinct-contract`. Use `retire-normative-ownership` only when every
   requirement and reciprocal feature owned by the source is selected for
   `transfer-to-proposed-owner` or `represent-as-applicability`. Otherwise use
   `retire-selected-normative-ownership`, migrate the selected records, and map
   every unselected requirement and reciprocal feature exactly back to its
   existing source with `preserve-in-place-pending-separate-audit`. Copy planned
   BDD transfers exactly into the ownership plan. A feature selected as current
   traceability may also remain in place when it still covers residual source
   behavior or is only topology evidence. Use `preserve-residual` when direct
   coverage still serves selected and residual behavior; use `add-matrix` and
   name the missing direct behavioral target for topology-only evidence rather
   than transferring the whole feature.
10. A coherent cross-finding plan. The same exact selected pinned requirement
    cannot receive incompatible dispositions or targets in separate candidates.
    If one source requirement combines observables for multiple owners, do not
    transfer it wholesale to either owner; require an explicit source split or
    report insufficient evidence first.
11. Risk, limitation, confidence, strength, and maintainer decision.

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
- One EARS line that combines two product domains is not two independently
  transferable requirements; split it before assigning canonical ownership.
- Implementation-specific filesystem isolation mechanics are not a second
  normative contract merely because they differ; do not merge them wholesale
  into a generic sandbox contract.
- Compatibility specs may intentionally preserve behavior excluded from the
  active-member contract.

## Recommendation strength

Use `strong`, `moderate`, or `exploratory`. A strong recommendation needs
confirmed evidence, a defensible owner, and a bounded BDD migration. A high
text-similarity score alone is never strong.
