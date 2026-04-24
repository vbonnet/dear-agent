package consolidation

import "time"

// Query specifies criteria for retrieving memories.
//
// Queries support multiple filter types that can be combined:
// - Type filtering (episodic, semantic, etc.)
// - Namespace prefix matching
// - Importance threshold
// - Time range filtering
// - Text substring search
//
// Example - Filter by type with limit:
//
//	query := Query{
//	    Type:  Episodic,
//	    Limit: 10,
//	}
//	memories, err := provider.RetrieveMemory(ctx, namespace, query)
//
// Example - Filter by importance and time:
//
//	query := Query{
//	    MinImportance: 0.8,
//	    TimeRange: &TimeRange{
//	        Start: time.Now().Add(-24 * time.Hour),
//	        End:   time.Now(),
//	    },
//	    Limit: 20,
//	}
//	memories, err := provider.RetrieveMemory(ctx, namespace, query)
type Query struct {
	// Text is optional natural language query for semantic search.
	// In v0.1.0, performs simple substring matching on content.
	// Future: semantic search using embeddings.
	Text string

	// Type filters by memory type (episodic, semantic, procedural, working).
	// Empty string matches all types.
	Type MemoryType

	// Namespace filters by namespace prefix.
	// Example: ["user", "alice"] matches all memories under user alice.
	Namespace []string

	// Limit caps the number of results (default: 10, 0 = unlimited).
	// Results are sorted by timestamp descending before limit is applied.
	Limit int

	// MinImportance filters memories by importance threshold (0-1).
	// Only memories with Importance >= MinImportance are returned.
	// Default: 0 (no filtering)
	MinImportance float64

	// TimeRange filters memories by timestamp.
	// Only memories within the time window are returned.
	TimeRange *TimeRange

	// Embedding is optional query vector for semantic search.
	// Not used in v0.1.0 (embeddings not implemented).
	Embedding []float64
}

// TimeRange specifies a time window for filtering.
type TimeRange struct {
	Start time.Time // Inclusive start
	End   time.Time // Exclusive end
}
