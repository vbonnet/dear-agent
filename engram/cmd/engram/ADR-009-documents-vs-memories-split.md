# ADR-009: Documents vs Memories — Two-Layer Knowledge Model

**Status**: Accepted

**Date**: 2026-06-21

**Context**:

Engram's knowledge model historically conflated two fundamentally different
kinds of stored knowledge under a single `Memory` abstraction
(`internal/consolidation.Memory`, persisted by the pluggable Provider of
ADR-007):

1. **Authored reference knowledge** — specs, architecture docs, research
   findings, runbooks. This content is *trusted as-is*, large, canonical, and
   changes only through deliberate authoring. It has no decay, no importance
   score, and no consolidation lifecycle. When it changes, the previous version
   still matters (audit, diff, rollback).

2. **Extracted facts** — small distilled claims derived from session history
   ("the user prefers concise responses", "the build uses `make oss`"). These
   are *learned*, mutated in place, merged, decayed, and pruned by the
   hippocampus consolidation pipeline. They are inherently mutable and
   probabilistic.

Storing both as one type caused real problems:

- **Stale-fact contamination**: consolidation logic (merge, prune, decay,
  importance scoring) is correct for extracted facts but actively *wrong* for
  canonical documents. A spec must not silently decay or get merged into a
  half-remembered fact.
- **Recall imprecision**: ecphory ranks a 5-line extracted preference against a
  2,000-line architecture doc with the same machinery, blurring results.
- **No version history** for reference material: the in-place `UpdateMemory`
  /`AppendContent` model destroys prior content, which is unacceptable for
  authored knowledge.

This mirrors the [supermemory](https://github.com/supermemoryai) OSS pattern,
which separates **documents** (source-of-truth blobs) from **memories**
(distilled, evolving facts).

**Decision**:

Split Engram's knowledge model into two distinct, sibling layers with separate
types and storage contracts.

| Aspect            | Documents (`internal/document`)        | Memories (`internal/consolidation`)     |
|-------------------|----------------------------------------|-----------------------------------------|
| Nature            | Stateless authored knowledge blob      | Extracted, distilled fact               |
| Mutability        | **Immutable**; edits append a version  | Mutable in place (`UpdateMemory`)       |
| Versioning        | Monotonic `(id, version)`, all retained| None (single current value)             |
| Lifecycle         | No decay / merge / prune               | Consolidation: decay, merge, prune      |
| Importance score  | No                                     | Yes (0–1)                               |
| Source of truth   | Yes — trusted as-is                    | No — probabilistic, derived             |
| Typical size      | Large (docs, specs)                    | Small (one claim)                       |
| Categorization    | `Kind`: spec/architecture/research/…   | `MemoryType`: episodic/semantic/…       |

**Document type** (`internal/document.Document`):

```go
type Document struct {
    SchemaVersion string
    ID            string   // stable logical id across versions
    Version       int      // monotonic, 1-based, store-assigned
    Kind          Kind     // spec | architecture | research | reference | adr
    Namespace     []string
    Title         string
    Content       string   // canonical blob (markdown/text)
    ContentHash   string   // sha256(content), store-computed
    Source        string
    Metadata      map[string]interface{}
    CreatedAt     time.Time
}
```

**Store contract** (`internal/document.Store`) is deliberately narrower than the
memory `Provider`: there is no in-place update and no append-to-content. The
only mutation is `Put`, which appends a new immutable version.

```go
type Store interface {
    Put(ctx, namespace, doc) (Document, error)        // appends a new version
    Get(ctx, namespace, id) (Document, error)         // latest version
    GetVersion(ctx, namespace, id, version) (Document, error)
    ListVersions(ctx, namespace, id) ([]Document, error)
    List(ctx, namespace, filter) ([]Document, error)  // latest of each
    Delete(ctx, namespace, id) error                  // admin: drop all versions
}
```

The v0.1 reference implementation is `FSStore`: one directory per document, one
JSON file per version (`{root}/{namespace…}/{id}/v{N}.json`). One file per
version makes immutability the natural on-disk state and keeps history
git-friendly. It reuses the security-first path-containment validation of
ADR-006.

**Corpus-callosum schema**: a new `document` schema is registered alongside
`bead`, `memory_trace`, and `ecphory_result`, bumping the Engram component
schema version to `1.1.0` (backward-compatible — additive only). Like every
Engram schema it carries a required `workspace` field for isolation (ADR-005).

**Rationale**:

1. **Recall precision**: documents and memories can be ranked and budgeted
   separately; ecphory no longer compares a one-line fact against a full spec
   with identical machinery.
2. **No stale-fact contamination**: consolidation (decay/merge/prune) applies
   only to memories. Canonical knowledge is never silently mutated.
3. **Versioned reference knowledge**: authored content gets immutable history
   for free — audit, diff, rollback.
4. **Clear authoring vs learning boundary**: a `Put` is an authoring act; a
   memory write is a learning act. The type system now reflects that.

**Alternatives Considered**:

1. **Keep one `Memory` type, add a `category` flag** — rejected: the lifecycle
   rules (decay, merge, immutability, versioning) diverge too far to live behind
   one interface; the flag would gate behavior everywhere and rot.
2. **Documents as a `MemoryType`** — rejected: `MemoryType` describes a
   *cognitive function* (episodic/semantic/…); document `Kind` describes an
   *editorial role*. Overloading the enum conflates orthogonal axes.
3. **Store documents as artifacts** (ADR-007 artifact handles) — rejected:
   artifacts are opaque binary blobs with no query/version semantics; documents
   need kind/title filtering and version history.

**Consequences**:

**Positive**:
- Sharper recall, no decay on canonical knowledge, free version history.
- Smaller, easier-to-reason-about interfaces per layer.
- Maps cleanly to the supermemory OSS mental model.

**Negative**:
- Two storage layers to maintain instead of one.
- Callers must choose the right layer; mis-classification is possible (mitigated
  by the type split making the choice explicit at the call site).
- Retrieval that wants *both* layers must query both and merge (future work).

**Implementation Phases**:

**Phase 1 (this ADR)**: Foundational layer
- [x] `internal/document` package: `Document`, `Kind`, `Store`, sentinel errors
- [x] `FSStore` filesystem reference implementation with version history
- [x] Path-containment validation (ADR-006)
- [x] Corpus `document` schema registration (component schema `1.1.0`)
- [x] Unit tests for store contract and security validation

**Phase 2 (future)**: Integration
- [ ] CLI surface: `engram document put|get|list|versions`
- [ ] MCP tools for document operations
- [ ] Ecphory retrieval over documents with separate ranking/budget
- [ ] Migration: reclassify existing reference-kind memories into documents

**Phase 3 (future)**: Cross-layer recall
- [ ] Unified query that ranks documents and memories with layer-aware scoring
- [ ] Provenance links from extracted memories back to source documents

**Related Decisions**:
- ADR-005: Hierarchical Workspace Structure (namespace/workspace isolation)
- ADR-006: Security-First Input Validation (path containment reused by FSStore)
- ADR-007: Pluggable Memory Provider Architecture (the memory layer this splits from)
