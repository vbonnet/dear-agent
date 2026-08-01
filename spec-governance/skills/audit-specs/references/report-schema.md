# Report schema and offline HTML contract

The structured artifacts use `spec-audit/v2`. The immutable inventory and
reviewer-authored decision ledger are separate documents. HTML is a complete
view of both documents, not a second findings store. `spec-audit/v1` is
rejected: it conflated collected facts with reviewer judgments.

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

## Immutable inventory document

```json
{
  "schema_version": "spec-audit/v2",
  "document_kind": "inventory",
  "snapshot": {
    "repository": "owner/name",
    "revision": "40-hex commit",
    "comparison_revision": "optional comparison-only 40-hex commit",
    "revision_committed_at": "RFC 3339 commit timestamp",
    "generated_at": "optional RFC 3339 semantic-report timestamp"
  },
  "scope": {
    "roots": ["."],
    "excluded": [],
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
  "limitations": [],
  "collector_execution_disclosure": "fixed non-attesting disclosure",
  "collector_execution": {
    "build_info_available": true,
    "vcs_metadata_available": false,
    "module_path": "present only when self-reported build information supplies it",
    "vcs_revision": "present only with valid complete VCS metadata",
    "vcs_modified": "boolean present only with valid complete VCS metadata",
    "go_toolchain": "self-reported Go toolchain",
    "goos": "runtime GOOS",
    "goarch": "runtime GOARCH"
  }
}
```

`collector_execution` comes from `runtime/debug.ReadBuildInfo` and runtime
values. Availability is explicit: no fallback module path or inferred VCS
value is emitted. The fixed `collector_execution_disclosure` appears in JSON
and HTML and is not a provenance attestation. Collection does not accept
reviewer exclusions: it accounts for the complete pinned corpus.

## Reviewer decision ledger

```json
{
  "schema_version": "spec-audit/v2",
  "document_kind": "decision-ledger",
  "inventory_ref": "sha256:<canonical inventory JSON digest>",
  "review_scope": {"exclusions": [{
    "path": "fixture/SPEC.md",
    "classification": "fixture",
    "rationale": "review classification; still collected and not deletion authority",
    "supporting_evidence": [{"path": "fixture/SPEC.md", "line": 1, "excerpt": "fixture marker"}]
  }]},
  "summary": {"candidate_count": 0, "by_verdict": {}},
  "methodology": {"semantic_review": "review method", "reproduce": ["exact decision-review command"]},
  "candidates": [],
  "non_candidates": [],
  "limitations": []
}
```

The ledger must omit `inventory`, `features`, `seeds`, and
`collector_execution`, including empty and null placeholders. Its
`inventory_ref` is the SHA-256 digest of the canonical bounded JSON bytes
emitted for the exact inventory document. Validation recomputes the inventory
and digest from the pinned revision; exclusions remain visible, never suppress
collection, and cannot be selected as a positive current owner.

Reviewer exclusion classifications are closed to `fixture`, `generated`,
`third-party`, `archived`, and `nested-repository`. Each exclusion path must
resolve in the pinned inventory and carries pinned supporting evidence; it is
review scope, never deletion authority.

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

Evidence is explicitly typed. `normative-contract` evidence is an exact
requirement citation from an inventoried `SPEC.md`, must include
`requirement_id`, and is the only evidence type that can establish a current
owner, proposed owner, or applicability row. `supporting` evidence may cite
any bounded tracked regular blob at the pinned revision (for example an
implementation or lower-level test), has no `requirement_id`, and is checked
against that pinned blob during validate/render. Supporting evidence can
explain a judgment but cannot establish those ownership or applicability facts
alone. HTML labels both kinds.

In the persisted ledger these are separate fields, not a tagged union:
`contract_evidence` entries contain `path`, `line`, `requirement_id`, and
`excerpt`; `supporting_evidence` entries contain only `path`, `line`, and
`excerpt`. This prevents a supporting citation from being represented as a
normative requirement. `current_owners` means the pinned `SPEC.md` paths that
claim ownership at the audited snapshot; it is evidence, not policy approval.
For a positive finding, the ledger must identify one neutral proposed owner
with an explicit `neutrality_rationale`; the rationale explains why the owner
is a product/domain seam rather than a harness configuration or implementation
colocation. This is a maintainer-pending preservation plan, not deletion
authority: current-owner requirement evidence, BDD coverage, and any
applicability evidence remain visible until a maintainer selects and executes a
separate migration. Path colocation never supplies that legitimacy.

The current active-member extraction is a dear-agent-specific adapter over the
pinned Go registry paths `agm/internal/harnessregistry/registry.go` and legacy
`agm/internal/agent/harnesses.go`. If neither path exists or parsing fails,
`active_members` is empty with a limitation and an `active-members` parity
finding is rejected. A generic pinned registry format is future work; callers
cannot provide a regex or an unpinned comma list.

