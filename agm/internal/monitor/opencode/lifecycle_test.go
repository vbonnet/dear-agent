package opencode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name        string
		eventBus    EventBusPublisher
		config      Config
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid configuration",
			eventBus: &mockEventBus{},
			config: Config{
				ServerURL: "http://localhost:4096",
				SessionID: "test-session",
				Reconnect: DefaultReconnectConfig(),
			},
			wantErr: false,
		},
		{
			name:     "nil eventBus",
			eventBus: nil,
			config: Config{
				ServerURL: "http://localhost:4096",
				SessionID: "test-session",
			},
			wantErr:     true,
			errContains: "eventBus cannot be nil",
		},
		{
			name:     "empty serverURL",
			eventBus: &mockEventBus{},
			config: Config{
				ServerURL: "",
				SessionID: "test-session",
			},
			wantErr:     true,
			errContains: "serverURL cannot be empty",
		},
		{
			name:     "empty sessionID",
			eventBus: &mockEventBus{},
			config: Config{
				ServerURL: "http://localhost:4096",
				SessionID: "",
			},
			wantErr:     true,
			errContains: "sessionID cannot be empty",
		},
		{
			name:     "defaults applied",
			eventBus: &mockEventBus{},
			config: Config{
				ServerURL: "http://localhost:4096",
				SessionID: "test-session",
				// HealthProbeURL and HealthTimeout not set
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewAdapter(tt.eventBus, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewAdapter() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewAdapter() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("NewAdapter() unexpected error = %v", err)
				return
			}

			if adapter == nil {
				t.Error("NewAdapter() returned nil adapter")
				return
			}

			// Verify components are initialized
			if adapter.sseClient == nil {
				t.Error("sseClient not initialized")
			}
			if adapter.parser == nil {
				t.Error("parser not initialized")
			}
			if adapter.publisher == nil {
				t.Error("publisher not initialized")
			}
			if adapter.mapper == nil {
				t.Error("mapper not initialized")
			}

			// Verify defaults
			if adapter.config.HealthProbeURL == "" {
				t.Error("HealthProbeURL default not applied")
			}
			if adapter.config.HealthTimeout == 0 {
				t.Error("HealthTimeout default not applied")
			}
		})
	}
}

func TestAdapter_HealthProbe_Success(t *testing.T) {
	// Create mock server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	eventBus := &mockEventBus{}
	config := Config{
		ServerURL:      server.URL,
		SessionID:      "test-session",
		HealthProbeURL: "/health",
		HealthTimeout:  5 * time.Second,
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	ctx := context.Background()
	err = adapter.healthProbe(ctx)
	if err != nil {
		t.Errorf("healthProbe() error = %v, want nil", err)
	}
}

func TestAdapter_HealthProbe_Failure(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantErr     bool
		errContains string
	}{
		{
			name:        "server returns 500",
			statusCode:  http.StatusInternalServerError,
			wantErr:     true,
			errContains: "non-OK status: 500",
		},
		{
			name:        "server returns 404",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			errContains: "non-OK status: 404",
		},
		{
			name:        "server returns 503",
			statusCode:  http.StatusServiceUnavailable,
			wantErr:     true,
			errContains: "non-OK status: 503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server with specified status code
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			eventBus := &mockEventBus{}
			config := Config{
				ServerURL:      server.URL,
				SessionID:      "test-session",
				HealthProbeURL: "/health",
				HealthTimeout:  5 * time.Second,
			}

			adapter, err := NewAdapter(eventBus, config)
			if err != nil {
				t.Fatalf("NewAdapter() error = %v", err)
			}

			ctx := context.Background()
			err = adapter.healthProbe(ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("healthProbe() expected error, got nil")
					return
				}
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("healthProbe() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("healthProbe() unexpected error = %v", err)
			}
		})
	}
}

