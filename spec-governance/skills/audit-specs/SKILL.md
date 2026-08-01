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

Source and BDD evidence do not establish live behavior. Mark runtime consistency
`UNVERIFIED` unless separate, current runtime evidence directly proves it.

## Workflow

1. Read repository instructions and artifact-routing policy. Store temporal
   outputs only in the approved research location or an authorized temporary
   directory.
2. Pin the exact target commit. Record an open-PR or other comparison revision
   as a separate inventory, not as evidence for the main snapshot.
3. Confirm the Git trust boundary above. Record the limitation if provenance
   cannot be established, then stop with `insufficient-evidence`.
4. From the dear-agent checkout that owns this skill and the root command, run:

   ```sh
   go run ./tools/specaudit inventory \
     -repo <repository-path> \
     -repository <owner/name> \
     -revision <40-hex-sha> > inventory.json
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
   Every positive finding must include a pending-maintainer-approval ownership
   plan. An existing proposed owner must already be a current owner and is the
   only owner that may retain the selected contract. A new owner must be outside
   every current owner's directory by path components; do not hide a replacement
   in the same directory or a descendant. Registration-local current owners are
   factual audit results, but must retire rather than remain canonical.
   Use full retirement only when every requirement and reciprocal feature owned
   by that source is selected. Otherwise retire only the selected records and
   preserve each unrelated record exactly in place pending a separate audit.
   A reciprocal feature that still traces residual source behavior, or proves
   only co-location/topology rather than the selected observable, also remains
   in place under selected retirement. Use BDD consequence `preserve-residual`
   when the feature directly covers both the selected and residual behavior;
   use `add-matrix` and name the missing behavioral scenario or feature when it
   is only topology evidence. Do not transfer the whole traceability link.
   Reconcile overlapping findings before validation: the same exact selected
   requirement may not receive incompatible dispositions or targets in two
   candidates. If one requirement bundles observables that belong to different
   owners, record a split-required or insufficient-evidence decision rather
   than transferring the indivisible source line twice.
   Copy the exact applicability matrix and planned BDD transfers into the plan.
   The plan records a proposal only; it never authorizes editing or deleting a
   file.
8. Require each current owner in a positive finding to have exactly one
   traceability state: an existing reciprocal selected feature, or one planned
   transfer to the existing proposed owner through a selected feature that is
   already reciprocal with that owner. Cite exact behavioral rows from the
   target feature for each planned transfer. A matching path graph, feature
   title, scenario title, or tag alone is topology, not evidence of equivalent
   observable behavior; validation resolves citations but cannot make that
   semantic judgment.
9. Validate the authored ledger against a freshly recomputed pinned inventory,
   then render it:

   ```sh
   go run ./tools/specaudit validate \
     -input findings.json \
     -inventory inventory.json \
     -repo <repository-path>

   go run ./tools/specaudit render \
     -input findings.json \
     -inventory inventory.json \
     -repo <repository-path> > report.html
   ```

   Validation rejects mismatched evidence and incomplete decision structure. It
   does not prove that an authored relationship, verdict, or proposed owner is
   semantically correct.
10. Inspect the self-contained HTML at desktop and narrow viewports. Check
   navigation, readability, clipping, exact evidence, filters, limitations,
   and source disclosure. Record defects instead of claiming inspection.
11. Present ranked candidates, keep-separate examples, reproducibility commands,
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
- every positive owner has exactly one current or planned BDD traceability state,
  and every `PLANNED` entry is visibly pending and cites exact behavioral text;
- full and selected retirement preserve all selected and residual records under
  the closed dispositions described by the report schema;
- topology-only or residual-source BDD links remain with their source while the
  report names the missing canonical behavioral coverage;
- no exact selected requirement receives incompatible cross-finding ownership
  mappings, and every mixed requirement is left pending an explicit split;
- every `keep-separate` result names the boundary that makes merging unsafe;
- JSON and HTML contain the same candidate IDs and verdicts;
- HTML is bounded, escaped, self-contained, and has no runtime dependency;
- runtime parity is `UNVERIFIED` unless separately proven; and
- the target worktree and external delivery state remain unchanged.

## References

- [Contract model](../write-spec/references/contract-model.md)
- [Audit verdicts](references/audit-verdicts.md)
- [Report schema and HTML contract](references/report-schema.md)
- Repository `internal/speccoverage` and active harness inventory
