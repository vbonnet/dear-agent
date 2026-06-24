package corpus

// Engram schemas for corpus callosum registration
// These schemas allow other components (AGM, Wayfinder, Swarm) to
// discover and query Engram's bead and memory data.

// EngramComponentName is the corpus-callosum component name for Engram.
const EngramComponentName = "engram"

// EngramComponentVersion is the schema version Engram registers with corpus callosum.
//
// 1.1.0 adds the "document" schema, splitting Engram's knowledge model into two
// layers: stateless versioned documents and mutable extracted memory traces.
const EngramComponentVersion = "1.1.0"

// GetEngramSchema returns the complete Engram schema definition for corpus callosum.
func GetEngramSchema() map[string]interface{} {
	return map[string]interface{}{
		"component":     EngramComponentName,
		"version":       EngramComponentVersion,
		"compatibility": "backward",
		"schemas": map[string]interface{}{
			"bead":           GetBeadSchema(),
			"document":       GetDocumentSchema(),
			"memory_trace":   GetMemoryTraceSchema(),
			"ecphory_result": GetEcphoryResultSchema(),
		},
	}
}

// GetBeadSchema returns the schema definition for bead (issue tracking) data.
func GetBeadSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Engram bead (issue) tracking schema",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Unique bead identifier (e.g., oss-abc1)",
				"pattern":     "^[a-z]+-[a-z0-9]{4}$",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Short description of the task/issue",
				"maxLength":   200,
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Detailed description of the task",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"open", "in-progress", "blocked", "closed"},
				"description": "Current status of the bead",
			},
			"priority": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"P0", "P1", "P2", "P3"},
				"description": "Priority level (P0 = highest)",
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Workspace this bead belongs to (e.g., oss, acme)",
			},
			"labels": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "Tags/labels for categorization",
			},
			"estimate": map[string]interface{}{
				"type":        "integer",
				"description": "Estimated time in minutes",
				"minimum":     0,
			},
			"created_at": map[string]interface{}{
				"type":        "string",
				"format":      "date-time",
				"description": "When the bead was created",
			},
			"updated_at": map[string]interface{}{
				"type":        "string",
				"format":      "date-time",
				"description": "Last update timestamp",
			},
			"closed_at": map[string]interface{}{
				"type":        "string",
				"format":      "date-time",
				"description": "When the bead was closed (if status=closed)",
			},
		},
		"required": []string{"id", "title", "status", "workspace"},
	}
}

// GetDocumentSchema returns the schema definition for document data.
//
// Documents are Engram's stateless knowledge layer: immutable, versioned blobs
// (specs, architecture docs, research findings, reference material). Unlike a
// memory_trace — a mutable extracted fact — a document is authored content
// trusted as-is. "Editing" a document appends a new version; the (id, version)
// pair is immutable. See internal/document for the storage contract.
func GetDocumentSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Engram document storage schema (stateless versioned knowledge blob)",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Stable logical identifier shared by every version (e.g. engram-spec)",
			},
			"version": map[string]any{
				"type":        "integer",
				"description": "Monotonic 1-based version number; (id, version) is immutable",
				"minimum":     1,
			},
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"spec", "architecture", "research", "reference", "adr"},
				"description": "Editorial role of the document",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Short human-readable label",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Canonical knowledge blob (typically markdown or text)",
			},
			"content_hash": map[string]any{
				"type":        "string",
				"description": "Lowercase hex SHA-256 of content (integrity and dedup)",
			},
			"source": map[string]any{
				"type":        "string",
				"description": "Provenance (file path, URL, or generating session)",
			},
			"workspace": map[string]any{
				"type":        "string",
				"description": "Workspace this document belongs to",
			},
			"created_at": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
		},
		"required": []string{"id", "version", "content", "workspace"},
	}
}

// GetMemoryTraceSchema returns the schema definition for memory/engram trace data.
func GetMemoryTraceSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Engram memory trace storage schema",
		"properties": map[string]interface{}{
			"trace_id": map[string]interface{}{
				"type":        "string",
				"description": "Unique identifier for this memory trace",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The stored memory/knowledge content",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Source of this memory (file path, URL, user input)",
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Workspace this memory belongs to",
			},
			"tags": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "Classification tags",
			},
			"embedding": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "number",
				},
				"description": "Vector embedding for similarity search",
			},
			"created_at": map[string]interface{}{
				"type":   "string",
				"format": "date-time",
			},
		},
		"required": []string{"trace_id", "content", "workspace"},
	}
}

// GetEcphoryResultSchema returns the schema for ecphory (memory retrieval) results.
func GetEcphoryResultSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Ecphory (memory retrieval) result schema",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query used",
			},
			"results": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"trace_id": map[string]interface{}{
							"type": "string",
						},
						"content": map[string]interface{}{
							"type": "string",
						},
						"relevance_score": map[string]interface{}{
							"type":    "number",
							"minimum": 0.0,
							"maximum": 1.0,
						},
					},
				},
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Workspace context for the query",
			},
			"timestamp": map[string]interface{}{
				"type":   "string",
				"format": "date-time",
			},
		},
		"required": []string{"query", "results", "workspace"},
	}
}
