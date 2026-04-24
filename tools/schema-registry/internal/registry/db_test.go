package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Use a test workspace
	workspace := "test-" + t.Name()
	defer cleanupTestRegistry(workspace)

	reg, err := New(workspace)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	if reg.GetWorkspace() != workspace {
		t.Errorf("Expected workspace %s, got %s", workspace, reg.GetWorkspace())
	}

	if reg.GetPath() == "" {
		t.Error("Expected non-empty registry path")
	}
}

func TestRegisterSchema(t *testing.T) {
	workspace := "test-" + t.Name()
	defer cleanupTestRegistry(workspace)

	reg, err := New(workspace)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	schema := map[string]interface{}{
		"$schema":       "https://corpus-callosum.dev/schema/v1",
		"component":     "test-component",
		"version":       "1.0.0",
		"compatibility": "backward",
		"description":   "Test component",
		"schemas": map[string]interface{}{
			"test-schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}

	err = reg.RegisterSchema("test-component", "1.0.0", "backward", schema)
	if err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Verify it was registered
	retrieved, err := reg.GetSchema("test-component", "1.0.0")
	if err != nil {
		t.Fatalf("Failed to get schema: %v", err)
	}

	if retrieved.Component != "test-component" {
		t.Errorf("Expected component test-component, got %s", retrieved.Component)
	}
	if retrieved.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", retrieved.Version)
	}
}

func TestGetSchemaLatest(t *testing.T) {
	workspace := "test-" + t.Name()
	defer cleanupTestRegistry(workspace)

	reg, err := New(workspace)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	schema := map[string]interface{}{
		"$schema":   "https://corpus-callosum.dev/schema/v1",
		"component": "test-component",
		"version":   "1.0.0",
		"schemas":   map[string]interface{}{},
	}

	// Register v1.0.0
	err = reg.RegisterSchema("test-component", "1.0.0", "backward", schema)
	if err != nil {
		t.Fatalf("Failed to register v1.0.0: %v", err)
	}

	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	// Register v1.1.0
	schema["version"] = "1.1.0"
	err = reg.RegisterSchema("test-component", "1.1.0", "backward", schema)
	if err != nil {
		t.Fatalf("Failed to register v1.1.0: %v", err)
	}

	// Get latest (should be v1.1.0)
	latest, err := reg.GetSchema("test-component", "")
	if err != nil {
		t.Fatalf("Failed to get latest schema: %v", err)
	}

	if latest.Version != "1.1.0" {
		t.Errorf("Expected latest version 1.1.0, got %s", latest.Version)
	}
}

func TestListComponents(t *testing.T) {
	workspace := "test-" + t.Name()
	defer cleanupTestRegistry(workspace)

	reg, err := New(workspace)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Register two components
	schema1 := map[string]interface{}{
		"$schema":   "https://corpus-callosum.dev/schema/v1",
		"component": "comp1",
		"version":   "1.0.0",
		"schemas":   map[string]interface{}{},
	}
	schema2 := map[string]interface{}{
		"$schema":   "https://corpus-callosum.dev/schema/v1",
		"component": "comp2",
		"version":   "1.0.0",
		"schemas":   map[string]interface{}{},
	}

	reg.RegisterSchema("comp1", "1.0.0", "backward", schema1)
	reg.RegisterSchema("comp2", "1.0.0", "backward", schema2)

	components, err := reg.ListComponents()
	if err != nil {
		t.Fatalf("Failed to list components: %v", err)
	}

	if len(components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(components))
	}
}

func TestUnregisterSchema(t *testing.T) {
	workspace := "test-" + t.Name()
	defer cleanupTestRegistry(workspace)

	reg, err := New(workspace)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	schema := map[string]interface{}{
		"$schema":   "https://corpus-callosum.dev/schema/v1",
		"component": "test-component",
		"version":   "1.0.0",
		"schemas":   map[string]interface{}{},
	}

	// Register
	err = reg.RegisterSchema("test-component", "1.0.0", "backward", schema)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Unregister
	err = reg.UnregisterSchema("test-component", "1.0.0")
	if err != nil {
		t.Fatalf("Failed to unregister: %v", err)
	}

	// Verify it's gone
	_, err = reg.GetSchema("test-component", "1.0.0")
	if err == nil {
		t.Error("Expected error getting unregistered schema")
	}
}

func TestDetectWorkspace(t *testing.T) {
	workspace := DetectWorkspace()
	if workspace == "" {
		t.Error("Expected non-empty workspace")
	}
}

func cleanupTestRegistry(workspace string) {
	homeDir, _ := os.UserHomeDir()
	registryPath := filepath.Join(homeDir, ".config", "corpus-callosum", workspace)
	os.RemoveAll(registryPath)
}
