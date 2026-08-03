# SPEC governance

This directory contains the canonical source workflows for two skills:

- [`skills/write-spec/SKILL.md`](skills/write-spec/SKILL.md) is the one
  authored workflow for writing or revising an observable, harness-neutral
  contract with explicit applicability and BDD consequences.
- [`skills/audit-specs/SKILL.md`](skills/audit-specs/SKILL.md) is the one
  authored workflow for a pinned, read-only ownership audit that produces a
  structured ledger and self-contained HTML.

These source files are not harness adapters or discovery registrations. A
committed skill proves source presence only; whether a harness discovers or
invokes it remains `UNVERIFIED` until separately observed in that harness.
If a repository-local harness discovery registration is added later, it must
be a contained tracked symlink to the canonical `SKILL.md`; `skill-lint`
rejects a second regular-file owner with the same skill name and rejects an
alias whose resolved target is not that tracked canonical owner.

The authoring skill is the canonical guidance for the rule that one shared
observable behavior has one harness-neutral product `SPEC.md` owner. The audit
skill supplies evidence for maintainer review; it neither selects that owner
nor edits product contracts.

The deterministic collector is the root-module command at `tools/specaudit`.
Its observable command behavior belongs only to
[`tools/specaudit/SPEC.md`](../tools/specaudit/SPEC.md), whose focused BDD
feature runs selected unit tests for the outcomes described there. The feature
does not exercise either skill in a harness. Run the collector from this
repository's root with `go run ./tools/specaudit`; pass the repository being
audited through `-repo`.
The collector inventories tracked `SPEC.md` and BDD feature objects at an exact
Git commit, emits non-verdict review leads, validates a semantic decision
ledger against that pinned inventory, and renders bounded offline HTML. It does
not edit specifications, consolidate owners, or authorize follow-on work.

## Evidence boundary

Pinned means Git-resolved, not independently attested. The target repository's
Git metadata, common object store, and configured object alternates are trusted
inputs to the audit. A commit hash and content digests make results reproducible
within that trust boundary; they do not prove that those inputs are authentic
or that runtime behavior matches the source. If that Git provenance is
untrusted, ambiguous, or inaccessible, stop and report `insufficient-evidence`.

Deterministic duplicate IDs, exact bodies, shared BDD references, identical
files, and harness terminology are leads for semantic review. None is a merge
verdict. Positive recommendations require source and reciprocal BDD evidence,
an explicit applicability analysis, a proposed owner, and maintainer review.
Runtime parity remains `UNVERIFIED` unless separate live evidence establishes
it.

## Evidence and provenance

The process adapts Matt Pocock's evidence-first
[`improve-codebase-architecture`](https://github.com/mattpocock/skills/blob/main/skills/engineering/improve-codebase-architecture/SKILL.md)
and [HTML report](https://github.com/mattpocock/skills/blob/main/skills/engineering/improve-codebase-architecture/HTML-REPORT.md)
methods, his domain-modeling and codebase-design vocabulary, Anthropic's
skill-creator evaluation loop, EARS requirements, and Cucumber's
implementation-agnostic Gherkin guidance.

Temporal research, forward evaluations, pinned inventories, findings, and
rendered reports belong in the configured research repository or an authorized
temporary directory, not in dear-agent's living product documentation.

## Commands

```sh
make lint-skills
go test ./pkg/instructionlint
go test ./tools/specaudit
go run ./tools/specaudit --help
```

See the two canonical skills for the authoring and audit workflows.
