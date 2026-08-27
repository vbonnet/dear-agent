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

For repository documentation and the behavioral `SPEC.md` starter, begin at
[`docs/spec-authoring.md`](../docs/spec-authoring.md). That page is a router;
the authored workflow remains in the canonical skill files above.

The deterministic collector's source lives at `tools/specaudit`; installed
skill workflows use the bundled `bin/specaudit` executable. Its observable
command behavior belongs only to
[`tools/specaudit/SPEC.md`](../tools/specaudit/SPEC.md), whose focused BDD
feature runs selected unit tests for the outcomes described there. The feature
does not exercise either skill in a harness. An installed workflow must first
obtain its authenticated package root from the installer or activation layer,
then invoke `"<distribution-root>/bin/specaudit"`; it must not discover the
tool through the current directory, a source checkout, or `PATH`. Pass the
repository being audited through `-repo`.
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

## Installed audit execution

After authenticating the installed distribution root:

```sh
"<distribution-root>/bin/specaudit" --help
```

The command emits the stable logical identity `specaudit` in help and report
methodology. Installed workflows bind that identity to the authenticated
absolute executable above; reports do not embed host-specific installation
paths.

## Staged distribution

The root-module [`spec-governance-package`](../cmd/spec-governance-package)
command accepts a prebuilt `specaudit` artifact and either stages one unique
private distribution or validates an existing staged root. It writes
`package-manifest.json` last, writes through retained directory handles, and
returns a deterministic manifest digest only after opened and visible identities
are reverified. Before allocation, it walks the opened staging-parent ancestry
and rejects the source root or any source descendant, including paths reached
through an intermediate symlink. If staging fails after allocation, the command leaves every
surviving tree untouched and reports the originally allocated path as diagnostic
state, including whether its original identity was still verified at return.
A separately authorized, liveness-aware lifecycle reaper may handle it later;
an unverified diagnostic path must never be removed automatically. This avoids
unlinking a concurrent pathname replacement. A retained failed root is not a
package receipt. If staging succeeds but JSON receipt delivery fails, the CLI
exits nonzero and reports the exact valid staged root on standard error rather
than orphaning it silently. A successful receipt proves structural closure at validation
time; it does not prove trusted installation, loader discovery, provider
invocation, or running-image identity. Those later layers must pin the returned
digest independently.

The stager is race-detecting, not a same-UID sandbox. On portable POSIX
filesystems, `mkdirat` does not atomically return a directory handle. The
96-bit-random private root narrows the interval before the stager opens and
captures each newly created directory identity; replacement rejection applies
from that handle capture through return. Each retained parent is rechecked
immediately before mutation, but POSIX cannot stop a hostile same-UID process
from reparenting an already-open inode after that check. Such namespace control
remains outside this package's isolation boundary; observed mutation still
causes staging to return no receipt. Source and sibling non-modification claims
apply within this documented handle-capture boundary.

## Source-development-only commands

The following commands are for contributors working from this repository's
root. They are not installed-skill instructions and do not establish package,
activation, or runtime evidence:

```sh
make lint-skills
go test ./pkg/instructionlint
go test ./tools/specaudit
go run ./tools/specaudit --help
```

See the two canonical skills for the authoring and audit workflows.