Validate and render authenticate input paths with no-follow descriptor
traversal only on Darwin and Linux. Other GOOS fail before report bytes are
read or artifacts are emitted. Cross-compilation checks only compile this
guard; they are not runtime authentication proof.

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
    "neutrality_rationale": "why this is a product/domain seam rather than a harness or implementation location"
  },
  "shared_outcome": "observable behavior",
  "material_differences": ["explicit difference or none observed"],
  "contract_evidence": [{
    "path": "relative/SPEC.md",
    "line": 12,
    "requirement_id": "REQ-01",
    "excerpt": "exact pinned source line"
  }],
  "supporting_evidence": [{"path": "internal/context.go", "line": 8, "excerpt": "optional pinned context"}],
  "applicability_basis": "active-members|non-harness-domain",
  "applicability_rationale": "why this basis is correct",
  "applicability": [{
    "member": "member-name",
    "disposition": "supported|adapted|unsupported|not-applicable|unknown",
    "contract_evidence": [{
      "path": "path/SPEC.md",
      "line": 12,
      "requirement_id": "REQ-01",
      "excerpt": "exact pinned line"
    }],
    "supporting_evidence": []
  }],
  "bdd": {
    "features": ["agm/test/bdd/features/example.feature"],
    "consequence": "merge|add-matrix|applicability-specific|none|resolve"
  },
  "recommendation": ["ordered proposed change"],
  "risk": "bounded risk",
  "limitations": ["gap"],
  "decision": "question for maintainer",
  "decision_status": "pending-maintainer-approval",
  "ownership_plan": {
    "status": "pending-maintainer-approval",
    "deletion_authority": false,
    "owner_actions": [{"owner_path": "path/SPEC.md", "disposition": "retain-distinct-contract|retire-normative-ownership", "rationale": "preservation decision pending maintainer approval"}],
    "requirements": [{"contract_evidence": {"path": "path/SPEC.md", "line": 12, "requirement_id": "REQ-01", "excerpt": "exact pinned source line"}, "disposition": "retain-distinct|transfer-to-proposed-owner|represent-as-applicability", "target_path": "path/SPEC.md", "target_requirement_id": "REQ-01", "target_state": "existing|planned", "rationale": "preserve requirement traceability"}],
    "features": [{"source_owner": "path/SPEC.md", "path": "agm/test/bdd/features/example.feature", "disposition": "retain-distinct|transfer-to-proposed-owner|represent-as-applicability", "target_path": "path/SPEC.md", "target_state": "existing|planned", "rationale": "preserve BDD traceability"}],
    "applicability_basis": "active-members|non-harness-domain",
    "applicability_rationale": "exact copy of finding basis rationale",
    "applicability": []
  },
  "boundary": "required for keep-separate"
}
```

Candidate ranks are unique and contiguous from one. `non_candidates` omit rank,
use `keep-separate`, and name the boundary. Histograms count both candidates
and non-candidates; `candidate_count` counts only `candidates`.

Every finding uses the closed `decision_status`
`pending-maintainer-approval`. Positive findings additionally carry an
`ownership_plan` with the same applicability basis, rationale, and matrix. The
plan maps every current-owner requirement by its full contract evidence and
every reciprocal BDD link by `(source_owner, path)` exactly once. An existing
target resolves its requirement ID or reciprocal BDD link at the pinned
revision; a planned target may intentionally lack that link. The plan preserves
review traceability and is explicitly not authorization to delete or mutate a
source owner.

## Enumerations

- verdict: `merge-now`, `extract-neutral-contract`, `keep-separate`,
  `resolve-product-divergence`, `insufficient-evidence`
- relationship: `same-observable`, `overlapping-observables`,
  `contradictory-observables`, `same-vocabulary-only`,
  `fixture-or-generated-copy`
- classification: `shared-contract`, `capability-variation`,
  `wrapper`, `fixture`, `implementation-detail`
- confidence: `confirmed`, `likely`, `tentative`
- strength: `strong`, `moderate`, `exploratory`
- applicability basis: `active-members`, `non-harness-domain`
- disposition: `supported`, `adapted`, `unsupported`, `not-applicable`, `unknown`

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
  completeness rationale, a defensible proposed owner, material differences,
  applicability, BDD consequences, risk, and maintainer decision.
- Require every selected BDD feature to exist and reciprocally name at least one
  current owner; require every current owner to be represented by a selected
  feature. A finding without selected BDD uses non-positive consequence `none`.
- Require active-member findings to cover each pinned member with evidence.
  Do not hide a harness-owned contract behind `non-harness-domain`.
- Require `merge-now` to use `same-observable` and
  `resolve-product-divergence` to use `contradictory-observables`. Fixtures and
  generated copies cannot become normative owners.

## HTML contract

Render one bounded, self-contained offline file. Escape all untrusted text and
retain the snapshot, trust limitations, metrics, exclusions, ranked findings,
owner topology, applicability, exact evidence, BDD consequences,
recommendations, decisions, keep-separate boundaries, lead and diagnostic
summary, methodology including every Git trust-input identity, and reproduction
commands.
Load no network font, style, script, or data.
