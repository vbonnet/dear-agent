# Report schema and offline HTML contract

The structured artifact uses `spec-audit/v3`. It is the source for the HTML
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

Resolve the authenticated distribution root for the installed package that
supplied the skill. The bundled executable must be invoked through the
absolute-path template below; do not use the current working directory, a
source checkout, or `PATH` to locate it:

```sh
"<distribution-root>/bin/specaudit" inventory \
  -repo "<repository-path>" \
  -repository "<owner/name>" \
  -revision "<40-hex-sha>" > inventory.json

"<distribution-root>/bin/specaudit" validate \
  -input findings.json \
  -inventory inventory.json \
  -repo "<repository-path>"

"<distribution-root>/bin/specaudit" render \
  -input findings.json \
  -inventory inventory.json \
  -repo "<repository-path>" > report.html
```

`-repository` is the stable repository label; it prevents clone and worktree
directory names from changing deterministic output. Commands emit inventory or
HTML bytes to standard output. The caller's authorized redirection selects the
destination. Generated reproduction commands POSIX-quote the repository label
as one argument and reject an empty label, surrounding whitespace, invalid
UTF-8, or any non-printable rune. Reports use `specaudit` as the stable logical
command identity; within an installed workflow, bind that identity to the same
authenticated `<distribution-root>/bin/specaudit` executable. Never resolve it
through `PATH`, and do not record a host-specific distribution root in
deterministic report content.

## Top-level object

