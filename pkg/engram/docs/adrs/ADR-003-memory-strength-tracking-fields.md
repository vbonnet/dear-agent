# ADR-003: Keep memory-strength fields in Engram metadata

Status: Accepted

## Context

Retrieval needs to distinguish intrinsic encoding strength from observed access
and reinforcement without rewriting the document body.

## Decision

Memory-strength and retrieval lifecycle values are typed frontmatter fields.
Encoding strength defaults to a neutral value when absent; counts and timestamps
retain their zero-value meanings until an access or update path changes them.
Ranking code consumes the typed values rather than reparsing raw YAML.

## Consequences

- Retrieval metadata remains inspectable with the source document.
- Updates must preserve unrelated frontmatter and body content.
- Field semantics are shared by parser, ranking, and migration tests.

## Evidence

- `../../engram.go`
- `../../parser.go` and backward-compatibility tests
