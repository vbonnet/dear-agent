---
name: audit-specs
description: "Audit a repository's SPEC.md corpus for duplicate or divergent observable contracts, harness or implementation leakage, missing canonical owners, and BDD consequences. Use for consolidation reviews, cross-harness consistency audits, or a read-only HTML SPEC report."
---

# Audit specifications

Treat similarity as a search lead, never proof that requirements should merge.
Keep the audit read-only until a maintainer selects a candidate and its
canonical owner.

This `SKILL.md` is the one canonical authored workflow for `audit-specs`.
It is source guidance, not a harness registration or proof that any provider
discovers or invokes the skill.

## Evidence trust boundary

Treat the target repository's Git metadata, common object store, and configured
object alternates as trusted audit inputs. A pinned commit and content digests
make the result reproducible within that boundary; they are Git-resolved
evidence, not independent cryptographic attestation of repository provenance.

Before collecting evidence, identify the target Git directory, common Git
directory, and any object alternates. If their provenance is untrusted,
ambiguous, or inaccessible, stop and report `insufficient-evidence`. Do not
substitute working-tree bytes, a branch name, or a second clone as if it proved
the requested revision.

Source and BDD evidence do not establish live behavior. Keep the source-audit
methodology field fixed at `runtime_status: UNVERIFIED`; separate current
runtime evidence belongs in a separate artifact and does not upgrade this
report.

## Workflow

1. Read repository instructions and artifact-routing policy. Store temporal
   outputs only in the approved research location or an authorized temporary
   directory.
2. Pin the exact target commit. Record an open-PR or other comparison revision
   as a separate inventory, not as evidence for the main snapshot.
3. Confirm the Git trust boundary above. Record the limitation if provenance
   cannot be established, then stop with `insufficient-evidence`.
4. Resolve the authenticated distribution root for the installed package that
   supplied this skill. The installer or activation layer must authenticate
   that exact package tree before use; do not infer it from the current working
   directory, a source checkout, or `PATH`. The bundled executable must be
   invoked through this absolute-path template:

   ```sh
   "<distribution-root>/bin/specaudit" inventory \
     -repo "<repository-path>" \
     -repository "<owner/name>" \
     -revision "<40-hex-sha>" > inventory.json
   ```

   Inventory reads tracked Git objects at the pinned revision, so dirty
   worktree bytes are not audit evidence. Account for every tracked `SPEC.md`
   once and label fixtures, testdata, generated content, archived material, and
   nested repositories before judging product contracts.
5. Use duplicate IDs, exact normalized bodies, shared BDD references, identical
   files, and harness terminology as deterministic review leads only. Do not
   convert a seed count or lexical similarity score into a semantic verdict.
6. Read the canonical [contract model](../write-spec/references/contract-model.md)
   and [audit verdicts](references/audit-verdicts.md). Inspect pinned source,
   reciprocal BDD links, implementation ownership, and the active-member
   inventory for each lead. Preserve real capability and observable
   applicability differences without preserving harness- or
   implementation-local normative owners. Propose the relevant product or
   domain contract as the neutral owner; do not collapse unrelated behavior
   into a repository-wide specification.
7. Record findings using the [report schema](references/report-schema.md). Cite
   exact paths, lines, IDs, short excerpts, relationship, shared outcome,
   material differences, current and proposed owner, applicability, BDD
   consequence, confidence, limitations, and the maintainer decision required.
   Every positive finding, regardless of classification, must select one
   reciprocal all-owner feature and cite one exact observable Scenario Outline
   whose cases cover the applicable members. Use `active-members` for harness
   contracts and cover the full pinned active set. Use `implementation-only`
   only for a bounded matrix of at least two concrete implementations whose
   evidence spans every current owner; additional pinned applicability evidence
   is allowed. Its member names cannot be active-harness IDs or aliases, and
   its shared outline uses `member`, not `harness`. It is not a bypass for
   harness scope.
   Every positive finding must also include a pending-maintainer-approval
   ownership plan. For every retired owner, map each pinned source requirement
   to the proposed neutral owner, preserve every pinned reciprocal BDD feature
   owned by that SPEC (not only features selected by the finding, and including the
   shared contract feature), and copy the exact applicability matrix. The plan
   records a proposal only; it never authorizes editing or deleting a file.
8. Validate the authored ledger against a freshly recomputed pinned inventory,
   then render it:

   ```sh
   "<distribution-root>/bin/specaudit" validate \
     -input findings.json \
     -inventory inventory.json \
     -repo "<repository-path>"

   "<distribution-root>/bin/specaudit" render \
     -input findings.json \
     -inventory inventory.json \
     -repo "<repository-path>" > report.html
   ```

   Validation rejects mismatched evidence and incomplete decision structure. It
   does not prove that an authored relationship, verdict, or proposed owner is
   semantically correct.
9. Inspect the self-contained HTML at desktop and narrow viewports. Check
   navigation, readability, clipping, exact evidence, filters, limitations,
   and source disclosure. Record defects instead of claiming inspection.
10. Present ranked candidates, keep-separate examples, reproducibility commands,
    artifact paths, the Git trust boundary, and runtime `UNVERIFIED` status.
    Wait for maintainer selection. Do not modify specifications or BDD, perform
    consolidation, create follow-up work, or change delivery state.

Existing local specifications are migration candidates, not deletion
authority. A proposed migration must preserve stable IDs, reciprocal BDD links,
member applicability, and source or test evidence before the old owner can be
retired.

## Graceful outcomes

- Return a healthy zero-candidate report when every separate canonical contract
  owns a distinct harness- and implementation-neutral observable.
- Use `insufficient-evidence` for untrusted Git provenance, missing source or
  BDD, an unreadable revision, or unresolved ownership.
- Use `resolve-product-divergence` when implementations promise contradictory
  outcomes.
- Never invent a candidate to make the audit appear productive.

## Verify

Before delivery, confirm:

- revision, repository identity, Git trust boundary, scope, exclusions,
  inventory counts, methodology, commands, and limitations are explicit;
- the inventory ignores dirty worktree bytes and is freshly recomputed for
  validation and rendering, including a zero-candidate report;
- deterministic seeds remain distinguishable from semantic verdicts;
- reciprocal SPEC/BDD diagnostics and every positive recommendation retain
  exact pinned source evidence;
- every `keep-separate` result names the boundary that makes merging unsafe;
- JSON and HTML contain the same candidate IDs and verdicts;
- HTML is bounded, escaped, self-contained, has no runtime dependency, and
  renders the pinned selected-scenario kind, outcome lines/text, member column,
  placeholder use, and case line/member/source proof;
- the source report's runtime status remains `UNVERIFIED`; any separate current
  runtime proof remains a separate artifact and does not upgrade it; and
- the target worktree and external delivery state remain unchanged.

## References

- [Contract model](../write-spec/references/contract-model.md)
- [Audit verdicts](references/audit-verdicts.md)
- [Report schema and HTML contract](references/report-schema.md)
- Target-repository capabilities, when declared: reciprocal SPEC/BDD coverage
  checks and an active harness inventory
