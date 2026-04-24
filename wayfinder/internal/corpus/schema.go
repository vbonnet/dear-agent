package corpus

// GetWayfinderSchema returns the schema definition for Wayfinder projects
// Schema includes workspace field for isolation
func GetWayfinderSchema() map[string]interface{} {
	return map[string]interface{}{
		"component": "wayfinder",
		"version":   "1.0.0",
		"entity":    "project",
		"fields": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Unique session identifier for this Wayfinder project",
				"required":    true,
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Workspace this project belongs to (oss, acme, etc.)",
				"required":    true,
				"indexed":     true,
			},
			"project_path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute path to project directory",
				"required":    true,
			},
			"project_id": map[string]interface{}{
				"type":        "string",
				"description": "Project identifier (directory name)",
				"required":    true,
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Project status (in_progress, completed, abandoned, etc.)",
				"required":    true,
			},
			"current_phase": map[string]interface{}{
				"type":        "string",
				"description": "Current phase in Wayfinder workflow (D1, D2, S4, etc.)",
				"required":    false,
			},
			"depth": map[string]interface{}{
				"type":        "string",
				"description": "Project depth tier (XS, S, M, L, XL)",
				"required":    false,
			},
			"started_at": map[string]interface{}{
				"type":        "string",
				"description": "Project start timestamp (RFC3339 format)",
				"required":    true,
			},
			"updated_at": map[string]interface{}{
				"type":        "string",
				"description": "Last update timestamp (RFC3339 format)",
				"required":    true,
			},
		},
	}
}

// GetPhaseSchema returns the schema definition for Wayfinder phases
func GetPhaseSchema() map[string]interface{} {
	return map[string]interface{}{
		"component": "wayfinder",
		"version":   "1.0.0",
		"entity":    "phase",
		"fields": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Session this phase belongs to",
				"required":    true,
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Workspace isolation field",
				"required":    true,
				"indexed":     true,
			},
			"phase_name": map[string]interface{}{
				"type":        "string",
				"description": "Phase identifier (D1, D2, D3, D4, S4, S5, S6, S7, S8, S9, S10, S11)",
				"required":    true,
			},
			"phase_title": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable phase title",
				"required":    false,
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Phase status (not_started, in_progress, completed, blocked)",
				"required":    true,
			},
			"started_at": map[string]interface{}{
				"type":        "string",
				"description": "Phase start timestamp (RFC3339 format)",
				"required":    false,
			},
			"completed_at": map[string]interface{}{
				"type":        "string",
				"description": "Phase completion timestamp (RFC3339 format)",
				"required":    false,
			},
			"deliverables": map[string]interface{}{
				"type":        "array",
				"description": "List of deliverable files for this phase",
				"required":    false,
			},
		},
	}
}

// GetValidationSchema returns the schema definition for Wayfinder validation results
func GetValidationSchema() map[string]interface{} {
	return map[string]interface{}{
		"component": "wayfinder",
		"version":   "1.0.0",
		"entity":    "validation",
		"fields": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Session this validation belongs to",
				"required":    true,
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Workspace isolation field",
				"required":    true,
				"indexed":     true,
			},
			"phase_name": map[string]interface{}{
				"type":        "string",
				"description": "Phase being validated",
				"required":    true,
			},
			"validation_type": map[string]interface{}{
				"type":        "string",
				"description": "Type of validation (frontmatter, deliverable, signature, etc.)",
				"required":    true,
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Validation result (passed, failed, warning)",
				"required":    true,
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Validation message or error details",
				"required":    false,
			},
			"timestamp": map[string]interface{}{
				"type":        "string",
				"description": "Validation timestamp (RFC3339 format)",
				"required":    true,
			},
		},
	}
}

// GetAllSchemas returns all Wayfinder corpus callosum schemas
func GetAllSchemas() []map[string]interface{} {
	return []map[string]interface{}{
		GetWayfinderSchema(),
		GetPhaseSchema(),
		GetValidationSchema(),
	}
}
