package corpus

import (
	"testing"
)

// TestGetWayfinderSchema tests the Wayfinder project schema definition
func TestGetWayfinderSchema(t *testing.T) {
	schema := GetWayfinderSchema()

	// Verify required fields
	if schema["component"] != "wayfinder" {
		t.Errorf("Expected component 'wayfinder', got '%v'", schema["component"])
	}

	if schema["version"] != "2.0.0" {
		t.Errorf("Expected version '2.0.0', got '%v'", schema["version"])
	}

	if schema["entity"] != "project" {
		t.Errorf("Expected entity 'project', got '%v'", schema["entity"])
	}

	// Verify fields exist
	fields, ok := schema["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema fields not found or invalid type")
	}

	// Verify critical fields
	requiredFields := []string{"schema_version", "workspace", "project_path", "project_name", "project_type", "risk_level", "current_waypoint", "status", "created_at", "updated_at"}
	for _, fieldName := range requiredFields {
		if _, exists := fields[fieldName]; !exists {
			t.Errorf("Required field '%s' missing from schema", fieldName)
		}
	}
	for _, retired := range []string{"session_id", "project_id", "current_phase", "depth", "started_at"} {
		if _, exists := fields[retired]; exists {
			t.Errorf("Retired project field %q remains in canonical schema", retired)
		}
	}

	// Verify workspace field is indexed
	workspaceField, ok := fields["workspace"].(map[string]interface{})
	if !ok {
		t.Fatal("Workspace field not found or invalid type")
	}

	if indexed, ok := workspaceField["indexed"].(bool); !ok || !indexed {
		t.Error("Workspace field should be indexed")
	}

	// Verify workspace field is required
	if required, ok := workspaceField["required"].(bool); !ok || !required {
		t.Error("Workspace field should be required")
	}
}

// TestGetPhaseSchema tests the Wayfinder phase schema definition
func TestGetPhaseSchema(t *testing.T) {
	schema := GetPhaseSchema()

	if schema["component"] != "wayfinder" {
		t.Errorf("Expected component 'wayfinder', got '%v'", schema["component"])
	}

	if schema["entity"] != "phase" {
		t.Errorf("Expected entity 'phase', got '%v'", schema["entity"])
	}

	fields, ok := schema["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema fields not found")
	}

	// Verify phase-specific fields
	phaseFields := []string{"project_name", "workspace", "name", "status"}
	for _, fieldName := range phaseFields {
		if _, exists := fields[fieldName]; !exists {
			t.Errorf("Required field '%s' missing from phase schema", fieldName)
		}
	}

	// Verify workspace field properties
	workspaceField := fields["workspace"].(map[string]interface{})
	if indexed, ok := workspaceField["indexed"].(bool); !ok || !indexed {
		t.Error("Workspace field should be indexed in phase schema")
	}
}

// TestGetValidationSchema tests the Wayfinder validation schema definition
func TestGetValidationSchema(t *testing.T) {
	schema := GetValidationSchema()

	if schema["component"] != "wayfinder" {
		t.Errorf("Expected component 'wayfinder', got '%v'", schema["component"])
	}

	if schema["entity"] != "validation" {
		t.Errorf("Expected entity 'validation', got '%v'", schema["entity"])
	}

	fields, ok := schema["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema fields not found")
	}

	// Verify validation-specific fields
	validationFields := []string{"project_name", "workspace", "waypoint_name", "validation_type", "status", "timestamp"}
	for _, fieldName := range validationFields {
		if _, exists := fields[fieldName]; !exists {
			t.Errorf("Required field '%s' missing from validation schema", fieldName)
		}
	}

	// Verify workspace isolation
	workspaceField := fields["workspace"].(map[string]interface{})
	if indexed, ok := workspaceField["indexed"].(bool); !ok || !indexed {
		t.Error("Workspace field should be indexed in validation schema")
	}
}

