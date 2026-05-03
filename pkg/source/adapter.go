// Package source defines the adapter contract by which the workflow
// engine reads and writes durable, addressable knowledge. It is the
// canonical surface behind the FetchSource / AddSource MCP tools and
// behind outputs declared with durability=engram_indexed.
//
// The adapter is intentionally minimal — Add stores a Source, Fetch
// retrieves Sources matching a query, HealthCheck reports liveness,
// Close releases resources. The default adapter is sqlite (see
// pkg/source/sqlite); Obsidian, llm-wiki, and OpenViking adapters
// can plug in by satisfying the same interface (Phase 5).
//
// The contract that every adapter must satisfy lives in
// pkg/source/contract: TestAdapter_AddFetch_RoundTrip,
// TestAdapter_FetchByCue_Filters, TestAdapter_FetchByWorkItem_Filters,
// TestAdapter_HealthCheck. New adapters that do not pass these tests
// are not substrate-quality and should be rejected at PR review.
package source

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by adapters that look up a Source by URI and
// find nothing. Callers should check with errors.Is.
var ErrNotFound = errors.New("source: not found")

// Adapter is the interface every storage backend implements. The
// engine talks to exactly one adapter at a time, selected at startup
// by a small registry (TODO Phase 5). Adapters are expected to be safe
// for concurrent use by multiple goroutines.
type Adapter interface {
	// Name returns a stable identifier for the backend, e.g. "sqlite",
	// "obsidian", "llm-wiki". The MCP tools include this in their
	// responses so the caller can detect a backend mismatch.
	Name() string

	// HealthCheck returns nil if the adapter can serve reads and writes,
	// or an error describing why it cannot. It must be cheap — callers
	// invoke it on every MCP request to surface broken backends loudly.
	HealthCheck(ctx context.Context) error

	// Fetch returns up to q.K Sources matching q. Empty Query is allowed
	// (returns the most-recent Sources matching the filters). Adapters
	// must order results by descending relevance, then by descending
	// IndexedAt for ties.
	Fetch(ctx context.Context, q FetchQuery) ([]Source, error)

	// Add stores s and returns a Ref that can be used to reach it later.
	// Adapters are expected to be idempotent on s.URI: re-adding the
	// same URI updates content and metadata in place rather than
	// creating a duplicate.
	Add(ctx context.Context, s Source) (Ref, error)

	// Close releases resources (DB handles, file watchers, etc.).
	// Calling Close on an already-closed adapter is a no-op.
	Close() error
}

// FetchQuery is the read shape: a free-form Query string plus structured
// Filters plus a count K. K=0 means "use the adapter default" (the SQLite
// adapter's default is 10).
type FetchQuery struct {
	// Query is a free-form search string. Adapters interpret this in
	// backend-native terms — FTS5 for sqlite, ripgrep for llm-wiki,
	// etc. Empty Query = no text predicate.
	Query string

	// Filters narrow the result set by structured fields. All filters
	// AND together. See Filters for the full list.
	Filters Filters

	// K is the maximum number of Sources to return. Zero means "adapter
	// default". Adapters must honour K exactly when non-zero.
	K int

	// Rerank, when true, asks the adapter to rerank results using any
	// available semantic / vector capability. Adapters without one must
	// silently ignore Rerank rather than failing — the contract is
	// best-effort.
	Rerank bool
}

// Filters is the structured-predicate part of a FetchQuery. Every field
// is optional; an empty Filters matches everything.
type Filters struct {
	// Cues filters by Source.Metadata.Cues. A Source matches if it
	// contains every Cue in the slice (AND semantics). Case-sensitive.
	Cues []string

	// Backend, if non-empty, asserts the expected adapter name. The
	// MCP layer checks this against Adapter.Name() and returns a
	// clear error on mismatch; adapters themselves ignore it.
	Backend string

	// After / Before filter by Source.IndexedAt. Both are inclusive on
	// the "since" side and exclusive on the "until" side, matching the
	// convention of the `dear-agent search --since` flag.
	After  *time.Time
	Before *time.Time

	// WorkItem filters by Source.Metadata.WorkItem (the run_id, or
	// run_id/node_id, that produced the source). Empty = no filter.
	WorkItem string
}

// Source is the canonical representation of one stored knowledge
// artifact. URI is the primary key; Title and Snippet are
// human-readable summaries; Content holds the bytes (which adapters
// may store opaque or index for FTS).
type Source struct {
	URI       string
	Title     string
	Snippet   string
	Content   []byte
	Score     float64
	Metadata  Metadata
	IndexedAt time.Time
}

// Metadata is the structured side-information attached to every
// Source. Adapters persist these fields so Filters can target them.
type Metadata struct {
	Cues       []string
	WorkItem   string
	Role       string
	Confidence float64
	Source     string
	Custom     map[string]any
}

// Ref is what Add returns: a stable handle to the just-stored Source.
// Backend identifies the adapter that owns it; URI is the primary key.
type Ref struct {
	URI       string
	Backend   string
	IndexedAt time.Time
}
