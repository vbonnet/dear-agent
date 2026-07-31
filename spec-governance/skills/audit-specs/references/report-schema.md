# Report schema and authenticated HTML contract

The structured finding artifact uses `spec-audit/v1`. It is the source for the
HTML renderer; HTML is a complete view, not a second findings store. A supplied
inventory is not trusted on its own: validation and rendering recompute it from
Git objects at the pinned revision.

## Required commands

```sh
go run ./spec-governance/skills/audit-specs/scripts/specaudit inventory \
  -repo <repository-path> \
  -repository <owner/name> \
  -revision <40-hex-sha> > inventory.json

go run ./spec-governance/skills/audit-specs/scripts/specaudit validate \
  -input findings.json \
  -inventory inventory.json \
  -repo <repository-path>

go run ./spec-governance/skills/audit-specs/scripts/specaudit render \
  -input findings.json \
  -inventory inventory.json \
  -repo <repository-path> > report.html
```

`-repository` is an explicit stable identity. It prevents different clone URLs
or worktree names from changing deterministic inventory output. Inventory and
render commands open no output path; authorized shell redirection stores their
standard output so existing path guards see the destination.

## Top-level object

```json
{
  "schema_version": "spec-audit/v1",
  "snapshot": {
    "repository": "owner/name",
    "revision": "40-hex commit",
    "comparison_revision": "optional 40-hex comparison only",
    "revision_committed_at": "RFC 3339 commit timestamp",
    "generated_at": "optional RFC 3339 semantic report timestamp"
  },
  "scope": {
    "roots": ["."],
    "excluded": [{"path": "...", "reason": "..."}],
    "active_members": ["..."]
  },
  "summary": {
    "spec_files": 0,
    "requirements": 0,
    "diagnostics": 0,
    "candidate_count": 0,
    "by_verdict": {}
  },
  "methodology": {
    "collector": "command and version",
    "seed_kinds": ["exact-body", "duplicate-id", "shared-bdd", "identical-file"],
    "semantic_review": "bounded description",
    "reproduce": ["exact read-only commands"]
  },
  "inventory": [],
  "features": [],
  "seeds": [],
  "candidates": [],
  "non_candidates": [],
  "limitations": []
}
```

`revision_committed_at` is the commit time. Do not label it as report creation
time. `generated_at` is optional because it is not deterministic. A comparison
SHA is metadata only unless a separate inventory authenticates it.

## Inventory objects

The collector emits every tracked `SPEC.md` once:

```json
{
  "path": "relative/SPEC.md",
  "sha256": "64-hex content digest",
  "requirements": [{
    "id": "REQ-01",
    "line": 12,
    "body": "normalized body",
    "excerpt": "exact source line"
  }],
  "bdd_features": [{"path": "agm/test/bdd/features/example.feature", "line": 21}],
  "diagnostics": [{
    "line": 30,
    "kind": "anonymous-requirement|nonconforming-requirement|missing-bdd-feature|nonreciprocal-bdd-feature",
    "excerpt": "exact source or referenced path"
  }]
}
```

Feature inventory objects retain the pinned digest and reciprocal owners:

```json
{
  "path": "agm/test/bdd/features/example.feature",
  "sha256": "64-hex content digest",
  "related_specs": ["relative/SPEC.md"]
}
```

Seeds contain `id`, one supported `kind`, a deterministic `key`, and at least
two structured evidence records from distinct specification paths. Seeds are
lexical leads only; they never constitute a consolidation verdict.

## Finding object

