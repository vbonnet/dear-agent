package corpus

// Engram schemas for corpus callosum registration
// These schemas allow other components (AGM, Wayfinder, Swarm) to
// discover and query Engram's bead and memory data.

const EngramComponentName = "engram"
const EngramComponentVersion = "1.0.0"

// GetEngramSchema returns the complete Engram schema definition for corpus callosum.
func GetEngramSchema() map[string]interface{} {
	return map[string]interface{}{
		"component":     EngramComponentName,
		"version":       EngramComponentVersion,
		"compatibility": "backward",
		"schemas": map[string]interface{}{
			"bead":           GetBeadSchema(),
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
