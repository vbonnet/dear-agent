package consolidation

import "context"

// Provider defines memory consolidation operations for agent memory management.
//
// Implementations must support four tiers of memory operations:
//   - Working Memory: immediate context (compiled view)
//   - Session Memory: full trajectory of current session (event log)
//   - Long-term Memory: cross-session knowledge (searchable)
//   - Artifacts: large objects referenced by handle (external storage)
//
// All operations are context-aware for cancellation and timeout support.
//
// Example - Store and retrieve memory:
//
//	provider, _ := Load(config)
//	memory := Memory{
//	    ID:        "mem-123",
//	    Type:      Episodic,
//	    Namespace: []string{"user", "alice"},
//	    Content:   "Implemented authentication",
//	    Timestamp: time.Now(),
//	}
//	provider.StoreMemory(ctx, memory.Namespace, memory)
//
//	query := Query{Type: Episodic, Limit: 10}
//	memories, _ := provider.RetrieveMemory(ctx, []string{"user", "alice"}, query)
//
// Example - Update memory:
//
//	appendText := " - Added JWT support"
//	updates := MemoryUpdate{AppendContent: &appendText}
//	provider.UpdateMemory(ctx, namespace, "mem-123", updates)
type Provider interface {
	// Working Memory Operations

	// GetWorkingContext retrieves the compiled working context for a session.
	// Working context is a dynamically constructed view of active tasks,
	// recent events, relevant memories, and pinned items.
	GetWorkingContext(ctx context.Context, sessionID string) (*WorkingContext, error)

	// UpdateWorkingContext applies updates to the working context.
	// Updates can add/remove tasks, pin/unpin items, or modify current phase.
	UpdateWorkingContext(ctx context.Context, sessionID string, updates ContextUpdate) error

	// Session Memory Operations

	// GetSessionHistory retrieves the complete event log for a session.
	// Returns all events from session start to current time.
	GetSessionHistory(ctx context.Context, sessionID string) (*SessionHistory, error)

	// AppendSessionEvent appends a new event to the session history.
	// Events are immutable and append-only for audit trail.
	AppendSessionEvent(ctx context.Context, sessionID string, event SessionEvent) error

	// PersistSession marks session as complete and persists to long-term storage.
	// After persistence, session history becomes part of searchable memory.
	PersistSession(ctx context.Context, sessionID string) error

	// Long-term Memory Operations

	// StoreMemory stores a memory entry in long-term storage.
	// Memories are organized by namespace for scoping and access control.
	// If a memory with the same ID exists in the namespace, it is replaced.
	StoreMemory(ctx context.Context, namespace []string, memory Memory) error

	// RetrieveMemory retrieves memories matching the query.
	// Supports filtering by type, time range, importance, and text search.
	// Results are sorted by timestamp descending (newest first).
	RetrieveMemory(ctx context.Context, namespace []string, query Query) ([]Memory, error)

	// UpdateMemory applies partial updates to an existing memory entry.
	// Only the fields specified in MemoryUpdate are modified.
	// Returns ErrNotFound if the memory does not exist.
	//
	// Example - Append to content:
	//
	//	appendText := " - Review complete"
	//	updates := MemoryUpdate{AppendContent: &appendText}
	//	err := provider.UpdateMemory(ctx, namespace, "mem-123", updates)
	//
	// Example - Update metadata and importance:
	//
	//	importance := 0.95
	//	updates := MemoryUpdate{
	//	    SetMetadata:   map[string]interface{}{"reviewed": true},
	//	    SetImportance: &importance,
	//	}
	//	err := provider.UpdateMemory(ctx, namespace, "mem-123", updates)
	UpdateMemory(ctx context.Context, namespace []string, memoryID string, updates MemoryUpdate) error

	// DeleteMemory removes a memory entry from long-term storage.
	// Returns ErrNotFound if the memory does not exist.
	DeleteMemory(ctx context.Context, namespace []string, memoryID string) error

	// Artifact Operations

	// StoreArtifact stores a large object (file, binary data) by handle.
	// Artifacts are referenced from memories but stored separately.
	StoreArtifact(ctx context.Context, artifactID string, data []byte) error

	// GetArtifact retrieves an artifact by handle.
	// Returns ErrNotFound if the artifact does not exist.
	GetArtifact(ctx context.Context, artifactID string) ([]byte, error)

	// DeleteArtifact removes an artifact from storage.
	// Returns ErrNotFound if the artifact does not exist.
	DeleteArtifact(ctx context.Context, artifactID string) error

	// Lifecycle Operations

	// Initialize prepares the provider with configuration.
	// Called once during provider registration.
	Initialize(ctx context.Context, config Config) error

	// Close cleanly shuts down the provider and releases resources.
	Close(ctx context.Context) error

	// HealthCheck verifies provider is operational.
	// Returns error if provider cannot perform operations.
	HealthCheck(ctx context.Context) error
}
