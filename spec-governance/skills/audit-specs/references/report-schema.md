# Report schema and offline HTML contract

The structured artifact uses `spec-audit/v2`. It is the source for the HTML
renderer; HTML is a complete view, not a second findings store.

## Trust and interpretation

The target repository's Git metadata, common object store, and configured
object alternates are trusted inputs. Inventory records privacy-preserving,
recomputation-bound identities for the canonical Git executable, Git-resolved
worktree root, repository Git directory, common directory, object directory,
and ordered alternate object directories that Git may use, including nested
alternate routing. Cycles,
duplicates, inaccessible paths, and unresolved routes fail closed. Validation
recomputes the supplied inventory from objects resolved at its pinned revision.
This detects mismatched or invented evidence within that Git trust boundary; it
is not independent cryptographic attestation of the repository or object store.

If the Git inputs are untrusted, ambiguous, or inaccessible, stop with
`insufficient-evidence`. This source-only schema makes no runtime-consistency
claim; a runtime conclusion requires separate current evidence.

## Commands

Run from the dear-agent checkout that owns `tools/specaudit`:

```sh
go run ./tools/specaudit inventory \
  -repo <repository-path> \
  -repository <owner/name> \
  -revision <40-hex-sha> > inventory.json

go run ./tools/specaudit validate \
  -input findings.json \
  -inventory inventory.json \
  -repo <repository-path>

go run ./tools/specaudit render \
  -input findings.json \
  -inventory inventory.json \
  -repo <repository-path> > report.html
```

`-repository` is the stable repository label; it prevents clone and worktree
directory names from changing deterministic output. Commands emit inventory or
HTML bytes to standard output. The caller's authorized redirection selects the
destination.

## Top-level object

```json
{
  "schema_version": "spec-audit/v2",
  "snapshot": {
    "repository": "owner/name",
    "revision": "40-hex commit",
    "comparison_revision": "optional comparison-only 40-hex commit",
    "revision_committed_at": "RFC 3339 commit timestamp",
    "generated_at": "optional RFC 3339 semantic-report timestamp"
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
    "collector": "go run ./tools/specaudit inventory",
    "seed_kinds": [
      "exact-body",
      "duplicate-id",
      "shared-bdd",
      "identical-file",
      "harness-terminology"
    ],
    "semantic_review": "bounded description",
    "git_evidence_trust": "fixed trust-boundary disclosure",
    "git_trust_inputs": {
      "executable": "sha256:canonical-executable-content-digest",
      "worktree_root": "path-sha256:git-resolved-worktree-root-identity",
      "git_dir": "path-sha256:canonical-git-directory-identity",
      "common_dir": "path-sha256:canonical-common-directory-identity",
      "object_dir": "path-sha256:canonical-object-directory-identity",
      "alternate_object_dirs": ["path-sha256:canonical-alternate-directory-identity"]
    },
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

`revision_committed_at` is the commit time, not report creation time. A
comparison SHA is metadata only unless a separate inventory collects it.

## Inventory and leads

Each tracked specification appears once:

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
  "bdd_features": [{
    "path": "agm/test/bdd/features/example.feature",
    "line": 21
  }],
  "diagnostics": [{
    "line": 30,
    "kind": "anonymous-requirement|nonconforming-requirement|malformed-bdd-feature-reference|duplicate-bdd-feature-reference|ambiguous-bdd-traceability-section|missing-bdd-feature|nonreciprocal-bdd-feature",
    "excerpt": "exact source or referenced path"
  }]
}
```

Feature records retain their pinned digest, reciprocal SPEC paths, and
diagnostics:

```json
{
  "path": "agm/test/bdd/features/example.feature",
  "sha256": "64-hex content digest",
  "related_specs": ["relative/SPEC.md"],
  "diagnostics": [{
    "line": 1,
    "kind": "missing-feature-spec-reference|malformed-feature-spec-reference|ambiguous-feature-spec-reference|missing-feature-spec|nonreciprocal-feature-spec",
    "excerpt": "exact source or referenced path"
  }]
}
```

The diagnostic total includes SPEC-side and feature-side records. Reciprocity
is computed only from the pinned Git objects; dirty worktree bytes are not
evidence.

A supported SPEC-side traceability section uses exactly one of the current H2
headings: `BDD Traceability`, `Test Traceability`, `Package Test Traceability`,
or `Traceability`. A supported entry is a full-line
`` - `path.feature` `` bullet, optionally followed by descriptive prose. It may
use one of these finite prefixes: `Feature`, `Related feature`, `BDD`,
`BDD feature`, `BDD Tests`, `Command BDD`, `Cross-surface BDD`,
`Cross-surface contracts`, `Status BDD`, `Strict-spec linkage`, or
`Strictness BDD`. Free-form prose, unsupported labels, multiple backtick groups,
and feature paths outside a supported section are not traceability claims.

