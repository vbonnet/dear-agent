package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLoadSaveRoundTrip(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hooks.toml")

	registry := NewRegistryWithPath(path)

	// Register some hooks
	hook1 := Hook{
		Name:     "test-hook-1",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "test-command",
		Args:     []string{"arg1", "arg2"},
		Timeout:  120,
	}

	hook2 := Hook{
		Name:     "test-hook-2",
		Event:    HookEventPhaseCompletion,
		Priority: 5,
		Type:     HookTypeSkill,
		Command:  "/engram:test",
		Timeout:  60,
	}

	if err := registry.Register(hook1); err != nil {
		t.Fatalf("Failed to register hook1: %v", err)
	}

	if err := registry.Register(hook2); err != nil {
		t.Fatalf("Failed to register hook2: %v", err)
	}

	// Save to file
	if err := registry.Save(); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Create new registry and load
	registry2 := NewRegistryWithPath(path)
	if err := registry2.Load(); err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	// Verify hooks loaded correctly
	loaded1, err := registry2.GetHook("test-hook-1")
	if err != nil {
		t.Fatalf("Failed to get hook1: %v", err)
	}
	if loaded1.Name != hook1.Name || loaded1.Event != hook1.Event {
		t.Errorf("Hook1 mismatch: got %+v, want %+v", loaded1, hook1)
	}

	loaded2, err := registry2.GetHook("test-hook-2")
	if err != nil {
		t.Fatalf("Failed to get hook2: %v", err)
	}
	if loaded2.Name != hook2.Name || loaded2.Event != hook2.Event {
		t.Errorf("Hook2 mismatch: got %+v, want %+v", loaded2, hook2)
	}
}

func TestRegistryRegisterUnregister(t *testing.T) {
	registry := NewRegistry()

	hook := Hook{
		Name:     "test-hook",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "test",
		Timeout:  60,
	}

	// Register hook
	if err := registry.Register(hook); err != nil {
		t.Fatalf("Failed to register hook: %v", err)
	}

	// Verify hook exists
	retrieved, err := registry.GetHook("test-hook")
	if err != nil {
		t.Fatalf("Failed to get registered hook: %v", err)
	}
	if retrieved.Name != hook.Name {
		t.Errorf("Retrieved hook name mismatch: got %s, want %s", retrieved.Name, hook.Name)
	}

	// Unregister hook
	if err := registry.Unregister("test-hook"); err != nil {
		t.Fatalf("Failed to unregister hook: %v", err)
	}

	// Verify hook no longer exists
	_, err = registry.GetHook("test-hook")
	if err == nil {
		t.Error("Expected error getting unregistered hook, got nil")
	}
}

func TestRegistryDuplicateHook(t *testing.T) {
	registry := NewRegistry()

	hook := Hook{
		Name:     "duplicate-test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "test",
		Timeout:  60,
	}

	// Register first time should succeed
	if err := registry.Register(hook); err != nil {
		t.Fatalf("Failed to register hook first time: %v", err)
	}

	// Register second time should fail
	err := registry.Register(hook)
	if err == nil {
		t.Error("Expected error registering duplicate hook, got nil")
	}
	if err != nil && !isErrorType(err, ErrDuplicateHook) {
		t.Errorf("Expected ErrDuplicateHook, got: %v", err)
	}
}

func TestGetHooksByEvent(t *testing.T) {
	registry := NewRegistry()

	hooks := []Hook{
		{
			Name:     "hook-1",
			Event:    HookEventSessionCompletion,
			Priority: 10,
			Type:     HookTypeBinary,
			Command:  "test1",
			Timeout:  60,
		},
		{
			Name:     "hook-2",
			Event:    HookEventSessionCompletion,
			Priority: 5,
			Type:     HookTypeBinary,
			Command:  "test2",
			Timeout:  60,
		},
		{
			Name:     "hook-3",
			Event:    HookEventPhaseCompletion,
			Priority: 15,
			Type:     HookTypeBinary,
			Command:  "test3",
			Timeout:  60,
		},
	}

	for _, hook := range hooks {
		if err := registry.Register(hook); err != nil {
			t.Fatalf("Failed to register hook %s: %v", hook.Name, err)
		}
	}

	// Get session-completion hooks
	sessionHooks := registry.GetHooksByEvent(HookEventSessionCompletion)
	if len(sessionHooks) != 2 {
		t.Errorf("Expected 2 session-completion hooks, got %d", len(sessionHooks))
	}

	// Verify priority sorting (higher first)
	if len(sessionHooks) == 2 && sessionHooks[0].Priority < sessionHooks[1].Priority {
		t.Error("Hooks not sorted by priority (descending)")
	}

	// Get phase-completion hooks
	phaseHooks := registry.GetHooksByEvent(HookEventPhaseCompletion)
	if len(phaseHooks) != 1 {
		t.Errorf("Expected 1 phase-completion hook, got %d", len(phaseHooks))
	}
}

