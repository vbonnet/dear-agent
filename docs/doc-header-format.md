# Doc metadata header format

<!-- Last audited at: 2026-07-27 -->

## The anti-pattern

Several docs in this repo (and many more in the sibling `engram-research`
repo) open with a metadata block written as inline bold `key: value` pairs
crammed onto one physical line, separated only by `·`, `|`, or nothing
consistent. Two real examples from this repo, unchanged:

`REVIEW.md:3`:

```
**Status:** authoritative · **Last updated:** 2026-06-11
```

`docs/code-review-automation-setup.md:3`:

```
**Status:** authoritative · **Last audited:** 2026-07-23
```

There is no line break between `**Status:**` and `**Last updated:**` — just a
`·`. GitHub (and every other Markdown renderer) displays this as one
unbroken run of text: distinct, unrelated fields visually smear together,
which is hard to scan and easy to get wrong (a stray edit can merge a key
into the previous value with no line break to catch it, and separators and
key names drift file to file with nothing to enforce a shared schema).

## The canonical format

Write each metadata field as its own bullet, with a real line break between
fields:

```
- **Status:** authoritative
- **Last updated:** 2026-06-11
```

Applied to the two examples above:

`REVIEW.md`, before → after:

```
**Status:** authoritative · **Last updated:** 2026-06-11
```

```
- **Status:** authoritative
- **Last updated:** 2026-06-11
```

`docs/code-review-automation-setup.md`, before → after:

```
**Status:** authoritative · **Last audited:** 2026-07-23
```

```
- **Status:** authoritative
- **Last audited:** 2026-07-23
```

A single field alone (e.g. the `Status: Accepted (2026-04-24)` line ADRs use)
is already fine — there is nothing for it to run together with, so it is not
this anti-pattern and does not need to change.

For headers with several fields where a tabular layout reads better, a small
Markdown table is also acceptable:

```
| Status        | Last updated |
| ------------- | ------------ |
| authoritative | 2026-06-11   |
```

The one hard rule is: every distinct metadata field gets its own line. Never
put two `**Label:**` fields on the same physical line.

## Why a list, not YAML frontmatter

YAML frontmatter (`---\nstatus: authoritative\n---`) is the other reasonable
fix — it is machine-parseable and is what static-site generators (Jekyll,
Hugo, Docusaurus) expect. This repo does not run one: docs are read as plain
Markdown directly on GitHub, which does not render frontmatter specially for
ordinary repository files — it would show up as a literal `---` block of raw
text, which is worse than the problem it would fix. Frontmatter would also be
a much bigger departure from how every other doc in this repo currently
opens (a level-1 title, then a short human-readable preamble), for a
machine-parseability benefit nothing here currently needs.

The bullet-list fix is a minimal, surgical change: same bold-label style
authors already use, same information, the one thing that was missing (a
line break) added back. It also matches how this repo already marks
document-level metadata elsewhere — see the `<!-- Last audited at: DATE -->`
HTML-comment convention used across `SPEC.md` / `ARCHITECTURE.md` files
(e.g. `pkg/headerlint/SPEC.md`, `engram/ecphory/ARCHITECTURE.md`) for a
machine-checkable audit timestamp that does not need to be human-visible.
Bulleted `**Status:**` / `**Last updated:**` fields are for metadata a human
skimming the doc should see immediately; the HTML comment is for metadata
tooling checks but a reader does not need to see. Use whichever fits: if the
field only matters to tooling, use the HTML-comment convention instead of
adding it to the visible header block.

## Enforcement

`tools/header-lint` (backed by `pkg/headerlint`) is a deterministic Go lint
check that flags this anti-pattern: two or more `**Label:**` bold-field
markers on the same physical line, within a document's "header zone" (the
top of the file, up to the first `##` heading or the first 15 lines,
whichever comes first). It intentionally does not flag two bolded terms
appearing later in a document as part of ordinary prose (for example
"**Complexity:** Low. **Timeline:** Comparable." inside a comparison
paragraph) — that is not a metadata header, just two bolded words in a
sentence.

Run it locally:

```
make lint-headers
```

or directly:

```
go run ./tools/header-lint -repo .
```

It runs in CI via `.github/workflows/doc-header-lint.yml` on every pull
request and on push to `main`/`develop`. It starts in an informational
posture (`continue-on-error: true`, same pattern as `doc-proximity.yml`)
because it finds the two pre-existing violations quoted above at
introduction time — failing the build today would block every PR on
unrelated pre-existing debt. Once those two files (and this repo's own
future backlog, if any accumulates before the check is flipped) are fixed,
flip `continue-on-error` off and add the job to branch-protection
required-status-checks so it blocks new violations outright.

This document defines the format and ships the lint check that catches new
violations. It does not itself fix the two examples quoted above, or the
~67 files with this pattern in `engram-research` — that backfill is tracked
separately.
