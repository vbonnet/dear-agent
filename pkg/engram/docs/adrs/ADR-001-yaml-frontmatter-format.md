# ADR-001: Store Engram metadata in YAML frontmatter

Status: Accepted

## Context

Engrams are Markdown documents that need machine-readable identity, retrieval,
and provenance fields without separating content from metadata.

## Decision

An Engram file begins with YAML frontmatter delimited by `---`, followed by
Markdown content. The parser rejects missing, unclosed, or invalid frontmatter.
Typed fields are owned by `Frontmatter`; unknown format evolution must remain
compatible with the parser and migrations.

## Consequences

- Files remain readable and editable as Markdown.
- Metadata and content move atomically.
- YAML parsing and delimiter errors are part of the public validation contract.

## Evidence

- `../../parser.go` and `../../parser_test.go`
- `../../engram.go`
