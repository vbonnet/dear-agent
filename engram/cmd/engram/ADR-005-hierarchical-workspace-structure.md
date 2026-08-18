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

Workspace identity is enforced by the selected storage path/provider. Records
do not currently persist a second logical-workspace field, so providers must
not mix workspace data under one selected path.

## Consequences

- Retrieval has a predictable context boundary.
- Shared knowledge requires an intentional scope.
- Moving data between providers requires preserving the workspace boundary.

## Evidence

- `cmd/workspace.go`
- `../../../pkg/workspace/` and `../../../pkg/engram/`