// TestGetAllSchemas tests that all schemas are returned
func TestGetAllSchemas(t *testing.T) {
	schemas := GetAllSchemas()

	if len(schemas) != 3 {
		t.Errorf("Expected 3 schemas, got %d", len(schemas))
	}

	// Verify all schemas have required fields
	entities := make(map[string]bool)
	for _, schema := range schemas {
		if component, ok := schema["component"].(string); !ok || component != "wayfinder" {
			t.Error("All schemas should have component='wayfinder'")
		}

		if entity, ok := schema["entity"].(string); ok {
			entities[entity] = true
		}
	}

	// Verify we have all expected entities
	expectedEntities := []string{"project", "phase", "validation"}
	for _, expected := range expectedEntities {
		if !entities[expected] {
			t.Errorf("Missing schema for entity '%s'", expected)
		}
	}
}

// TestSchemasSupportWorkspaceIsolation verifies all schemas include workspace field
func TestSchemasSupportWorkspaceIsolation(t *testing.T) {
	schemas := GetAllSchemas()

	for _, schema := range schemas {
		entity, _ := schema["entity"].(string)
		fields, ok := schema["fields"].(map[string]interface{})
		if !ok {
			t.Fatalf("Schema for entity '%s' has no fields", entity)
		}

		workspaceField, exists := fields["workspace"]
		if !exists {
			t.Errorf("Schema for entity '%s' missing workspace field", entity)
			continue
		}

		workspaceProps, ok := workspaceField.(map[string]interface{})
		if !ok {
			t.Errorf("Workspace field for entity '%s' has invalid structure", entity)
			continue
		}

		// Verify workspace is required
		if required, ok := workspaceProps["required"].(bool); !ok || !required {
			t.Errorf("Workspace field for entity '%s' should be required", entity)
		}

		// Verify workspace is indexed
		if indexed, ok := workspaceProps["indexed"].(bool); !ok || !indexed {
			t.Errorf("Workspace field for entity '%s' should be indexed", entity)
		}
	}
}

// TestCorpusCallosumBin_DefaultIsNotCC verifies that the default binary name
// is NOT "cc", which collides with the macOS/Linux C compiler (clang/gcc).
// ce-8tjw: clang was being invoked when corpus-callosum wasn't installed.
func TestCorpusCallosumBin_DefaultIsNotCC(t *testing.T) {
	t.Setenv("CORPUS_CALLOSUM_BIN", "")
	bin := corpusCallosumBin()
	if bin == "cc" {
		t.Error("default binary must not be 'cc' — it collides with the C compiler on macOS/Linux")
	}
}

// TestCorpusCallosumBin_EnvVarOverride verifies CORPUS_CALLOSUM_BIN is respected.
func TestCorpusCallosumBin_EnvVarOverride(t *testing.T) {
	t.Setenv("CORPUS_CALLOSUM_BIN", "/usr/local/bin/my-cc")
	bin := corpusCallosumBin()
	if bin != "/usr/local/bin/my-cc" {
		t.Errorf("expected env override, got %q", bin)
	}
}

// TestIsCorpusCallosumAvailable tests the availability check.
// After the ce-8tjw fix, the default binary is "corpus-callosum" (not "cc"),
// which is not installed here — so the function should return false and all
// callers gracefully degrade.
func TestIsCorpusCallosumAvailable(t *testing.T) {
	t.Setenv("CORPUS_CALLOSUM_BIN", "")
	// On this machine, "corpus-callosum" is not installed.
	available := isCorpusCallosumAvailable()
	if available {
		t.Log("corpus-callosum binary found on PATH — skipping not-installed assertion")
		return
	}
	t.Log("corpus-callosum not installed; isCorpusCallosumAvailable correctly returns false")
}

// TestRegisterWayfinderSchemas_GracefulDegradation tests graceful degradation
func TestRegisterWayfinderSchemas_GracefulDegradation(t *testing.T) {
	// Should not error even if cc is not available
	err := RegisterWayfinderSchemas("test-workspace")
	if err != nil {
		t.Errorf("RegisterWayfinderSchemas should not error even if cc unavailable: %v", err)
	}
}

