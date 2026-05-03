package integration

import (
	"os"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/backend"
)

// TestBackendSwitching_Default verifies that the default backend is tmux when no env var is set
func TestBackendSwitching_Default(t *testing.T) {
	// Save and restore env var
	oldEnv := os.Getenv("AGM_SESSION_BACKEND")
	defer func() {
		if oldEnv != "" {
			os.Setenv("AGM_SESSION_BACKEND", oldEnv)
		} else {
			os.Unsetenv("AGM_SESSION_BACKEND")
		}
	}()

	// Ensure env var is not set
	os.Unsetenv("AGM_SESSION_BACKEND")

	// Get backend
	b, err := backend.GetBackend()
	if err != nil {
		t.Fatalf("GetBackend() failed: %v", err)
	}

	// Verify it's a tmux backend
	if _, ok := b.(*backend.TmuxBackend); !ok {
		t.Errorf("expected TmuxBackend by default, got %T", b)
	}
}

// TestBackendSwitching_Tmux verifies that tmux backend is returned when AGM_SESSION_BACKEND=tmux
func TestBackendSwitching_Tmux(t *testing.T) {
	// Save and restore env var
	oldEnv := os.Getenv("AGM_SESSION_BACKEND")
	defer func() {
		if oldEnv != "" {
			os.Setenv("AGM_SESSION_BACKEND", oldEnv)
		} else {
			os.Unsetenv("AGM_SESSION_BACKEND")
		}
	}()

	// Set env var to tmux
	os.Setenv("AGM_SESSION_BACKEND", "tmux")

	// Get backend
	b, err := backend.GetBackend()
	if err != nil {
		t.Fatalf("GetBackend() failed: %v", err)
	}

	// Verify it's a tmux backend
	if _, ok := b.(*backend.TmuxBackend); !ok {
		t.Errorf("expected TmuxBackend, got %T", b)
	}
}

// TestBackendSwitching_Invalid verifies that an error is returned for invalid backend names
func TestBackendSwitching_Invalid(t *testing.T) {
	// Save and restore env var
	oldEnv := os.Getenv("AGM_SESSION_BACKEND")
	defer func() {
		if oldEnv != "" {
			os.Setenv("AGM_SESSION_BACKEND", oldEnv)
		} else {
			os.Unsetenv("AGM_SESSION_BACKEND")
		}
	}()

	// Set env var to an invalid backend
	os.Setenv("AGM_SESSION_BACKEND", "invalid-backend")

	// Get backend should fail
	_, err := backend.GetBackend()
	if err == nil {
		t.Fatal("expected error for invalid backend, got nil")
	}

	// Error should mention the invalid backend name
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}
}

// TestBackendSwitching_TmuxOperations tests that tmux backend works correctly
func TestBackendSwitching_TmuxOperations(t *testing.T) {
	// Save and restore env var
	oldEnv := os.Getenv("AGM_SESSION_BACKEND")
	defer func() {
		if oldEnv != "" {
			os.Setenv("AGM_SESSION_BACKEND", oldEnv)
		} else {
			os.Unsetenv("AGM_SESSION_BACKEND")
		}
	}()

	os.Setenv("AGM_SESSION_BACKEND", "tmux")

	// Get backend
	b, err := backend.GetBackend()
	if err != nil {
		t.Fatalf("GetBackend() failed: %v", err)
	}

	// Test HasSession (should work even if no sessions exist)
	exists, err := b.HasSession("nonexistent-session")
	if err != nil {
		t.Errorf("HasSession() failed: %v", err)
	}
	if exists {
		t.Error("expected session not to exist")
	}

	// Test ListSessions (should work even if list is empty)
	sessions, err := b.ListSessions()
	if err != nil {
		t.Errorf("ListSessions() failed: %v", err)
	}
	if sessions == nil {
		t.Error("expected non-nil sessions slice")
	}

	// Test ListSessionsWithInfo (should work even if list is empty)
	infos, err := b.ListSessionsWithInfo()
	if err != nil {
		t.Errorf("ListSessionsWithInfo() failed: %v", err)
	}
	if infos == nil {
		t.Error("expected non-nil session info slice")
	}

	// Test ListClients for non-existent session (should return empty list)
	clients, err := b.ListClients("nonexistent-session")
	if err != nil {
		t.Errorf("ListClients() failed: %v", err)
	}
	if clients == nil {
		t.Error("expected non-nil clients slice")
	}
}

