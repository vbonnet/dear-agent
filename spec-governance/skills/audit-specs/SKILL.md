---
name: audit-specs
description: "Audit a repository's SPEC.md corpus for duplicate or divergent observable contracts, harness or implementation leakage, missing canonical owners, and BDD consequences. Use for consolidation reviews, cross-harness consistency audits, or a read-only HTML SPEC report."
---

# Audit specifications

Treat similarity as a search lead, never as proof that requirements should
merge. This workflow is read-only until a maintainer selects a candidate and
its canonical owner.

## Workflow

1. Read repository instructions and artifact-routing policy. Put temporal
   outputs in the approved research location or the operating-system temporary
   directory, never in a product path that forbids audit artifacts.
2. Pin the exact Git commit and record any open-PR comparison as a separate
   inventory, not as main-snapshot proof. Inventory tracked files from Git
   objects so a dirty checkout cannot silently alter the corpus.
3. Run the deterministic collector with
   `go run ./spec-governance/skills/audit-specs/scripts/specaudit inventory -repo <repository-path> -repository <owner/name> -revision <40-hex-sha> > inventory.json`.
   The tool emits bytes only to standard output so the shell's normal path and
   write guards remain authoritative for artifact storage.
   Account for every tracked `SPEC.md` once and
   label fixtures, testdata, migrations, generated content, archived material,
   and nested repositories before judging product contracts.
4. Use exact normalized bodies, repeated IDs, shared BDD references, explicit
   owner links, and harness terminology to seed review. Do not treat duplicate
   IDs or lexical similarity as semantic equivalence.
5. Read the canonical [contract model](../write-spec/references/contract-model.md)
   and [audit verdicts](references/audit-verdicts.md). Inspect source, linked
   BDD, implementation ownership, and the active-member registry for each
   candidate. Preserve native adapter boundaries and unsupported capabilities.
6. Record findings in the [report schema](references/report-schema.md). Cite
   exact paths, lines, IDs, short excerpts, relation, shared outcomes, material
   differences, current and proposed owner, applicability, BDD consequence,
   confidence, limitations, and the maintainer decision required.
7. Validate and render the structured findings with
   `go run ./spec-governance/skills/audit-specs/scripts/specaudit validate -input findings.json -inventory inventory.json -repo <repository-path>`
   and
   `go run ./spec-governance/skills/audit-specs/scripts/specaudit render -input findings.json -inventory inventory.json -repo <repository-path> > report.html`.
   The HTML must
   be self-contained and work offline. Verify the structured artifact first;
   rendering must not invent, discard, or reclassify findings.
8. Inspect the HTML at desktop and narrow viewports. Check navigation,
   readability, clipping, exact evidence, filters, diagrams, limitations, and
   source disclosure. Record visual defects rather than claiming inspection.
9. Present the ranked candidates, top recommendation, keep-separate examples,
   reproducibility command, and artifact paths. Wait for maintainer selection;
   do not modify specifications, BDD, catalogs, wrappers, issues, or PR state.

## Graceful outcomes

- Return a healthy zero-candidate report when every local contract is a
  legitimate delta.
- Use `insufficient-evidence` when a file, linked test, revision, or owner
  cannot be read.
- Use `resolve-product-divergence` when implementations promise contradictory
  outcomes.
- Never invent a merge candidate to make the audit appear productive.

## Verify

Before delivery, confirm:

- revision, scope, exclusions, inventory counts, methodology, commands, and
  limitations are explicit;
- the supplied inventory was freshly recomputed from Git objects by both
  validation and rendering, including for a zero-candidate report;
- deterministic seeds are distinguishable from semantic verdicts;
- every positive recommendation has exact source and BDD evidence;
- every `keep-separate` result names the boundary that makes merging unsafe;
- JSON and HTML contain the same candidate IDs and verdicts;
- HTML has no external runtime dependency; and
- the product worktree and external delivery state remain unchanged.

## References

- [Contract model](../write-spec/references/contract-model.md)
- [Audit verdicts](references/audit-verdicts.md)
- [Report schema and HTML contract](references/report-schema.md)
- Repository `internal/speccoverage` and active harness registry
