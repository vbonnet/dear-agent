# Doc metadata header format

<!-- Last audited at: 2026-07-27 -->

## The anti-pattern

A document metadata block written as inline bold `key: value` pairs crammed
onto one physical line, separated only by `·`, `|`, or nothing consistent:

```
**Status:** authoritative · **Last updated:** DATE
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
- **Last updated:** DATE
```

A single field alone (e.g. the `Status: Accepted (2026-04-24)` line ADRs use)
is already fine — there is nothing for it to run together with, so it is not
this anti-pattern and does not need to change.

For headers with several fields where a tabular layout reads better, a small
Markdown table is also acceptable:

```
| Status        | Last updated |
| ------------- | ------------ |
| authoritative | DATE         |
```

The one hard rule is: every distinct metadata field gets its own line. Never
put two `**Label:**` fields on the same physical line.

## Why a list, not YAML frontmatter

YAML frontmatter (`---\nstatus: authoritative\n---`) is the other reasonable
fix — it is machine-parseable and is what static-site generators (Jekyll,
Hugo, Docusaurus) expect. GitHub recognizes YAML frontmatter in repository
Markdown and renders its fields as a metadata table, so rendering is not a
reason to reject it. The repo still prefers lists because its documents
consistently open with a level-1 title and short human-readable preamble, and
none of the current repository tooling consumes frontmatter. Introducing a
second metadata convention would be a larger migration for a
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

It runs in fast preflight and in CI via
`.github/workflows/doc-header-lint.yml` on every pull request and on push to
`main`/`develop`. Both enforcement paths fail on any tracked violation.
