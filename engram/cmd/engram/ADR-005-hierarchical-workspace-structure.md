# ADR-005: Scope Engram knowledge by workspace

Status: Accepted

## Context

Knowledge can be personal, shared across related work, or specific to one
repository. A single undifferentiated store leaks irrelevant context; a store
per command makes reuse impossible.

## Decision

Engram resolves an explicit workspace when supplied and otherwise derives the
workspace through its configuration and repository context. Commands pass that
scope into storage and retrieval boundaries. Cross-workspace aggregation is an
explicit operation rather than a default query behavior.

Filesystem paths are configuration details; documents and memories retain
logical workspace identity independent of their physical provider.

## Consequences

- Retrieval has a predictable context boundary.
- Shared knowledge requires an intentional scope.
- Moves between physical providers do not change logical ownership.

## Evidence

- `cmd/workspace.go`
- `../../../pkg/workspace/` and `../../../pkg/engram/`
