# document — Engram's stateless knowledge layer

`internal/document` is one of Engram's two knowledge layers (see
[ADR-009](../../cmd/engram/ADR-009-documents-vs-memories-split.md)):

| Layer | Package | Nature |
|-------|---------|--------|
| **Documents** | `internal/document` | Stateless, immutable, **versioned** knowledge blobs (specs, architecture, research, reference, ADRs). Trusted as-is. |
| **Memories** | `internal/consolidation` | Mutable, extracted facts distilled from session history. Decayed, merged, pruned. |

A `Document` is authored content that is never mutated in place. "Editing" a
document appends a new immutable version; prior versions are retained. There is
no decay, importance score, or consolidation lifecycle — that machinery belongs
to the memory layer and would be actively wrong applied to canonical knowledge.

## Usage

```go
store, _ := document.NewFSStore("/path/to/docs")
ns := []string{"project", "engram"}

// Author v1.
v1, _ := store.Put(ctx, ns, document.Document{
    ID:      "engram-spec",
    Kind:    document.KindSpec,
    Title:   "Engram Spec",
    Content: "# Engram\n...",
})

// Editing appends v2; v1 is retained and unchanged.
v2, _ := store.Put(ctx, ns, document.Document{ID: "engram-spec", Content: "..."})

latest, _ := store.Get(ctx, ns, "engram-spec")        // v2
old, _    := store.GetVersion(ctx, ns, "engram-spec", 1) // v1
specs, _  := store.List(ctx, ns, document.Filter{Kind: document.KindSpec})
```

## Storage layout (`FSStore`)

```
{root}/{namespace…}/{id}/v{N}.json
```

One file per version makes immutability the natural on-disk state and keeps
history git-friendly. Namespace and ID inputs are treated as hostile and
validated for path traversal, with a load-bearing post-join containment check
(mirrors ADR-006 and the simple memory provider).
