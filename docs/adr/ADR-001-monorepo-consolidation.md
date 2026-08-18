# ADR-001: Monorepo Consolidation

Status: Accepted (2026-04-24)

`ai-tools` and `engram` lived in two repositories with `replace` directives
pointing at absolute local paths. Every coordinated change wanted two PRs,
import paths drifted, and cross-cutting refactors were nearly impossible to
review atomically.

Consolidate into one Go module rooted at the old `ai-tools` repo. Subtrees:
`agm/`, `engram/`, `wayfinder/`, `pkg/`, `internal/`, `tools/`. The old
`engram` repository keeps stub READMEs pointing here. `go build ./...` now
covers everything.

The trade-off is one large tree to navigate versus atomic cross-component
commits. We took the latter: a refactor that touches AGM, the engram client,
and a shared package is one diff to review, not three coordinated PRs across
two repos that may drift while the reviews land.
