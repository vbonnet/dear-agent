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

**SPECAUDIT-03** When inventory succeeds, the command shall emit a `spec-audit/v2` immutable inventory document containing complete pinned facts and no reviewer decision or exclusion; when validation or rendering receives a decision ledger, the command shall require its `inventory_ref` to equal the canonical SHA-256 digest of the separately supplied inventory, recompute that complete inventory from the named Git objects at the pinned revision, and reject any supplied inventory, decision ledger, finding set, or zero-finding result that does not match. A decision ledger shall omit `inventory`, `features`, `seeds`, `collector_execution`, and its disclosure; reviewer exclusions shall remain visible as typed classification, rationale, and pinned supporting evidence, shall not suppress collection, and shall not authorize deletion or a positive current-owner selection.

**SPECAUDIT-04** When report validation checks a finding, the command shall require the factual current-owner set, existing proposed owner, and each applicability row to be established by exact pinned `normative-contract` requirement evidence, and shall allow `supporting` evidence only as an additional citation from a deduplicated, bounded batch of tracked regular blobs at the pinned revision. A positive finding may report a harness-registration `SPEC.md` as a current owner that must retire, but shall reject a harness-registration proposed owner, an existing proposed owner absent from the current-owner set, or a new proposed owner's directory equal to or component-descendant of any current owner's directory; a root-directory current owner shall block every nested new owner, while sibling directories and non-component string prefixes shall remain eligible. Validation shall also reject incomplete ownership rationale, neutrality rationale, ranking or applicability, or any owner, proposed owner, or applicability claim established by supporting evidence alone.

**SPECAUDIT-05** When report validation checks BDD impact for a positive finding, the command shall require every selected feature to exist, remain reviewer-included, and reciprocally reference at least one current owner at the pinned revision, and shall require every current owner to have exactly one of current reciprocal coverage or one typed planned transfer. Each planned transfer shall identify one uncovered current owner, the existing proposed owner, and a selected feature already reciprocal with that proposed owner, shall carry exact target-feature behavior evidence resolved by the bounded supporting-evidence pipeline, and shall not be treated as a current link or deterministic semantic proof; when no feature is selected, validation shall require a non-positive finding with BDD consequence `none`.

**SPECAUDIT-06** When rendering a validated audit artifact, the command shall emit self-contained offline HTML that safely represents evidence and retains every decision field, owner topology, applicability row, BDD path, exclusion, methodology fact including every Git trust-input identity, and limitation, including each applicability citation's source, line, requirement identifier, and excerpt, and shall render every planned BDD transfer explicitly as `PLANNED` with its source, target, selected feature, rationale, and exact behavior evidence.

**SPECAUDIT-07** When inventory or rendering succeeds, the command shall emit exactly one complete artifact to standard output, and when validation, collection, or rendering fails, it shall emit no artifact bytes and shall not create, replace, or delete an artifact destination.

**SPECAUDIT-08** When an audit names a comparison revision, the artifact shall label it as comparison-only unless that revision has its own complete pinned inventory.

**SPECAUDIT-09** When the collector resolves pinned Git evidence, the artifact shall record privacy-preserving concrete identities for the canonical caller-approved Git executable, Git-resolved worktree root, repository Git directory, common directory, object directory, and every configured alternate object directory, together with the caller-supplied repository label and exact revision; validation shall recompute those inputs rather than treat them as independently authenticated source truth.

**SPECAUDIT-10** When the collector resolves pinned Git evidence, only the caller-approved Git toolchain and target object-store routing shall influence object selection, and network access, lazy fetching, replacement objects, prompts, the working directory, and unapproved inherited repository or configuration settings shall not supply or redirect evidence.

**SPECAUDIT-11** When inventory emits candidate seeds, the artifact shall derive them deterministically from equal normalized requirement bodies, repeated requirement identifiers, shared BDD references, identical full `SPEC.md` bodies, and harness terminology and shall not contain a semantic relationship, disposition, or canonical-owner verdict.