```json
{
  "id": "SPEC-CLUSTER-001",
  "rank": 1,
  "title": "Short decision title",
  "verdict": "extract-neutral-contract",
  "relationship": "overlapping-observables",
  "classification": "shared-contract",
  "confidence": "confirmed",
  "strength": "strong",
  "current_owners": [{
    "path": "path/SPEC.md",
    "rationale": "why this path normatively owns the outcome"
  }],
  "ownership_completeness": "bounded search and seed evidence for why the set is complete",
  "proposed_owner": {
    "path": "path/SPEC.md",
    "state": "existing|new",
    "rationale": "why this owner is the correct implementation or contract seam"
  },
  "shared_outcome": "observable behavior",
  "material_differences": ["explicit difference or none observed"],
  "evidence": [{
    "path": "relative/SPEC.md",
    "line": 12,
    "requirement_id": "REQ-01",
    "excerpt": "exact pinned source line"
  }],
  "applicability_basis": "active-members|implementation-only",
  "applicability_rationale": "why this basis is correct",
  "applicability": [{
    "member": "codex-cli",
    "disposition": "supported|adapted|unsupported|not-applicable|unknown",
    "evidence": [{"path": "path/SPEC.md", "line": 12, "requirement_id": "REQ-01", "excerpt": "exact line"}]
  }],
  "bdd": {
    "features": ["agm/test/bdd/features/example.feature"],
    "consequence": "merge|add-matrix|adapter-only|none|resolve"
  },
  "recommendation": ["ordered change"],
  "risk": "bounded risk",
  "limitations": ["gap"],
  "decision": "question for maintainer",
  "boundary": "required for keep-separate"
}
```

Candidate ranks are unique and contiguous from one. `non_candidates` omit rank,
use `keep-separate`, and require a boundary. Histograms count candidates and
non-candidates; `candidate_count` counts only `candidates`.

## Enumerations

- verdict: `merge-now`, `extract-neutral-contract`, `keep-separate`,
  `resolve-product-divergence`, `insufficient-evidence`
- relationship: `same-observable`, `overlapping-observables`,
  `contradictory-observables`, `same-vocabulary-only`,
  `fixture-or-generated-copy`
- classification: `shared-contract`, `capability-variation`, `native-adapter`,
  `wrapper`, `fixture`, `implementation-detail`
- confidence: `confirmed`, `likely`, `tentative`
- strength: `strong`, `moderate`, `exploratory`
- applicability basis: `active-members`, `implementation-only`
- disposition: `supported`, `adapted`, `unsupported`, `not-applicable`, `unknown`

## Fail-closed validation

- Every validation and render requires both `-inventory` and `-repo`, including
  a zero-finding report.
- The inventory is recomputed from the pinned Git revision and compared in
  full: snapshot, scope, counts, methodology, files, features, seeds, and
  limitations.
- The current-owner set exactly equals the distinct paths in source evidence;
  an evidence path cannot be retained while its owner is omitted. Each path,
  line, ID, and excerpt equals the pinned inventory.
- Positive findings record both an owner-set rationale and a bounded
  completeness rationale. At least two current owners are required.
- An `existing` proposed owner must be a member of that authenticated current
  owner set, which gives it exact pinned requirement evidence. A `new` proposed
  owner must not exist at the pinned revision or appear in the current-owner
  set. Both states require a selection rationale.
- When a finding selects BDD features, every selected feature exists in the
  pinned tree and reciprocally links at least one current owner in both
  directions, and every current owner is covered by at least one selected
  feature. The selected features need not form a false Cartesian product with
  every owner. A finding with no selected feature must be non-positive and use
  BDD consequence `none`.
- `active-members` findings cover every pinned member with exact evidence.
  `implementation-only` findings state a rationale and cannot hide a harness
  configuration owner.
- Positive findings require at least two SPEC owners, confirmed evidence,
  non-exploratory strength, authenticated ownership topology and rationale, a
  proposed owner and owner state, BDD impact, material differences,
  recommendations, risk, and a maintainer decision.
- `merge-now` requires `same-observable`; `resolve-product-divergence` requires
  `contradictory-observables`. Fixtures and generated copies cannot become
  normative owners.

## HTML structure

The renderer produces one offline file containing the pinned and comparison
labels, corpus and diagnostic metrics, scope and exclusions, explicit top rank,
ownership topology, every finding field, active-member matrix, exact evidence,
BDD paths, recommendations, decisions, keep-separate boundaries, seed summary,
methodology, commands, source disclosure, and limitations. It embeds its CSS
and minimal filter logic and loads no network font, style, script, or data.
