package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.yaml")

	// Create test registry
	registry := &Registry{
		Version:          1,
		ProtocolVersion:  "1.0.0",
		DefaultWorkspace: "test",
		DefaultSettings: map[string]interface{}{
			"log_level": "info",
		},
		Workspaces: []Workspace{
			{Name: "test", Root: tmpDir, Enabled: true},
		},
	}

	// Save registry
	if err := SaveRegistry(registryPath, registry); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Load registry
	loaded, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	// Verify
	if loaded.Version != 1 {
		t.Errorf("Expected version 1, got %d", loaded.Version)
	}
	if loaded.ProtocolVersion != "1.0.0" {
		t.Errorf("Expected protocol version 1.0.0, got %s", loaded.ProtocolVersion)
	}
	if loaded.DefaultWorkspace != "test" {
		t.Errorf("Expected default workspace 'test', got '%s'", loaded.DefaultWorkspace)
	}
	if len(loaded.Workspaces) != 1 {
		t.Errorf("Expected 1 workspace, got %d", len(loaded.Workspaces))
	}
}

func TestValidateRegistry(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		registry  *Registry
		expectErr bool
	}{
		{
			name: "valid registry",
			registry: &Registry{
				Version: 1,
				Workspaces: []Workspace{
					{Name: "test", Root: tmpDir, Enabled: true},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid version",
			registry: &Registry{
				Version: 2,
				Workspaces: []Workspace{
					{Name: "test", Root: tmpDir, Enabled: true},
				},
			},
			expectErr: true,
		},
		{
			name: "no workspaces",
			registry: &Registry{
				Version:    1,
				Workspaces: []Workspace{},
			},
			expectErr: false,
		},
		{
			name: "duplicate workspace names",
			registry: &Registry{
				Version: 1,
				Workspaces: []Workspace{
					{Name: "test", Root: tmpDir, Enabled: true},
					{Name: "test", Root: tmpDir, Enabled: true},
				},
			},
			expectErr: true,
		},
		{
			name: "no enabled workspaces",
			registry: &Registry{
				Version: 1,
				Workspaces: []Workspace{
					{Name: "test", Root: tmpDir, Enabled: false},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegistry(tt.registry)
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestInitializeRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.yaml")

	if err := InitializeRegistry(registryPath); err != nil {
		t.Fatalf("Failed to initialize registry: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Fatal("Registry file was not created")
	}

	// Load and verify
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("Failed to load initialized registry: %v", err)
	}

	if registry.Version != 1 {
		t.Errorf("Expected version 1, got %d", registry.Version)
	}
	if registry.ProtocolVersion != "1.0.0" {
		t.Errorf("Expected protocol version 1.0.0, got %s", registry.ProtocolVersion)
	}
}

func TestRegistryAddRemoveWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	registry := &Registry{
		Version:    1,
		Workspaces: []Workspace{},
	}

	// Add workspace
	ws := Workspace{
		Name:    "test",
		Root:    tmpDir,
		Enabled: true,
	}

	if err := registry.AddWorkspace(ws); err != nil {
		t.Fatalf("Failed to add workspace: %v", err)
	}

	if len(registry.Workspaces) != 1 {
		t.Errorf("Expected 1 workspace, got %d", len(registry.Workspaces))
	}

	// Try to add duplicate
	err := registry.AddWorkspace(ws)
	if err == nil {
		t.Error("Expected error when adding duplicate workspace")
	}

	// Remove workspace
	if err := registry.RemoveWorkspace("test"); err != nil {
		t.Fatalf("Failed to remove workspace: %v", err)
	}

	if len(registry.Workspaces) != 0 {
		t.Errorf("Expected 0 workspaces, got %d", len(registry.Workspaces))
	}

	// Try to remove non-existent
	err = registry.RemoveWorkspace("nonexistent")
	if err == nil {
		t.Error("Expected error when removing non-existent workspace")
	}
}

func TestGetWorkspaceByName(t *testing.T) {
	tmpDir := t.TempDir()

	registry := &Registry{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "enabled", Root: tmpDir, Enabled: true},
			{Name: "disabled", Root: tmpDir, Enabled: false},
		},
	}

	// Get enabled workspace
	ws, err := registry.GetWorkspaceByName("enabled")
	if err != nil {
		t.Fatalf("Failed to get enabled workspace: %v", err)
	}
	if ws.Name != "enabled" {
		t.Errorf("Expected workspace 'enabled', got '%s'", ws.Name)
	}

	// Get disabled workspace (should fail)
	_, err = registry.GetWorkspaceByName("disabled")
	if err == nil {
		t.Error("Expected error when getting disabled workspace")
	}

	// Get non-existent workspace
	_, err = registry.GetWorkspaceByName("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent workspace")
	}
}