The inventory artifact is the only report form that contains `inventory`,
`features`, and `seeds`. A semantic findings report must omit all three fields,
including empty or null placeholders; validation rejects an embedded copy
before checking its findings against the separately supplied inventory.

Seeds have an ID, supported kind, deterministic key, and pinned evidence. Exact
bodies, duplicate IDs, shared BDD paths, identical files, and harness terms are
non-verdict review leads. A seed never authorizes a finding, owner choice, or
consolidation.

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
    "rationale": "why this owner is the correct contract seam",
    "neutrality_rationale": "why this path is outside harness surfaces and owns a neutral product seam"
  },
  "ownership_plan": {
    "approval": "pending-maintainer-approval",
    "current_owners": [{
      "path": "path/SPEC.md",
      "action": "retain|retire-normative-ownership",
      "rationale": "why this owner is retained or proposed for retirement",
      "preservation": {
        "requirements": [{
          "source": {"path": "path/SPEC.md", "line": 12, "requirement_id": "REQ-01", "excerpt": "exact pinned source line"},
          "target_id": "REQ-01",
          "target_state": "existing|planned",
          "strategy": "preserve-id|canonical-reference"
        }],
        "bdd": [{"feature": "agm/test/bdd/features/example.feature", "source_owner": "path/SPEC.md", "target_owner": "path/SPEC.md"}],
        "applicability_basis": "active-members",
        "applicability": []
      }
    }]
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
    "member": "member-name",
    "disposition": "supported|adapted|unsupported|not-applicable|unknown",
    "evidence": [{
      "path": "path/SPEC.md",
      "line": 12,
      "requirement_id": "REQ-01",
      "excerpt": "exact pinned line"
    }]
  }],
  "bdd": {
    "features": ["agm/test/bdd/features/example.feature"],
    "shared_contract_feature": "agm/test/bdd/features/example.feature",
    "consequence": "merge|add-matrix|adapter-only|none|resolve"
  },
  "recommendation": ["ordered proposed change"],
  "risk": "bounded risk",
  "limitations": ["gap"],
  "decision": "question for maintainer",
  "boundary": "required for keep-separate"
}
```

Candidate ranks are unique and contiguous from one. `non_candidates` omit rank,
use `keep-separate`, and name the boundary. Histograms count both candidates
and non-candidates; `candidate_count` counts only `candidates`.

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

`native-adapter` describes the placement found in the pinned corpus; it does
not authorize a harness- or implementation-local canonical owner.
`adapter-only` means an applicability-specific BDD consequence for a canonical
neutral requirement, not a local adapter `SPEC.md`.

## Validation boundary

Passing validation means the decision ledger is structurally complete and its
cited source matches the supplied, freshly recomputed pinned inventory. It does
not attest the Git store, prove semantic equivalence, select an owner, verify
runtime behavior, or authorize changes.

- Require both `-inventory` and `-repo`, including for a zero-finding report.
- Recompute the inventory from the pinned revision and compare the snapshot,
  scope, counts, methodology (including Git trust-input identities), files,
  features, leads, and limitations.
- Require each source-evidence path, line, ID, and excerpt to equal the pinned
  inventory and require the current-owner set to match the evidence paths.
- Require positive findings to name every current owner, a bounded
  completeness rationale, a defensible harness-neutral proposed owner,
  material differences, applicability, BDD consequences, risk, and maintainer
  decision. A new owner cannot be nested under a current-owner directory.
- Require every positive finding to carry a `pending-maintainer-approval`
  ownership plan with exact entries for all current owners. Retirements map
  every cited source requirement, selected reciprocal BDD feature, and exact
  applicability matrix to the proposed owner. Existing targets must already
  exist in the pinned proposed owner; planned targets are allowed only for a
  new owner. The plan is not change or deletion authority.
- Require every selected BDD feature to exist and reciprocally name at least one
  current owner; require every current owner to be represented by a selected
  feature. Positive `shared-contract` findings additionally identify a selected
  `shared_contract_feature` that reciprocally names every current owner. A
  finding without selected BDD uses non-positive consequence `none`.
- Require active-member findings to cover each pinned member with evidence.
  Do not hide an applicability-scoped contract behind `implementation-only`.
- Require `merge-now` to use `same-observable` and
  `resolve-product-divergence` to use `contradictory-observables`. Fixtures and
  generated copies cannot become normative owners.

## HTML contract

Render one bounded, self-contained offline file. Escape all untrusted text and
retain the snapshot, trust limitations, metrics, exclusions, ranked findings,
owner topology, neutrality rationale, pending ownership/preservation plan,
applicability, exact evidence, BDD consequences,
recommendations, decisions, keep-separate boundaries, lead and diagnostic
summary, methodology including every Git trust-input identity, and reproduction
commands.
Load no network font, style, script, or data.
