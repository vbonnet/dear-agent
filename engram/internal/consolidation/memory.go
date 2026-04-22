package consolidation

import "time"

// MemoryType categorizes memories by cognitive function.
type MemoryType string

const (
	// Episodic memories are specific experiences (what happened).
	// Example: "Completed D1 validation phase at 2025-12-15T10:30:00Z"
	Episodic MemoryType = "episodic"

	// Semantic memories are extracted knowledge (facts learned).
	// Example: "MemoryConsolidation API uses tiered architecture"
	Semantic MemoryType = "semantic"

	// Procedural memories are skills and procedures (how to do things).
	// Example: "Multi-persona review: create personas, draft questions, score responses"
	Procedural MemoryType = "procedural"

	// Working memories are active context (immediate focus).
	// Example: "Currently implementing D4 solution requirements"
	Working MemoryType = "working"
)

// Memory represents a single memory entry in long-term storage.
//
// Memories are organized by namespace (hierarchical scoping) and type (cognitive function).
// Optional fields like Embedding and Importance can be omitted for simpler use cases.
type Memory struct {
	// SchemaVersion indicates the memory schema version (e.g., "1.0").
	// Enables schema evolution and backward compatibility.
	SchemaVersion string `json:"schema_version"`

	// ID uniquely identifies this memory (e.g., "mem-uuid").
	ID string `json:"id"`

	// Type categorizes the memory (episodic, semantic, procedural, working).
	Type MemoryType `json:"type"`

	// Namespace organizes memories hierarchically for scoping.
	// Example: ["user", "alice", "project", "myapp"]
	Namespace []string `json:"namespace"`

	// Content is the memory payload (flexible structure).
	// Can be string, struct, or any JSON-serializable type.
	Content interface{} `json:"content"`

	// Metadata stores additional key-value attributes.
	// Example: {"importance": 0.9, "source": "wayfinder", "phase": "D1"}
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Timestamp when memory was created.
	Timestamp time.Time `json:"timestamp"`

	// Embedding is optional vector representation for semantic search.
	// Empty in v0.1.0 (embeddings not implemented).
	Embedding []float64 `json:"embedding,omitempty"`

	// Importance is optional priority score (0-1 scale).
	// Higher values indicate more critical memories.
	Importance float64 `json:"importance,omitempty"`
}

// WorkingContext is a compiled view of active session state.
//
// Working context aggregates relevant information for current work:
// - Current phase and active tasks
// - Recent history (sliding window of events)
// - Relevant memories retrieved from long-term storage
// - Pinned items that must stay in context
// - Artifact references (not full content)
type WorkingContext struct {
	SessionID      string         // Session identifier
	CurrentPhase   string         // Current workflow phase (e.g., "D4", "S8")
	ActiveTasks    []Task         // Tasks in progress
	RecentHistory  []SessionEvent // Last N events (sliding window)
	RelevantMemory []Memory       // Retrieved memories for current context
	PinnedItems    []Memory       // Critical info that must stay in context
	Artifacts      []ArtifactRef  // Artifact handles (not full content)
}

// SessionHistory is an append-only event log for a session.
//
// Session history provides complete audit trail of all events from
// session start to current time (or session end if persisted).
type SessionHistory struct {
	SessionID string         // Session identifier
	Events    []SessionEvent // Chronological event list
	StartTime time.Time      // Session start
	EndTime   *time.Time     // Session end (nil if active)
}

// Task represents an active task in working context.
type Task struct {
	ID          string
	Description string
	Status      string // "pending", "in_progress", "completed"
}

// SessionEvent represents a single event in session history.
type SessionEvent struct {
	Timestamp time.Time
	Type      string      // Event type (e.g., "phase_started", "memory_stored")
	Data      interface{} // Event payload
}

// ArtifactRef is a handle to an artifact (not the content itself).
//
// Artifacts are stored separately from memories to avoid bloating
// memory entries with large binary data.
type ArtifactRef struct {
	ID   string
	Type string // File extension or MIME type
	Size int64  // Byte size
}

// ContextUpdate describes changes to working context.
//
// Updates are applied atomically. Only specified fields are modified.
// Nil pointer fields are ignored, enabling partial updates.
type ContextUpdate struct {
	SetPhase      *string  // Update current phase
	AddTasks      []Task   // Add new tasks
	CompleteTasks []string // Mark task IDs as completed
	PinMemories   []string // Pin memory IDs
	UnpinMemories []string // Unpin memory IDs
}

// MemoryUpdate specifies partial updates to an existing memory.
//
// Only non-nil fields are applied, enabling selective updates without
// having to read and replace the entire memory.
//
// Example - Replace content:
//
//	var content interface{} = "New content"
//	updates := MemoryUpdate{SetContent: &content}
//	err := provider.UpdateMemory(ctx, namespace, "mem-123", updates)
//
// Example - Append to content (string only):
//
//	appendText := " - Additional info"
//	updates := MemoryUpdate{AppendContent: &appendText}
//	err := provider.UpdateMemory(ctx, namespace, "mem-123", updates)
//
// Example - Update metadata:
//
//	updates := MemoryUpdate{
//	    SetMetadata: map[string]interface{}{"reviewed": true},
//	}
//	err := provider.UpdateMemory(ctx, namespace, "mem-123", updates)
//
// Example - Multiple updates:
//
//	importance := 0.9
//	updates := MemoryUpdate{
//	    AppendContent: &appendText,
//	    SetMetadata:   map[string]interface{}{"reviewed": true},
//	    SetImportance: &importance,
//	}
//	err := provider.UpdateMemory(ctx, namespace, "mem-123", updates)
type MemoryUpdate struct {
	// SetContent replaces the entire content field.
	// Use this for complete content replacement.
	SetContent *interface{} `json:"set_content,omitempty"`

	// AppendContent appends to the existing content.
	// Only works if current content is a string. Returns error otherwise.
	AppendContent *string `json:"append_content,omitempty"`

	// SetMetadata updates metadata fields (merges with existing).
	// Keys present in SetMetadata override existing values.
	SetMetadata map[string]interface{} `json:"set_metadata,omitempty"`

	// SetImportance updates the importance score (0-1 scale).
	SetImportance *float64 `json:"set_importance,omitempty"`

	// SetType changes the memory type.
	SetType *MemoryType `json:"set_type,omitempty"`

	// SetEmbedding updates the embedding vector.
	// Not used in v0.1.0 (embeddings not implemented).
	SetEmbedding []float64 `json:"set_embedding,omitempty"`
}
