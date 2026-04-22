package corpus

import (
	"encoding/json"
	"testing"
)

func TestGetEngramSchema(t *testing.T) {
	schema := GetEngramSchema()

	// Verify component metadata
	if schema["component"] != EngramComponentName {
		t.Errorf("Expected component name %q, got %q", EngramComponentName, schema["component"])
	}

	if schema["version"] != EngramComponentVersion {
		t.Errorf("Expected version %q, got %q", EngramComponentVersion, schema["version"])
	}

	// Verify schemas map exists
	schemas, ok := schema["schemas"].(map[string]interface{})
	if !ok {
		t.Fatal("schemas field is not a map")
	}

	// Verify required schemas
	requiredSchemas := []string{"bead", "memory_trace", "ecphory_result"}
	for _, name := range requiredSchemas {
		if _, exists := schemas[name]; !exists {
			t.Errorf("Required schema %q not found", name)
		}
	}

	// Verify schema is valid JSON
	_, err := json.Marshal(schema)
	if err != nil {
		t.Errorf("Schema failed to marshal to JSON: %v", err)
	}
}

func TestGetBeadSchema(t *testing.T) {
	schema := GetBeadSchema()

	// Verify it's an object
	if schemaType, ok := schema["type"].(string); !ok || schemaType != "object" {
		t.Error("Bead schema should be of type 'object'")
	}

	// Verify required properties exist
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties field is not a map")
	}

	requiredProps := []string{"id", "title", "status", "priority", "workspace"}
	for _, prop := range requiredProps {
		if _, exists := properties[prop]; !exists {
			t.Errorf("Required property %q not found in bead schema", prop)
		}
	}

	// Verify required field
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field is not a string array")
	}

	if len(required) == 0 {
		t.Error("Bead schema should have required fields")
	}

	// Verify workspace is in required fields
	hasWorkspace := false
	for _, field := range required {
		if field == "workspace" {
			hasWorkspace = true
			break
		}
	}
	if !hasWorkspace {
		t.Error("workspace should be a required field for workspace isolation")
	}

	// Verify status enum
	statusProp, ok := properties["status"].(map[string]interface{})
	if !ok {
		t.Fatal("status property is not a map")
	}

	statusEnum, ok := statusProp["enum"].([]string)
	if !ok {
		t.Fatal("status enum is not a string array")
	}

	expectedStatuses := []string{"open", "in-progress", "blocked", "closed"}
	if len(statusEnum) != len(expectedStatuses) {
		t.Errorf("Expected %d status values, got %d", len(expectedStatuses), len(statusEnum))
	}
}

func TestGetMemoryTraceSchema(t *testing.T) {
	schema := GetMemoryTraceSchema()

	// Verify it's an object
	if schemaType, ok := schema["type"].(string); !ok || schemaType != "object" {
		t.Error("Memory trace schema should be of type 'object'")
	}

	// Verify required properties
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties field is not a map")
	}

	requiredProps := []string{"trace_id", "content", "workspace"}
	for _, prop := range requiredProps {
		if _, exists := properties[prop]; !exists {
			t.Errorf("Required property %q not found in memory trace schema", prop)
		}
	}

	// Verify workspace is required
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field is not a string array")
	}

	hasWorkspace := false
	for _, field := range required {
		if field == "workspace" {
			hasWorkspace = true
			break
		}
	}
	if !hasWorkspace {
		t.Error("workspace should be required for memory trace")
	}
}

func TestGetEcphoryResultSchema(t *testing.T) {
	schema := GetEcphoryResultSchema()

	// Verify it's an object
	if schemaType, ok := schema["type"].(string); !ok || schemaType != "object" {
		t.Error("Ecphory result schema should be of type 'object'")
	}

	// Verify properties
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties field is not a map")
	}

	// Must have query, results, and workspace
	requiredProps := []string{"query", "results", "workspace"}
	for _, prop := range requiredProps {
		if _, exists := properties[prop]; !exists {
			t.Errorf("Required property %q not found in ecphory result schema", prop)
		}
	}

	// Verify workspace is required
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required field is not a string array")
	}

	hasWorkspace := false
	for _, field := range required {
		if field == "workspace" {
			hasWorkspace = true
			break
		}
	}
	if !hasWorkspace {
		t.Error("workspace should be required for ecphory results")
	}

	// Verify results is an array
	resultsProp, ok := properties["results"].(map[string]interface{})
	if !ok {
		t.Fatal("results property is not a map")
	}

	if resultsProp["type"] != "array" {
		t.Error("results should be of type array")
	}
}

func TestSchemasSupportWorkspaceIsolation(t *testing.T) {
	// All schemas must include workspace field for proper isolation
	schemas := map[string]func() map[string]interface{}{
		"bead":           GetBeadSchema,
		"memory_trace":   GetMemoryTraceSchema,
		"ecphory_result": GetEcphoryResultSchema,
	}

	for name, schemaFunc := range schemas {
		t.Run(name, func(t *testing.T) {
			schema := schemaFunc()

			properties, ok := schema["properties"].(map[string]interface{})
			if !ok {
				t.Fatalf("%s: properties field is not a map", name)
			}

			// Verify workspace property exists
			if _, exists := properties["workspace"]; !exists {
				t.Errorf("%s: missing workspace property (required for isolation)", name)
			}

			// Verify workspace is in required fields
			required, ok := schema["required"].([]string)
			if !ok {
				t.Fatalf("%s: required field is not a string array", name)
			}

			hasWorkspace := false
			for _, field := range required {
				if field == "workspace" {
					hasWorkspace = true
					break
				}
			}

			if !hasWorkspace {
				t.Errorf("%s: workspace not in required fields (breaks isolation)", name)
			}
		})
	}
}