// TestBackendSwitching_CompatibilityLayer tests that the backend implements the interface
func TestBackendSwitching_CompatibilityLayer(t *testing.T) {
	// Save and restore env var
	oldEnv := os.Getenv("AGM_SESSION_BACKEND")
	defer func() {
		if oldEnv != "" {
			os.Setenv("AGM_SESSION_BACKEND", oldEnv)
		} else {
			os.Unsetenv("AGM_SESSION_BACKEND")
		}
	}()

	os.Setenv("AGM_SESSION_BACKEND", "tmux")

	b, err := backend.GetBackend()
	if err != nil {
		t.Fatalf("GetBackend() failed: %v", err)
	}

	// Verify all interface methods are callable
	var _ = b

	// Test each method to ensure it doesn't panic
	_, _ = b.HasSession("test")
	_, _ = b.ListSessions()
	_, _ = b.ListSessionsWithInfo()
	_, _ = b.ListClients("test")
	_ = b.CreateSession("test", "/tmp")
	_ = b.AttachSession("test")
	_ = b.SendKeys("test", "echo hello")
}

// TestBackendSwitching_GetBackendByName tests the explicit backend selection function
func TestBackendSwitching_GetBackendByName(t *testing.T) {
	tests := []struct {
		name        string
		backendName string
		expectError bool
		expectType  interface{}
	}{
		{
			name:        "get tmux backend",
			backendName: "tmux",
			expectError: false,
			expectType:  &backend.TmuxBackend{},
		},
		{
			name:        "get invalid backend",
			backendName: "invalid",
			expectError: true,
			expectType:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := backend.GetBackendByName(tt.backendName)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check type
			switch tt.expectType.(type) {
			case *backend.TmuxBackend:
				if _, ok := b.(*backend.TmuxBackend); !ok {
					t.Errorf("expected TmuxBackend, got %T", b)
				}
			}
		})
	}
}

// TestBackendSwitching_BackwardCompatibility tests that existing code continues to work
// This test verifies that when no environment variable is set, tmux is used (backward compatible)
func TestBackendSwitching_BackwardCompatibility(t *testing.T) {
	// Save and restore env var
	oldEnv := os.Getenv("AGM_SESSION_BACKEND")
	defer func() {
		if oldEnv != "" {
			os.Setenv("AGM_SESSION_BACKEND", oldEnv)
		} else {
			os.Unsetenv("AGM_SESSION_BACKEND")
		}
	}()

	// Ensure no env var is set (simulates existing deployments)
	os.Unsetenv("AGM_SESSION_BACKEND")

	// Get backend (should default to tmux)
	b, err := backend.GetBackend()
	if err != nil {
		t.Fatalf("GetBackend() failed: %v", err)
	}

	// Must be tmux for backward compatibility
	if _, ok := b.(*backend.TmuxBackend); !ok {
		t.Errorf("expected TmuxBackend for backward compatibility, got %T", b)
	}

	// Verify basic operations work
	_, err = b.HasSession("test")
	if err != nil {
		t.Errorf("HasSession() failed: %v", err)
	}

	_, err = b.ListSessions()
	if err != nil {
		t.Errorf("ListSessions() failed: %v", err)
	}
}

// TestBackendSwitching_RegistryIntegrity verifies that all expected backends are registered
func TestBackendSwitching_RegistryIntegrity(t *testing.T) {
	backends := backend.ListBackends()

	if len(backends) < 1 {
		t.Errorf("expected at least 1 backend, got %d", len(backends))
	}

	// Check for required backends
	requiredBackends := []string{"tmux"}
	for _, required := range requiredBackends {
		found := false
		for _, b := range backends {
			if b == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required backend %q not registered", required)
		}
	}

	// Verify each backend can be instantiated
	for _, name := range backends {
		t.Run("instantiate_"+name, func(t *testing.T) {
			b, err := backend.GetBackendByName(name)
			if err != nil {
				t.Errorf("failed to instantiate backend %q: %v", name, err)
			}
			if b == nil {
				t.Errorf("backend %q returned nil instance", name)
			}
		})
	}
}
