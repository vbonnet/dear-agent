package plugin

import (
	"testing"
	"time"
)

// Mock plugin for testing
type mockPlugin struct {
	name            string
	supportsSession bool
	tasks           []Task
	phaseStats      []PhaseStats
	getTasksError   error
	getPhaseError   error
}

func (m *mockPlugin) Metadata() PluginMetadata {
	return PluginMetadata{
		Name:        m.name,
		Version:     "1.0.0",
		Author:      "Test Author",
		Description: "Mock plugin for testing",
	}
}

func (m *mockPlugin) GetTasks(sessionDir string) ([]Task, error) {
	if m.getTasksError != nil {
		return nil, m.getTasksError
	}
	return m.tasks, nil
}

func (m *mockPlugin) GetPhaseProgress(sessionDir string) ([]PhaseStats, error) {
	if m.getPhaseError != nil {
		return nil, m.getPhaseError
	}
	return m.phaseStats, nil
}

func (m *mockPlugin) SupportsSession(sessionDir string) bool {
	return m.supportsSession
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if registry.plugins == nil {
		t.Fatal("Registry plugins map is nil")
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name        string
		plugins     []*mockPlugin
		wantErr     bool
		errContains string
	}{
		{
			name: "register single plugin",
			plugins: []*mockPlugin{
				{name: "plugin1", supportsSession: true},
			},
			wantErr: false,
		},
		{
			name: "register multiple plugins",
			plugins: []*mockPlugin{
				{name: "plugin1", supportsSession: true},
				{name: "plugin2", supportsSession: true},
			},
			wantErr: false,
		},
		{
			name: "register duplicate plugin",
			plugins: []*mockPlugin{
				{name: "duplicate", supportsSession: true},
				{name: "duplicate", supportsSession: true},
			},
			wantErr:     true,
			errContains: "already registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()

			var lastErr error
			for _, plugin := range tt.plugins {
				err := registry.Register(plugin)
				if err != nil {
					lastErr = err
				}
			}

			if tt.wantErr {
				if lastErr == nil {
					t.Errorf("Register() expected error, got nil")
				} else if tt.errContains != "" && !contains(lastErr.Error(), tt.errContains) {
					t.Errorf("Register() error = %v, want error containing %q", lastErr, tt.errContains)
				}
			} else {
				if lastErr != nil {
					t.Errorf("Register() unexpected error: %v", lastErr)
				}
			}
		})
	}
}

func TestGet(t *testing.T) {
	registry := NewRegistry()

	plugin1 := &mockPlugin{name: "plugin1", supportsSession: true}
	plugin2 := &mockPlugin{name: "plugin2", supportsSession: true}

	registry.Register(plugin1)
	registry.Register(plugin2)

	tests := []struct {
		name       string
		pluginName string
		wantNil    bool
	}{
		{
			name:       "get existing plugin",
			pluginName: "plugin1",
			wantNil:    false,
		},
		{
			name:       "get non-existent plugin",
			pluginName: "nonexistent",
			wantNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registry.Get(tt.pluginName)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Get(%q) = %v, want nil", tt.pluginName, got)
				}
			} else {
				if got == nil {
					t.Errorf("Get(%q) = nil, want non-nil", tt.pluginName)
				}
			}
		})
	}
}

func TestList(t *testing.T) {
	registry := NewRegistry()

	// Empty registry
	names := registry.List()
	if len(names) != 0 {
		t.Errorf("List() on empty registry = %v, want empty slice", names)
	}

	// Add plugins
	registry.Register(&mockPlugin{name: "plugin1"})
	registry.Register(&mockPlugin{name: "plugin2"})
	registry.Register(&mockPlugin{name: "plugin3"})

	names = registry.List()
	if len(names) != 3 {
		t.Errorf("List() returned %d plugins, want 3", len(names))
	}

	// Verify all plugin names are present
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	for _, expected := range []string{"plugin1", "plugin2", "plugin3"} {
		if !nameMap[expected] {
			t.Errorf("List() missing plugin %q", expected)
		}
	}
}

func TestAutoDetect(t *testing.T) {
	registry := NewRegistry()

	plugin1 := &mockPlugin{name: "plugin1", supportsSession: false}
	plugin2 := &mockPlugin{name: "plugin2", supportsSession: true}
	plugin3 := &mockPlugin{name: "plugin3", supportsSession: false}

	registry.Register(plugin1)
	registry.Register(plugin2)
	registry.Register(plugin3)

	// Should return plugin2 (first that supports)
	detected := registry.AutoDetect("/test/session")
	if detected == nil {
		t.Fatal("AutoDetect() returned nil, expected plugin2")
	}

	if detected.Metadata().Name != "plugin2" {
		t.Errorf("AutoDetect() returned %q, want %q", detected.Metadata().Name, "plugin2")
	}
}

func TestAutoDetect_NoMatch(t *testing.T) {
	registry := NewRegistry()

	plugin1 := &mockPlugin{name: "plugin1", supportsSession: false}
	plugin2 := &mockPlugin{name: "plugin2", supportsSession: false}

	registry.Register(plugin1)
	registry.Register(plugin2)

	// Should return nil (no plugin supports)
	detected := registry.AutoDetect("/test/session")
	if detected != nil {
		t.Errorf("AutoDetect() = %v, want nil (no supporting plugins)", detected)
	}
}

func TestGetGlobalRegistry(t *testing.T) {
	global := GetGlobalRegistry()
	if global == nil {
		t.Fatal("GetGlobalRegistry() returned nil")
	}

	// Should return same instance
	global2 := GetGlobalRegistry()
	if global != global2 {
		t.Error("GetGlobalRegistry() returned different instances, want singleton")
	}
}

func TestTask_Structure(t *testing.T) {
	// Verify Task struct can be instantiated
	task := Task{
		ID:          "test-123",
		Title:       "Test Task",
		Description: "Test description",
		Status:      "in_progress",
		Phase:       "phase-0",
		Labels:      []string{"label1", "label2"},
		Metadata:    map[string]string{"key": "value"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if task.ID != "test-123" {
		t.Errorf("Task.ID = %q, want %q", task.ID, "test-123")
	}
	if task.Status != "in_progress" {
		t.Errorf("Task.Status = %q, want %q", task.Status, "in_progress")
	}
}

func TestPhaseStats_Structure(t *testing.T) {
	// Verify PhaseStats struct can be instantiated
	stats := PhaseStats{
		Phase:      "phase-0",
		Total:      10,
		Pending:    3,
		InProgress: 2,
		Completed:  5,
		Blocked:    0,
		Cancelled:  0,
		Percentage: 50.0,
	}

	if stats.Phase != "phase-0" {
		t.Errorf("PhaseStats.Phase = %q, want %q", stats.Phase, "phase-0")
	}
	if stats.Percentage != 50.0 {
		t.Errorf("PhaseStats.Percentage = %f, want %f", stats.Percentage, 50.0)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && (s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