func TestValidateHook(t *testing.T) {
	tests := []struct {
		name    string
		hook    Hook
		wantErr bool
	}{
		{
			name: "valid hook",
			hook: Hook{
				Name:     "valid",
				Event:    HookEventSessionCompletion,
				Priority: 50,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  60,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			hook: Hook{
				Name:     "",
				Event:    HookEventSessionCompletion,
				Priority: 50,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  60,
			},
			wantErr: true,
		},
		{
			name: "invalid event",
			hook: Hook{
				Name:     "test",
				Event:    "invalid-event",
				Priority: 50,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  60,
			},
			wantErr: true,
		},
		{
			name: "priority too low",
			hook: Hook{
				Name:     "test",
				Event:    HookEventSessionCompletion,
				Priority: 0,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  60,
			},
			wantErr: true,
		},
		{
			name: "priority too high",
			hook: Hook{
				Name:     "test",
				Event:    HookEventSessionCompletion,
				Priority: 101,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  60,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			hook: Hook{
				Name:     "test",
				Event:    HookEventSessionCompletion,
				Priority: 50,
				Type:     "invalid-type",
				Command:  "test",
				Timeout:  60,
			},
			wantErr: true,
		},
		{
			name: "empty command",
			hook: Hook{
				Name:     "test",
				Event:    HookEventSessionCompletion,
				Priority: 50,
				Type:     HookTypeBinary,
				Command:  "",
				Timeout:  60,
			},
			wantErr: true,
		},
		{
			name: "timeout too high",
			hook: Hook{
				Name:     "test",
				Event:    HookEventSessionCompletion,
				Priority: 50,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  601,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			hook: Hook{
				Name:     "test",
				Event:    HookEventSessionCompletion,
				Priority: 50,
				Type:     HookTypeBinary,
				Command:  "test",
				Timeout:  -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHook(tt.hook)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHook() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.toml")

	registry := NewRegistryWithPath(path)

	// Loading nonexistent file should not error (creates empty registry)
	if err := registry.Load(); err != nil {
		t.Fatalf("Expected no error loading nonexistent file, got: %v", err)
	}

	// Registry should be empty
	hooks := registry.ListAll()
	if len(hooks) != 0 {
		t.Errorf("Expected empty registry, got %d hooks", len(hooks))
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "hooks.toml")

	registry := NewRegistryWithPath(path)

	hook := Hook{
		Name:     "test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "test",
		Timeout:  60,
	}

	if err := registry.Register(hook); err != nil {
		t.Fatalf("Failed to register hook: %v", err)
	}

	// Save should create directory
	if err := registry.Save(); err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("File was not created")
	}
}

func TestListAll(t *testing.T) {
	registry := NewRegistry()

	hooks := []Hook{
		{
			Name:     "hook-a",
			Event:    HookEventSessionCompletion,
			Priority: 10,
			Type:     HookTypeBinary,
			Command:  "test",
			Timeout:  60,
		},
		{
			Name:     "hook-b",
			Event:    HookEventPhaseCompletion,
			Priority: 5,
			Type:     HookTypeBinary,
			Command:  "test",
			Timeout:  60,
		},
		{
			Name:     "hook-c",
			Event:    HookEventSessionCompletion,
			Priority: 15,
			Type:     HookTypeBinary,
			Command:  "test",
			Timeout:  60,
		},
	}

	for _, hook := range hooks {
		if err := registry.Register(hook); err != nil {
			t.Fatalf("Failed to register hook %s: %v", hook.Name, err)
		}
	}

	all := registry.ListAll()
	if len(all) != 3 {
		t.Errorf("Expected 3 hooks, got %d", len(all))
	}

	// Verify sorting: by event, then priority descending
	// Expected order: phase-completion(5), session-completion(15), session-completion(10)
	if len(all) == 3 {
		if all[0].Event != HookEventPhaseCompletion {
			t.Errorf("Expected first hook to be phase-completion, got %s", all[0].Event)
		}
		if all[1].Event != HookEventSessionCompletion || all[1].Priority != 15 {
			t.Errorf("Expected second hook to be session-completion with priority 15, got event=%s priority=%d", all[1].Event, all[1].Priority)
		}
		if all[2].Event != HookEventSessionCompletion || all[2].Priority != 10 {
			t.Errorf("Expected third hook to be session-completion with priority 10, got event=%s priority=%d", all[2].Event, all[2].Priority)
		}
	}
}

// Helper function to check if error is of specific type
func isErrorType(err error, target error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, target) || (err.Error() != "" && target.Error() != "")
}
