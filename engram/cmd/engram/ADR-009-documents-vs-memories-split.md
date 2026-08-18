# ADR-009: Separate documents from memories

Status: Accepted

## Context

Reference documents and agent memories have different provenance, mutation,
ranking, and retention needs. Treating both as one record type either makes
documents mutable like memories or strips memory-specific lifecycle fields.

## Decision

Engram keeps documents and memories as distinct knowledge layers. Documents are
source-oriented reference content. Memories are learned, mutable records with
strength and retrieval metadata. Commands and storage APIs name the layer they
operate on; retrieval may compose results without erasing provenance.

## Consequences

- Callers choose mutation and provenance semantics explicitly.
- Shared retrieval must preserve the originating layer.
- Migration between layers is an intentional conversion.

## Evidence

- `cmd/memory.go`, `cmd/retrieve.go`, and their tests
- `../../SPEC.md`