// TestUnregisterWayfinderSchemas_GracefulDegradation tests graceful degradation
func TestUnregisterWayfinderSchemas_GracefulDegradation(t *testing.T) {
	// Should not error even if cc is not available
	err := UnregisterWayfinderSchemas("test-workspace")
	if err != nil {
		t.Errorf("UnregisterWayfinderSchemas should not error even if cc unavailable: %v", err)
	}
}

// TestGetRegistrationStatus_GracefulDegradation tests graceful degradation
func TestGetRegistrationStatus_GracefulDegradation(t *testing.T) {
	// Should return empty list if cc not available
	entities, err := GetRegistrationStatus("test-workspace")
	if err != nil {
		t.Errorf("GetRegistrationStatus should not error: %v", err)
	}

	// Should return empty list (cc not available or not registered)
	if entities == nil {
		t.Error("GetRegistrationStatus should return empty list, not nil")
	}
}

// TestPublishProject_GracefulDegradation tests graceful degradation
func TestPublishProject_GracefulDegradation(t *testing.T) {
	project := map[string]interface{}{
		"project_name": "test-project",
		"project_path": "/tmp/test-project",
		"status":       "in-progress",
	}

	err := PublishProject("test-workspace", project)
	if err != nil {
		t.Errorf("PublishProject should not error even if cc unavailable: %v", err)
	}
}

// TestPublishPhase_GracefulDegradation tests graceful degradation
func TestPublishPhase_GracefulDegradation(t *testing.T) {
	phase := map[string]interface{}{
		"project_name": "test-project",
		"name":         "PROBLEM",
		"status":       "completed",
	}

	err := PublishPhase("test-workspace", phase)
	if err != nil {
		t.Errorf("PublishPhase should not error even if cc unavailable: %v", err)
	}
}

// TestPublishValidation_GracefulDegradation tests graceful degradation
func TestPublishValidation_GracefulDegradation(t *testing.T) {
	validation := map[string]interface{}{
		"project_name":    "test-project",
		"waypoint_name":   "PROBLEM",
		"validation_type": "frontmatter",
		"status":          "passed",
	}

	err := PublishValidation("test-workspace", validation)
	if err != nil {
		t.Errorf("PublishValidation should not error even if cc unavailable: %v", err)
	}
}

// TestQueryAGMSessions_GracefulDegradation tests graceful degradation
func TestQueryAGMSessions_GracefulDegradation(t *testing.T) {
	sessions, err := QueryAGMSessions("test-workspace", nil)
	if err != nil {
		t.Errorf("QueryAGMSessions should not error: %v", err)
	}

	if sessions == nil {
		t.Error("QueryAGMSessions should return empty list, not nil")
	}
}

// TestGetCurrentAGMSession_GracefulDegradation tests graceful degradation
func TestGetCurrentAGMSession_GracefulDegradation(t *testing.T) {
	session, err := GetCurrentAGMSession("test-workspace")
	if err != nil {
		t.Errorf("GetCurrentAGMSession should not error: %v", err)
	}

	// Should return nil if no session found
	t.Logf("Current AGM session: %v", session)
}

// TestQueryEngramBeads_GracefulDegradation tests graceful degradation
func TestQueryEngramBeads_GracefulDegradation(t *testing.T) {
	beads, err := QueryEngramBeads("test-workspace", nil)
	if err != nil {
		t.Errorf("QueryEngramBeads should not error: %v", err)
	}

	if beads == nil {
		t.Error("QueryEngramBeads should return empty list, not nil")
	}
}

// TestGetBeadsBySession_GracefulDegradation tests graceful degradation
func TestGetBeadsBySession_GracefulDegradation(t *testing.T) {
	beads, err := GetBeadsBySession("test-workspace", "test-session")
	if err != nil {
		t.Errorf("GetBeadsBySession should not error: %v", err)
	}

	if beads == nil {
		t.Error("GetBeadsBySession should return empty list, not nil")
	}
}

