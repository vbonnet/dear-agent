# SPEC Audit Inventory and Report Specification

**Status:** Active
**Scope:** Deterministic pinned inventory, finding validation, and offline audit
artifact production.

This specification owns the observable `specaudit` command behavior. It does
not own semantic consolidation decisions or the replaceable implementation
used to obtain those outcomes.

## EARS Requirements

**SPECAUDIT-01** When inventory receives an explicit repository identity and exact Git revision, the artifact shall account for every tracked `SPEC.md` and BDD feature in that revision's Git objects exactly once and shall order all records deterministically without consulting working-tree content.

**SPECAUDIT-02** When inventory interprets a specification or BDD feature, the artifact shall exclude fenced examples from normative counts, derive identified requirements from canonical Markdown-normalized EARS text, accept only a documented full-line traceability bullet with one backtick-delimited repository-relative `.feature` path inside one documented traceability section, and diagnose anonymous or nonconforming requirements and missing, malformed, duplicate, ambiguous, or nonreciprocal SPEC/BDD links.

**SPECAUDIT-03** When report validation or rendering receives an inventory, the command shall recompute the complete inventory from the named Git objects at the pinned revision and shall reject any supplied inventory, finding set, or zero-finding result that does not match; semantic reports shall omit `inventory`, `features`, and `seeds` and shall use only the separately supplied pinned inventory.

**SPECAUDIT-04** When report validation checks a positive finding, the command shall require the current-owner set to equal the pinned requirement-evidence owners, shall require pinned evidence for an existing proposed owner, shall reject a proposed owner on a known harness surface or a new owner nested beneath a current owner, and shall require separate ownership and harness-neutrality rationales.

**SPECAUDIT-05** When report validation checks BDD impact with one or more selected features, the command shall require every selected feature to exist at the pinned revision and to reciprocally reference at least one current owner, shall require every current owner to be reciprocally covered by at least one selected feature, and for a positive shared-contract finding shall require one selected feature that reciprocally references every current owner; when no feature is selected, it shall require a non-positive finding with BDD consequence `none`.

**SPECAUDIT-06** When rendering a validated audit artifact, the command shall emit self-contained offline HTML that safely represents evidence and retains every decision field, owner topology, proposed-owner neutrality rationale, pending ownership and preservation plan, applicability row, BDD path and shared-contract feature, exclusion, methodology fact including every Git trust-input identity, and limitation, including each applicability citation's source, line, requirement identifier, and excerpt.

**SPECAUDIT-07** When inventory or rendering succeeds, the command shall emit exactly one complete artifact to standard output, and when validation, collection, or rendering fails, it shall emit no artifact bytes and shall not create, replace, or delete an artifact destination.

**SPECAUDIT-08** When an audit names a comparison revision, the artifact shall label it as comparison-only unless that revision has its own complete pinned inventory.

**SPECAUDIT-09** When the collector resolves pinned Git evidence, the artifact shall record privacy-preserving concrete identities for the canonical caller-approved Git executable, Git-resolved worktree root, repository Git directory, common directory, object directory, and every configured alternate object directory, together with the caller-supplied repository label and exact revision; validation shall recompute those inputs rather than treat them as independently authenticated source truth.

**SPECAUDIT-10** When the collector resolves pinned Git evidence, only the caller-approved Git toolchain and target object-store routing shall influence object selection, and network access, lazy fetching, replacement objects, prompts, the working directory, and unapproved inherited repository or configuration settings shall not supply or redirect evidence.

**SPECAUDIT-11** When inventory emits candidate seeds, the artifact shall derive them deterministically from equal normalized requirement bodies, repeated requirement identifiers, shared BDD references, identical full `SPEC.md` bodies, and harness terminology and shall not contain a semantic relationship, disposition, or canonical-owner verdict.

**SPECAUDIT-12** When validation or rendering reads JSON input, the command shall accept only one stable, bounded JSON value with unique object keys and no trailing content and shall reject input that changes during the read or exceeds its declared ceiling.

**SPECAUDIT-13** When inventory JSON or rendered HTML would exceed its declared output ceiling, the command shall reject the result before writing any artifact bytes; inventory JSON encoding shall enforce that ceiling while escaping untrusted values and shall not first materialize an unbounded escaped document.

**SPECAUDIT-14** When collection cannot account for the complete pinned corpus within its declared file-count, aggregate-byte, per-file-byte, output-byte, and wall-time ceilings, the command shall fail with no inventory artifact rather than return a partial result.

**SPECAUDIT-15** When report validation or rendering succeeds, the artifact shall distinguish pinned-object evidence and deterministic structural validation from reviewer-authored semantic recommendations, shall require every positive finding to retain exactly one existing neutral owner or plan one new neutral owner while mapping retired requirements, reciprocal BDD, and applicability, and shall accept only `pending-maintainer-approval` so the report cannot authorize consolidation or deletion.

**SPECAUDIT-16** When inventory, validation, or rendering runs, the target repository, working tree, Git index and references, specifications, BDD features, issue state, and delivery state shall remain unchanged, and the command shall not apply a consolidation recommendation.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