**SPECAUDIT-12** When validation or rendering reads JSON input, the command shall accept only one stable, bounded JSON value with unique object keys, exact case-sensitive declared field names, and no trailing content and shall reject input that changes during the read or exceeds its declared ceiling.

**SPECAUDIT-13** When inventory JSON or rendered HTML would exceed its declared output ceiling, the command shall reject the result before writing any artifact bytes; inventory JSON encoding shall enforce that ceiling while escaping untrusted values and shall not first materialize an unbounded escaped document.

**SPECAUDIT-14** When collection cannot account for the complete pinned corpus within its declared file-count, aggregate-byte, per-file-byte, output-byte, and wall-time ceilings, the command shall fail with no inventory artifact rather than return a partial result.

**SPECAUDIT-15** When report validation or rendering succeeds, the artifact shall distinguish pinned-object evidence and deterministic structural validation from reviewer-authored semantic recommendations and shall not attest that a semantic relationship, disposition, or canonical-owner recommendation is correct without maintainer review.

**SPECAUDIT-16** When inventory, validation, or rendering runs, the target repository, working tree, Git index and references, specifications, BDD features, issue state, and delivery state shall remain unchanged, and the command shall not apply a consolidation recommendation.

**SPECAUDIT-17** When inventory runs, the inventory document shall disclose `collector_execution` and a fixed non-attesting disclosure in JSON and HTML. It shall state build-information and VCS-metadata availability explicitly, include a module path only when build information reports one, include a 40-hex VCS revision and boolean modified state only when both VCS settings are present and valid, and shall not invent a fallback module path or claim it independently authenticates the collector source or build.

**SPECAUDIT-18** When validation or rendering reads report inputs, the command shall authenticate each input by no-follow descriptor traversal on Darwin and Linux and shall fail before reading report content or emitting artifact bytes on every other GOOS.

**SPECAUDIT-19** When the pinned dear-agent harness registry path is available, inventory shall extract active members only from a package-level Go AST declaration of the canonical `agm/internal/harnessregistry/registry.go` path or the documented legacy `agm/internal/agent/harnesses.go` adapter; when neither is available or parseable it shall record an empty active-member set and a limitation, and validation shall reject an active-member parity claim. It shall not accept a caller-supplied regular expression, comment, inert literal, or unpinned member list.

**SPECAUDIT-20** When the collector or validator resolves Git paths, the command shall reject control-character, backslash, non-canonical, and non-UTF-8 relative paths, force literal Git pathspec handling, and accept only regular blobs for collected or supporting evidence.

**SPECAUDIT-21** When validation checks positive findings, the command shall require each `decision_status` to be `pending-maintainer-approval` and a non-deletion ownership plan that exactly copies the finding's planned BDD transfers and preserves every current-owner requirement, reciprocal BDD link, and applicability record exactly once. Each plan shall assign exactly one closed owner action with rationale to each current owner: only an existing proposed owner may `retain-distinct-contract`; `retire-normative-ownership` shall require every source requirement and reciprocal feature to be selected and then transferred or represented; and `retire-selected-normative-ownership` shall transfer or represent selected requirements while preserving every unselected record at its exact existing source under `preserve-in-place-pending-separate-audit` until a separate audit and maintainer approval. A selected reciprocal feature may also remain under `preserve-in-place-pending-separate-audit` when it still traces residual source behavior or is topology evidence rather than proof of the selected observable; validation shall require BDD consequence `preserve-residual` for the former or `add-matrix` for the latter, and an `add-matrix` finding shall state the missing behavioral target instead of transferring the whole feature. Across findings, the command shall reject incompatible dispositions or targets for the same exact pinned normative requirement.

**SPECAUDIT-22** When validation reads `collector_execution`, the command shall treat it as a historical self-report, validate its structural consistency and fixed non-attesting disclosure, and shall not require it to equal the current validator build or platform.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
