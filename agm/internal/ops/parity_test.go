package ops

import (
	"encoding/json"
	"testing"
)

// TestSurfaceParity verifies that the ops registry covers all expected operations.
// This is the contract test that ensures CLI, MCP, and Skills all expose the same functionality.
func TestSurfaceParity_AllOpsRegistered(t *testing.T) {
	result := ListOps()
	if result == nil {
		t.Fatal("ListOps returned nil")
	}
	if result.Total == 0 {
		t.Fatal("ListOps returned 0 operations — registry is empty")
	}

	// Every operation must have name, description, category, and surface
	for _, op := range result.Operations {
		if op.Name == "" {
			t.Error("operation has empty name")
		}
		if op.Description == "" {
			t.Errorf("operation %q has empty description", op.Name)
		}
		if op.Category == "" {
			t.Errorf("operation %q has empty category", op.Name)
		}
		if op.Surface == "" {
			t.Errorf("operation %q has empty surface", op.Name)
		}

		// Category must be one of: read, mutation, meta
		switch op.Category {
		case "read", "mutation", "meta":
			// valid
		default:
			t.Errorf("operation %q has invalid category %q (expected read, mutation, or meta)", op.Name, op.Category)
		}
	}
}

// TestSurfaceParity_CoreOpsExist verifies core operations are registered.
func TestSurfaceParity_CoreOpsExist(t *testing.T) {
	result := ListOps()
	opsMap := make(map[string]OpInfo)
	for _, op := range result.Operations {
		opsMap[op.Name] = op
	}

	coreOps := []string{
		"list_sessions",
		"get_session",
		"search_sessions",
		"get_status",
		"list_workspaces",
	}

	for _, name := range coreOps {
		if _, ok := opsMap[name]; !ok {
			t.Errorf("core operation %q missing from ops registry", name)
		}
	}
}

// TestSurfaceParity_ReadOpsAreRead verifies read operations are categorized correctly.
func TestSurfaceParity_ReadOpsAreRead(t *testing.T) {
	result := ListOps()
	readOps := []string{"list_sessions", "get_session", "search_sessions", "get_status", "list_workspaces"}

	opsMap := make(map[string]OpInfo)
	for _, op := range result.Operations {
		opsMap[op.Name] = op
	}

	for _, name := range readOps {
		op, ok := opsMap[name]
		if !ok {
			continue // covered by CoreOpsExist test
		}
		if op.Category != "read" {
			t.Errorf("operation %q should be category 'read', got %q", name, op.Category)
		}
	}
}

// TestErrorFormat_RFC7807Compliance verifies all error constructors produce valid RFC 7807.
func TestErrorFormat_RFC7807Compliance(t *testing.T) {
	errors := []*OpError{
		ErrSessionNotFound("test"),
		ErrSessionArchived("test"),
		ErrInvalidInput("field", "detail"),
		ErrStorageError("op", nil),
		ErrTmuxNotRunning(),
	}

	requiredFields := []string{"status", "type", "code", "title", "detail"}

	for _, err := range errors {
		data := err.JSON()
		var parsed map[string]interface{}
		if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
			t.Errorf("error %s produces invalid JSON: %v", err.Code, jsonErr)
			continue
		}

		for _, field := range requiredFields {
			if _, ok := parsed[field]; !ok {
				t.Errorf("error %s missing RFC 7807 field %q", err.Code, field)
			}
		}

		// Suggestions should always be present
		suggestions, ok := parsed["suggestions"]
		if !ok {
			t.Errorf("error %s missing suggestions", err.Code)
		} else if arr, ok := suggestions.([]interface{}); !ok || len(arr) == 0 {
			t.Errorf("error %s has empty suggestions", err.Code)
		}
	}
}

// TestErrorCodes_Unique verifies all error codes are unique.
func TestErrorCodes_Unique(t *testing.T) {
	codes := []string{
		ErrCodeSessionNotFound,
		ErrCodeSessionArchived,
		ErrCodeTmuxNotRunning,
		ErrCodeDoltUnavailable,
		ErrCodeInvalidInput,
		ErrCodePermissionDenied,
		ErrCodeSessionExists,
		ErrCodeHarnessUnavailable,
		ErrCodeWorkspaceNotFound,
		ErrCodeUUIDNotAssociated,
		ErrCodeStorageError,
		ErrCodeDryRun,
	}

	seen := make(map[string]bool)
	for _, code := range codes {
		if seen[code] {
			t.Errorf("duplicate error code: %s", code)
		}
		seen[code] = true
	}
}

// TestFieldMask_WithResultStruct verifies field mask works on ops result types.
func TestFieldMask_WithResultStruct(t *testing.T) {
	result := &ListSessionsResult{
		Operation: "list_sessions",
		Sessions:  []SessionSummary{{ID: "1", Name: "test", Status: "active"}},
		Total:     1,
		Limit:     100,
		Offset:    0,
	}

	// Filter to just operation and total
	filtered, err := ApplyFieldMask(result, []string{"operation", "total"})
	if err != nil {
		t.Fatalf("field mask error: %v", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(filtered, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("expected 2 fields, got %d", len(parsed))
	}
	if _, ok := parsed["operation"]; !ok {
		t.Error("missing 'operation' field")
	}
	if _, ok := parsed["sessions"]; ok {
		t.Error("'sessions' field should be filtered out")
	}
}
