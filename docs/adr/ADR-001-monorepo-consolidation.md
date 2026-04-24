# ADR-001: Monorepo Consolidation

## Status

Accepted

## Context

The ai-tools ecosystem was split across two repositories:
- `ai-tools` — AGM session management, codegen, and standalone tools
- `engram` — Memory system, wayfinder, hooks, and shared libraries

This split caused cross-repo dependency issues, import path complexity
with `replace` directives pointing to absolute local paths, and made
it difficult to make coordinated changes across components.

## Decision

Consolidate into a single monorepo (`ai-tools`) with one `go.mod` at root:
- `agm/` — AGM (renamed from agm)
- `engram/` — Engram memory system (copied from engram repo)
- `wayfinder/` — Wayfinder plugin (copied from engram repo)
- `pkg/` — Shared libraries
- `internal/` — Shared internal packages
- `tools/` — Standalone tools

## Consequences

- Single `go build ./...` compiles everything
- No more cross-repo `replace` directives
- Coordinated changes are atomic commits
- engram repo retains stub READMEs pointing to new locations
