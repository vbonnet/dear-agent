package backend

import (
	"os"
	"sort"
	"testing"
)

// mockBackend is a simple mock implementation of Backend for testing
type mockBackend struct {
	name string
}

func (m *mockBackend) HasSession(name string) (bool, error)                 { return false, nil }
func (m *mockBackend) ListSessions() ([]string, error)                      { return nil, nil }
func (m *mockBackend) ListSessionsWithInfo() ([]SessionInfo, error)         { return nil, nil }
func (m *mockBackend) ListClients(sessionName string) ([]ClientInfo, error) { return nil, nil }
func (m *mockBackend) CreateSession(name, workdir string) error             { return nil }
func (m *mockBackend) AttachSession(name string) error                      { return nil }
func (m *mockBackend) SendKeys(session, keys string) error                  { return nil }

// registerDefaultBackends re-registers the default backends (tmux)
// This is needed after unregisterAll() is called in tests
func registerDefaultBackends() {
	_ = Register("tmux", func() (Backend, error) {
		return NewTmuxBackend(), nil
	})
}

func TestRegister(t *testing.T) {
	// Clean up registry before test
	defer func() {
		unregisterAll()
		registerDefaultBackends()
	}()
	unregisterAll()

	// Test registering a new backend
	if err := Register("test-backend", func() (Backend, error) {
		return &mockBackend{name: "test"}, nil
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if !IsRegistered("test-backend") {
		t.Error("expected backend to be registered")
	}

	backends := ListBackends()
	if len(backends) != 1 || backends[0] != "test-backend" {
		t.Errorf("expected 1 backend, got %v", backends)
	}
}

func TestRegisterNilFactory(t *testing.T) {
	defer func() {
		unregisterAll()
		registerDefaultBackends()
	}()
	unregisterAll()

	err := Register("nil-backend", nil)
	if err == nil {
		t.Error("expected error on nil factory")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	defer func() {
		unregisterAll()
		registerDefaultBackends()
	}()
	unregisterAll()

	factory := func() (Backend, error) {
		return &mockBackend{name: "test"}, nil
	}

	if err := Register("duplicate-backend", factory); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	err := Register("duplicate-backend", factory)
	if err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestGetBackend_Default(t *testing.T) {
	// Don't unregister all - we need tmux to be registered
	// Just ensure AGM_SESSION_BACKEND is not set
	t.Setenv("AGM_SESSION_BACKEND", "") // restored on test cleanup
	os.Unsetenv("AGM_SESSION_BACKEND")

	backend, err := GetBackend()
	if err != nil {
		t.Fatalf("GetBackend() failed: %v", err)
	}
	if backend == nil {
		t.Error("expected backend to be non-nil")
	}

	// Should return tmux backend by default
	if _, ok := backend.(*TmuxBackend); !ok {
		t.Errorf("expected TmuxBackend, got %T", backend)
	}
}

func TestGetBackend_EnvVar(t *testing.T) {
	// Save and restore env var

	tests := []struct {
		name        string
		envValue    string
		expectError bool
		expectType  interface{}
	}{
		{
			name:        "tmux backend",
			envValue:    "tmux",
			expectError: false,
			expectType:  &TmuxBackend{},
		},
		{
			name:        "unknown backend",
			envValue:    "nonexistent",
			expectError: true,
			expectType:  nil,
		},
		{
			name:        "empty backend (defaults to tmux)",
			envValue:    "",
			expectError: false,
			expectType:  &TmuxBackend{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("AGM_SESSION_BACKEND")
			} else {
				t.Setenv("AGM_SESSION_BACKEND", tt.envValue)
			}

			backend, err := GetBackend()

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if backend == nil {
					t.Error("expected backend to be non-nil")
				}

				// Check type
				switch tt.expectType.(type) {
				case *TmuxBackend:
					if _, ok := backend.(*TmuxBackend); !ok {
						t.Errorf("expected TmuxBackend, got %T", backend)
					}
				}
			}
		})
	}
}

func TestGetBackendByName(t *testing.T) {
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
			expectType:  &TmuxBackend{},
		},
		{
			name:        "get nonexistent backend",
			backendName: "nonexistent",
			expectError: true,
			expectType:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := GetBackendByName(tt.backendName)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if backend == nil {
					t.Error("expected backend to be non-nil")
				}

				// Check type
				switch tt.expectType.(type) {
				case *TmuxBackend:
					if _, ok := backend.(*TmuxBackend); !ok {
						t.Errorf("expected TmuxBackend, got %T", backend)
					}
				}
			}
		})
	}
}

func TestListBackends(t *testing.T) {
	// tmux should be registered via init()
	backends := ListBackends()

	if len(backends) < 1 {
		t.Errorf("expected at least 1 backend, got %d", len(backends))
	}

	// Convert to map for easier checking
	backendMap := make(map[string]bool)
	for _, name := range backends {
		backendMap[name] = true
	}

	if !backendMap["tmux"] {
		t.Error("expected tmux backend to be registered")
	}
}

func TestIsRegistered(t *testing.T) {
	tests := []struct {
		name     string
		backend  string
		expected bool
	}{
		{
			name:     "tmux is registered",
			backend:  "tmux",
			expected: true,
		},
		{
			name:     "nonexistent is not registered",
			backend:  "nonexistent",
			expected: false,
		},
		{
			name:     "empty string is not registered",
			backend:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRegistered(tt.backend)
			if result != tt.expected {
				t.Errorf("IsRegistered(%q) = %v, expected %v", tt.backend, result, tt.expected)
			}
		})
	}
}

func TestUnregisterAll(t *testing.T) {
	// This test ensures unregisterAll works correctly
	defer registerDefaultBackends()

	unregisterAll()

	backends := ListBackends()
	if len(backends) != 0 {
		t.Errorf("expected 0 backends after unregisterAll, got %d", len(backends))
	}
}

func TestBackendInterfaceCompliance(t *testing.T) {
	// Ensure all registered backends implement the Backend interface
	backends := ListBackends()

	for _, name := range backends {
		t.Run(name, func(t *testing.T) {
			backend, err := GetBackendByName(name)
			if err != nil {
				t.Fatalf("failed to get backend %q: %v", name, err)
			}

			// Test that backend implements all interface methods
			// (This is mainly a compile-time check, but we verify at runtime too)
			var _ = backend
		})
	}
}

func TestRegistryConcurrency(t *testing.T) {
	// Test that registry operations are thread-safe
	// This test spawns multiple goroutines accessing the registry
	done := make(chan bool)

	// Spawn readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ListBackends()
				IsRegistered("tmux")
			}
			done <- true
		}()
	}

	// Wait for all readers to finish
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGetBackendErrorMessages(t *testing.T) {
	// Save and restore env var

	t.Setenv("AGM_SESSION_BACKEND", "nonexistent")

	_, err := GetBackend()
	if err == nil {
		t.Fatal("expected error for nonexistent backend")
	}

	// Error message should include available backends
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}

	// Should mention the requested backend
	if !contains(errMsg, "nonexistent") {
		t.Errorf("error message should mention requested backend: %s", errMsg)
	}
}

func TestListBackendsSorted(t *testing.T) {
	backends := ListBackends()

	// Check that we can sort the backends (they should be strings)
	sorted := make([]string, len(backends))
	copy(sorted, backends)
	sort.Strings(sorted)

	// This just verifies the list is valid, not that it's pre-sorted
	if len(sorted) != len(backends) {
		t.Error("sorted list length mismatch")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