func TestAdapter_HealthProbe_Timeout(t *testing.T) {
	// Create mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Delay longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	eventBus := &mockEventBus{}
	config := Config{
		ServerURL:      server.URL,
		SessionID:      "test-session",
		HealthProbeURL: "/health",
		HealthTimeout:  100 * time.Millisecond, // Short timeout
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	ctx := context.Background()
	err = adapter.healthProbe(ctx)

	if err == nil {
		t.Error("healthProbe() expected timeout error, got nil")
		return
	}

	if !contains(err.Error(), "health probe failed") {
		t.Errorf("healthProbe() error = %q, want error containing 'health probe failed'", err.Error())
	}
}

func TestAdapter_Start_HealthProbeFailure(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	eventBus := &mockEventBus{}
	config := Config{
		ServerURL:      server.URL,
		SessionID:      "test-session",
		HealthProbeURL: "/health",
		HealthTimeout:  5 * time.Second,
		FallbackTmux:   false, // Disable fallback
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	ctx := context.Background()
	err = adapter.Start(ctx)

	if err == nil {
		t.Error("Start() expected error, got nil")
		return
	}

	if !contains(err.Error(), "health check failed") {
		t.Errorf("Start() error = %q, want error containing 'health check failed'", err.Error())
	}
}

func TestAdapter_Start_WithFallback(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	eventBus := &mockEventBus{}
	config := Config{
		ServerURL:      server.URL,
		SessionID:      "test-session",
		HealthProbeURL: "/health",
		HealthTimeout:  5 * time.Second,
		FallbackTmux:   true, // Enable fallback
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	ctx := context.Background()
	err = adapter.Start(ctx)

	// Should return error but with fallback message
	if err == nil {
		t.Error("Start() expected error with fallback message, got nil")
		return
	}

	if !contains(err.Error(), "tmux fallback active") {
		t.Errorf("Start() error = %q, want error containing 'tmux fallback active'", err.Error())
	}
}

func TestAdapter_Stop(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	eventBus := &mockEventBus{}
	config := Config{
		ServerURL:      server.URL,
		SessionID:      "test-session",
		HealthProbeURL: "/health",
		HealthTimeout:  5 * time.Second,
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	// Stop without starting should not error
	ctx := context.Background()
	err = adapter.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}

	// Verify session mapper is cleared
	if adapter.mapper.Count() != 0 {
		t.Errorf("mapper.Count() = %d, want 0", adapter.mapper.Count())
	}
}

func TestAdapter_Health(t *testing.T) {
	eventBus := &mockEventBus{}
	config := Config{
		ServerURL: "http://localhost:4096",
		SessionID: "test-session",
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	health := adapter.Health()

	// Should delegate to SSE client health
	if health.Connected {
		t.Error("Health() Connected = true, want false (not started)")
	}
	if health.Metadata == nil {
		t.Error("Health() Metadata is nil")
	}
}

func TestAdapter_Name(t *testing.T) {
	eventBus := &mockEventBus{}
	config := Config{
		ServerURL: "http://localhost:4096",
		SessionID: "test-session",
	}

	adapter, err := NewAdapter(eventBus, config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	name := adapter.Name()
	if name != "opencode-sse" {
		t.Errorf("Name() = %q, want %q", name, "opencode-sse")
	}
}

func TestSessionMapper_RegisterAndLookup(t *testing.T) {
	mapper := NewSessionMapper()

	// Test lookup on empty mapper
	_, ok := mapper.Lookup("opencode-123")
	if ok {
		t.Error("Lookup() on empty mapper returned true")
	}

	// Register mapping
	mapper.Register("opencode-123", "agm-session-1")

	// Test successful lookup
	agmID, ok := mapper.Lookup("opencode-123")
	if !ok {
		t.Error("Lookup() returned false for registered ID")
	}
	if agmID != "agm-session-1" {
		t.Errorf("Lookup() = %q, want %q", agmID, "agm-session-1")
	}

	// Test count
	if mapper.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mapper.Count())
	}

	// Register another mapping
	mapper.Register("opencode-456", "agm-session-2")
	if mapper.Count() != 2 {
		t.Errorf("Count() = %d, want 2", mapper.Count())
	}
}

func TestSessionMapper_Remove(t *testing.T) {
	mapper := NewSessionMapper()

	// Register mapping
	mapper.Register("opencode-123", "agm-session-1")
	mapper.Register("opencode-456", "agm-session-2")

	// Remove one mapping
	mapper.Remove("opencode-123")

	// Verify removed
	_, ok := mapper.Lookup("opencode-123")
	if ok {
		t.Error("Lookup() returned true for removed ID")
	}

	// Verify other mapping still exists
	agmID, ok := mapper.Lookup("opencode-456")
	if !ok {
		t.Error("Lookup() returned false for existing ID")
	}
	if agmID != "agm-session-2" {
		t.Errorf("Lookup() = %q, want %q", agmID, "agm-session-2")
	}

	// Verify count
	if mapper.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mapper.Count())
	}
}

func TestSessionMapper_Clear(t *testing.T) {
	mapper := NewSessionMapper()

	// Register multiple mappings
	mapper.Register("opencode-123", "agm-session-1")
	mapper.Register("opencode-456", "agm-session-2")
	mapper.Register("opencode-789", "agm-session-3")

	// Clear all mappings
	mapper.Clear()

	// Verify all removed
	if mapper.Count() != 0 {
		t.Errorf("Count() = %d, want 0", mapper.Count())
	}

	_, ok := mapper.Lookup("opencode-123")
	if ok {
		t.Error("Lookup() returned true after Clear()")
	}
}

func TestSessionMapper_Concurrent(t *testing.T) {
	mapper := NewSessionMapper()

	// Test concurrent reads and writes
	done := make(chan bool)
	numGoroutines := 10

	// Writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			mapper.Register("opencode-"+string(rune(id)), "agm-"+string(rune(id)))
			done <- true
		}(i)
	}

	// Readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			mapper.Lookup("opencode-" + string(rune(id)))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}

	// Test should complete without race detector errors
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Enabled {
		t.Error("DefaultConfig() Enabled = true, want false")
	}
	if config.ServerURL == "" {
		t.Error("DefaultConfig() ServerURL is empty")
	}
	if config.HealthProbeURL == "" {
		t.Error("DefaultConfig() HealthProbeURL is empty")
	}
	if config.HealthTimeout == 0 {
		t.Error("DefaultConfig() HealthTimeout is zero")
	}
	if !config.FallbackTmux {
		t.Error("DefaultConfig() FallbackTmux = false, want true")
	}
}