// TestGetOpenBeads_GracefulDegradation tests graceful degradation
func TestGetOpenBeads_GracefulDegradation(t *testing.T) {
	beads, err := GetOpenBeads("test-workspace")
	if err != nil {
		t.Errorf("GetOpenBeads should not error: %v", err)
	}

	if beads == nil {
		t.Error("GetOpenBeads should return empty list, not nil")
	}
}

// TestQueryWayfinderProjects_GracefulDegradation tests graceful degradation
func TestQueryWayfinderProjects_GracefulDegradation(t *testing.T) {
	projects, err := QueryWayfinderProjects("test-workspace", nil)
	if err != nil {
		t.Errorf("QueryWayfinderProjects should not error: %v", err)
	}

	if projects == nil {
		t.Error("QueryWayfinderProjects should return empty list, not nil")
	}
}

// TestGetProjectByName_GracefulDegradation tests graceful degradation.
func TestGetProjectByName_GracefulDegradation(t *testing.T) {
	project, err := GetProjectByName("test-workspace", "test-project")
	if err != nil {
		t.Errorf("GetProjectByName should not error: %v", err)
	}

	// Can be nil if project not found
	t.Logf("Project: %v", project)
}

// TestGetActiveProjects_GracefulDegradation tests graceful degradation
func TestGetActiveProjects_GracefulDegradation(t *testing.T) {
	projects, err := GetActiveProjects("test-workspace")
	if err != nil {
		t.Errorf("GetActiveProjects should not error: %v", err)
	}

	if projects == nil {
		t.Error("GetActiveProjects should return empty list, not nil")
	}
}

// TestQueryPhases_GracefulDegradation tests graceful degradation
func TestQueryPhases_GracefulDegradation(t *testing.T) {
	phases, err := QueryPhases("test-workspace", "test-session")
	if err != nil {
		t.Errorf("QueryPhases should not error: %v", err)
	}

	if phases == nil {
		t.Error("QueryPhases should return empty list, not nil")
	}
}

// TestGetCurrentPhase_GracefulDegradation tests graceful degradation
func TestGetCurrentPhase_GracefulDegradation(t *testing.T) {
	phase, err := GetCurrentPhase("test-workspace", "test-session")
	if err != nil {
		t.Errorf("GetCurrentPhase should not error: %v", err)
	}

	// Can be nil if no current phase
	t.Logf("Current phase: %v", phase)
}

// TestCrossComponentQuery_GracefulDegradation tests graceful degradation
func TestCrossComponentQuery_GracefulDegradation(t *testing.T) {
	result, err := CrossComponentQuery("test-workspace")
	if err != nil {
		t.Errorf("CrossComponentQuery should not error: %v", err)
	}

	if result == nil {
		t.Error("CrossComponentQuery should return result map, not nil")
	}

	// Should have keys even if values are empty
	t.Logf("Cross-component query result: %v", result)
}

// TestDiscoverComponents_GracefulDegradation tests graceful degradation
func TestDiscoverComponents_GracefulDegradation(t *testing.T) {
	components, err := DiscoverComponents("test-workspace")
	if err != nil {
		t.Errorf("DiscoverComponents should not error: %v", err)
	}

	if components == nil {
		t.Error("DiscoverComponents should return empty list, not nil")
	}

	t.Logf("Discovered components: %v", components)
}

// TestWorkspaceIsolation verifies workspace field is properly set
func TestWorkspaceIsolation(t *testing.T) {
	workspace := "test-workspace"

	// Test project publication
	project := map[string]interface{}{
		"project_name": "test-project",
	}

	// Should not error
	if err := PublishProject(workspace, project); err != nil {
		t.Errorf("PublishProject failed: %v", err)
	}

	// Verify workspace was added to project
	if project["workspace"] != workspace {
		t.Errorf("Workspace field not set correctly: expected '%s', got '%v'", workspace, project["workspace"])
	}

	// Verify component and entity metadata
	if project["_component"] != "wayfinder" {
		t.Error("Component metadata not set")
	}
	if project["_entity"] != "project" {
		t.Error("Entity metadata not set")
	}
}
