# Header Lint Package Specification

<!-- Last audited at: 2026-07-27 -->

## Overview

`pkg/headerlint` detects the single-line bold metadata "header block"
anti-pattern in Markdown documents: two or more `**Label:**` bold-field
markers crammed onto one physical line, with no real line break between
distinct fields, near the top of a file — for example:

```
**Status:** authoritative · **Last updated:** 2026-06-11
```

See [`docs/doc-header-format.md`](../../docs/doc-header-format.md) for the
canonical replacement format and the reasoning behind it, and
[`tools/header-lint`](../../tools/header-lint) for the thin CLI adapter.

## EARS Requirements

**HEADERLINT-01** When a Markdown line within a document's header zone contains two or more `**Label:**` bold-field markers, the system shall report one violation for that line.

**HEADERLINT-02** Where a document's header zone is defined, the system shall scan from the top of the file up to, but not including, the first level-2-through-6 ATX heading, or the first 15 lines, whichever comes first.

**HEADERLINT-03** The system shall not report a violation for two or more bold-field-shaped terms appearing on a line outside the header zone.

**HEADERLINT-04** The system shall not report a violation for a line inside a fenced code block, even within the header zone.

**HEADERLINT-05** The system shall not report a violation for a line containing exactly one `**Label:**` bold-field marker.

**HEADERLINT-06** When `CheckFile` is called on a Markdown file, the system shall scan that file's header zone.

**HEADERLINT-07** When `CheckDir` is called on a directory, the system shall recursively scan every file with a `.md` extension under that directory.

**HEADERLINT-08** When `CheckRepository` is called on a Git repository root, the system shall scan every Git-tracked file with a `.md` extension in that repository.

**HEADERLINT-09** When a file cannot be read, the system shall return an operational error rather than a violation.

## BDD Traceability

No dedicated BDD feature file. Coverage lives in
`pkg/headerlint/headerlint_test.go`, including the real-world REVIEW.md and
`docs/code-review-automation-setup.md` header shapes (as fixtures, not by
reading those files directly) and the prose false-positive case from the
originating task.