```json
{
  "schema_version": "spec-audit/v3",
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
    "active_members": ["..."],
    "adapter_scopes": [{
      "id": "codex-cli",
      "kind": "harness",
      "lifecycle": "active",
      "names": [".codex", "codex", "codex-cli"],
      "evidence": [{"path": "agm/internal/agent/harnesses.go", "line": 7, "excerpt": "exact pinned source line"}]
    }]
  },
  "summary": {
    "spec_files": 0,
    "requirements": 0,
    "diagnostics": 0,
    "candidate_count": 0,
    "by_verdict": {}
  },
  "methodology": {
    "collector": "specaudit inventory",
    "seed_kinds": [
      "exact-body",
      "duplicate-id",
      "shared-bdd",
      "identical-file",
      "harness-terminology"
    ],
    "semantic_review": "bounded description",
    "runtime_status": "UNVERIFIED",
    "git_evidence_trust": "fixed trust-boundary disclosure",
    "git_trust_inputs": {
      "executable": "sha256:canonical-executable-content-digest",
      "worktree_root": "path-sha256:git-resolved-worktree-root-identity",
      "git_dir": "path-sha256:canonical-git-directory-identity",
      "common_dir": "path-sha256:canonical-common-directory-identity",
      "object_dir": "path-sha256:canonical-object-directory-identity",
      "alternate_object_dirs": ["path-sha256:canonical-alternate-directory-identity"]
    },
    "reproduce": ["exact read-only command templates"]
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

`adapter_scopes` is a deterministic path-classification catalog derived from
narrowly per-source-bounded, pinned Go AST projections of the central or
in-package active and deprecated harness registry, `NormalizeHarnessName`, the
authoritative
`configdirparity.SurfaceForHarness` repo-local directory mapping, the
`marketplaceparity.ClaudeCatalogPath` native mirror, and `OpenAIAdapter.Name`.
Every non-identifier catalog name retains its exact pinned source citation.
`NeutralCatalogPath` identifies `.dear-agent` as the harness-neutral catalog
and is deliberately not added to an adapter scope. Every selected catalog Go
blob passes its 256 KiB per-source ceiling before body loading and is parsed without Go
object resolution. Inventory checks its context before and after each
synchronous bounded projection; the ceiling makes the interval finite, but the
collector does not claim strict mid-parse preemption. Missing required
normalization-alias, configuration-directory, or marketplace metadata, or
source metadata with an unsupported case, return, or
declaration shape, fails closed with an exact catalog limitation instead of
emitting a partial scope. Any source-catalog limitation forbids positive
recommendations, so `.claude-plugin` cannot become an admissible canonical
owner merely because its pinned derivation source is absent. The registry AST
is scanned completely: `activeHarnesses` and `deprecatedHarnesses` must each
occur exactly once as a supported package `[]string` declaration. A duplicate,
other lexical binding (including a named parameter or range binding), direct or
recursive assignment target, increment/decrement target, malformed occurrence,
or missing target produces the exact
incomplete-catalog limitation with empty adapter and active-member scopes. The
catalog rejects exact
config roots and normalized catalog segments with a finite adapter-package
suffix set. It never treats an arbitrary `internal`, `pkg`, or
harness-registry parent directory as implementation-specific merely because
of its location.

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
bounded Gherkin scenario proof, and diagnostics:

```json
{
  "path": "agm/test/bdd/features/example.feature",
  "sha256": "64-hex content digest",
  "related_specs": ["relative/SPEC.md"],
  "scenarios": [{
    "line": 9,
    "name": "Active members preserve the shared outcome",
    "kind": "scenario-outline",
    "outcomes": [{"line": 12, "text": "the shared outcome is visible"}],
    "member_column": "harness",
    "uses_member_placeholder": true,
    "member_cases": [
      {"line": 16, "member": "codex-cli", "source": "examples-harness"},
      {"line": 17, "member": "pi-cli", "source": "examples-harness"}
    ]
  }],
  "diagnostics": [{
    "line": 1,
    "kind": "missing-feature-spec-reference|malformed-feature-spec-reference|ambiguous-feature-spec-reference|missing-feature-spec|nonreciprocal-feature-spec|malformed-gherkin-structure|malformed-gherkin-member-cases|gherkin-structure-limit-exceeded",
    "excerpt": "exact source or referenced path"
  }]
}
```

The diagnostic total includes SPEC-side and feature-side records. Reciprocity
is computed only from the pinned Git objects; dirty worktree bytes are not
evidence.

Scenario structure is projected with the repository's Cucumber Gherkin parser,
not a replacement grammar. Each selected feature is limited to 256 KiB before
object reading. A context-aware wrapper around the official scanner, dialect
matcher, and AST builder then enforces a 16 KiB line ceiling and, before each
bounded token is delegated to the AST builder, language-aware ceilings of
4,096 structural tokens, 256 scenarios, 256 steps, 128 observable outcomes,
32 Examples blocks, 128 Examples cases, 256 table rows, and 64 cells per row.
Before delegation it also bounds every AST-retained nonstructural surface:
2,048 tag items and 128 KiB of tag text; 4,096 comments and 192 KiB of comment
text; 4,096 description lines and 192 KiB of description text; and 256
DocStrings, 512 separators, 4,096 content lines, and 192 KiB each for delimiter
metadata and joined content. Official token types distinguish DocString content
from descriptions and table rows and support localized Gherkin keywords. The
parser stops at its first syntax error; after the first resource, cancellation,
or AST-builder failure the wrapper stops delegating into the AST while ordinary
syntax errors remain diagnostics. The parsed AST is then defensively checked
for scenario, step, Examples, case, row, cell, outcome, and 512-byte
scenario-name ceilings and independently recounts retained tags, comments,
descriptions, and DocString nodes, delimiters, and content. It does not claim to
reconstruct the discarded closing-separator token stream from the AST.
Exceeding a resource bound aborts inventory with no artifact; structures are
never truncated into a partial inventory. The adapter catalog retains at most
64 scopes, 32 names, and 16 citations per scope.

JSON inputs are limited to 32 MiB and must be valid UTF-8. Before typed decoding,
a schema-aware streaming walk enforces exact-case unique keys, one value with no
trailing content, 128 nesting levels, 1,000,000 tokens, 250,000 aggregate
elements, 16 MiB of aggregate decoded string content, 64 KiB per string, and an
explicit local ceiling for every collection field. Both report-level and
finding-level `limitations` contain at most 1,024 entries of at most 16 KiB
each. Generated reports pass the same field, token, element, string, and UTF-8
contract before encoding; invalid bytes are rejected rather than normalized to
a replacement character.

A SPEC may use multiple supported traceability H2 sections across these
spellings: `BDD Traceability`, `Test Traceability`, `Package Test Traceability`,
or `Traceability`, provided that at most one is feature-bearing. A valid or
malformed `.feature` claim makes its section feature-bearing; unit-only
supported sections do not. The first feature-bearing section is the sole
source of links. Every later feature-bearing supported H2, including a repeated
or differently spelled heading, is diagnosed as ambiguous, its claims are
still checked for malformed syntax, and none of its links are combined with
the first section. A supported entry is a full-line
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
    "shared_contract_scenario": {
      "line": 9,
      "name": "Active members preserve the shared outcome"
    },
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
- adapter-scope kind: `harness`, `compatibility-adapter`
- adapter-scope lifecycle: `active`, `deprecated`, `compatibility`
- feature scenario kind: `scenario`, `scenario-outline`

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
- Require inventory and semantic methodology to carry matching fixed
  `runtime_status: UNVERIFIED`; source-only validation never proves runtime
  parity.
- Require each source-evidence path, line, ID, and excerpt to equal the pinned
  inventory and require the current-owner set to match the evidence paths.
- Require every semantic path to use its canonical repository-relative spelling;
  aliases such as `./one/SPEC.md`, cleaned traversal, repeated separators, and
  backslash spellings are invalid rather than distinct proposed owners.
- Require positive findings to name every current owner, a bounded
  completeness rationale, a defensible harness-neutral proposed owner,
  material differences, applicability, BDD consequences, risk, and maintainer
  decision. A new owner cannot be nested under a current-owner directory, and
  a proposed owner cannot match the pinned adapter-scope catalog. `merge-now`
  requires at least two current owners and selects an existing proposed owner.
  `extract-neutral-contract` accepts one or more current owners, but a
  single-owner extraction requires a new proposed owner and retires that owner.
- Reject every positive recommendation while an active-harness,
  deprecated-harness, or bounded adapter-catalog source limitation is present.
- Require every positive finding to carry a `pending-maintainer-approval`
  ownership plan with exact entries for all current owners. Retirements map
  every requirement in the retired SPEC's pinned inventory by exact path,
  line, identifier, and excerpt, plus every pinned reciprocal BDD feature owned
  by that retiring SPEC whether or not the finding selected it, always including
  the positive finding's shared-contract feature, and the exact
  applicability matrix, to the proposed owner. Finding evidence is
  not a substitute for whole-file retirement coverage. Existing targets must
  already exist in the pinned proposed owner; planned targets are allowed only
  for a new owner. The plan is not change or deletion authority.
- Require every selected BDD feature to exist and reciprocally name at least one
  current owner; require every current owner to be represented by a selected
  feature. Every positive finding regardless of classification identifies a
  selected `shared_contract_feature` that reciprocally names every current
  owner and an exact scenario line/name. That scenario is an observable Scenario Outline,
  uses its `harness` or `member` placeholder, and has unique examples cases
  exactly covering at least two applicability members whose dispositions are
  not `not-applicable`. If that outline has a harness/member Examples table,
  every additional Examples table must also carry a harness/member column;
  mixed member and non-member tables are a structural diagnostic. A finding
  without selected BDD uses non-positive consequence `none`.
- Require every positive finding to use one bounded applicability basis.
  `active-members` covers each pinned active member with evidence.
  `implementation-only` covers at least two named implementations and cites
  every current owner across that matrix; it may carry additional pinned
  applicability evidence. It cannot name an active-harness ID or alias, and
  its selected shared outline uses `member`, not `harness`. A classification
  label cannot weaken either requirement or turn a harness contract into an
  implementation-only exception.
- Require `merge-now` to use `same-observable` and
  `extract-neutral-contract` to use `same-observable` or
  `overlapping-observables`; require
  `resolve-product-divergence` to use `contradictory-observables`. Fixtures and
  generated copies cannot become normative owners.

## HTML contract

Render one bounded, self-contained offline file. Escape all untrusted text and
retain the snapshot, trust limitations, metrics, exclusions, ranked findings,
owner topology, neutrality rationale, pending ownership/preservation plan,
adapter-scope catalog and citations, applicability, exact evidence, BDD
scenario proof including the pinned kind, every outcome line/text, member
column, placeholder-use flag, and every case line/member/source, and consequences,
recommendations, decisions, keep-separate boundaries, lead and diagnostic
summary, methodology including every Git trust-input identity, and reproduction
commands. Render the fixed `UNVERIFIED` runtime status prominently. Render every
requirement-preservation source path, line, identifier, and excerpt with HTML
escaping, including preservation records not repeated in finding-level
evidence. Resolve the selected-scenario detail from the freshly recomputed
inventory by exact feature path, line, and name; the semantic ledger does not
carry or override that structural proof.
Load no network font, style, script, or data.
