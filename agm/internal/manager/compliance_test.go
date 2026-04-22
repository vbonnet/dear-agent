package manager_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
)

// mockBackend is an in-memory implementation of manager.Backend for testing.
// It can be used to verify interface compliance for any backend.
type mockBackend struct {
	name     string
	mu       sync.Mutex
	sessions map[manager.SessionID]*mockSession
}

type mockSession struct {
	info    manager.SessionInfo
	output  string
	state   manager.State
	canRecv manager.CanReceive
}

func newMockBackend(name string) *mockBackend {
	return &mockBackend{
		name:     name,
		sessions: make(map[manager.SessionID]*mockSession),
	}
}

func (b *mockBackend) Name() string { return b.name }

func (b *mockBackend) Capabilities() manager.BackendCapabilities {
	return manager.BackendCapabilities{
		SupportsAttach:        false,
		SupportsStructuredIO:  true,
		SupportsInterrupt:     true,
		MaxConcurrentSessions: 10,
	}
}

func (b *mockBackend) CreateSession(_ context.Context, config manager.SessionConfig) (manager.SessionID, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := manager.SessionID(config.Name)
	if _, exists := b.sessions[id]; exists {
		return "", fmt.Errorf("session %q already exists", config.Name)
	}
	b.sessions[id] = &mockSession{
		info: manager.SessionInfo{
			ID:      id,
			Name:    config.Name,
			State:   manager.StateIdle,
			Harness: config.Harness,
		},
		state:   manager.StateIdle,
		canRecv: manager.CanReceiveYes,
	}
	return id, nil
}

func (b *mockBackend) TerminateSession(_ context.Context, id manager.SessionID) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, id)
	return nil
}

func (b *mockBackend) ListSessions(_ context.Context, filter manager.SessionFilter) ([]manager.SessionInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var results []manager.SessionInfo
	for _, s := range b.sessions {
		results = append(results, s.info)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

func (b *mockBackend) GetSession(_ context.Context, id manager.SessionID) (manager.SessionInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[id]
	if !ok {
		return manager.SessionInfo{}, fmt.Errorf("session %q not found", id)
	}
	return s.info, nil
}

func (b *mockBackend) RenameSession(_ context.Context, id manager.SessionID, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	s.info.Name = name
	return nil
}

func (b *mockBackend) SendMessage(_ context.Context, id manager.SessionID, message string) (manager.SendResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[id]
	if !ok {
		return manager.SendResult{Delivered: false}, fmt.Errorf("session %q not found", id)
	}
	s.output += message + "\n"
	return manager.SendResult{Delivered: true}, nil
}

func (b *mockBackend) ReadOutput(_ context.Context, id manager.SessionID, lines int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[id]
	if !ok {
		return "", fmt.Errorf("session %q not found", id)
	}
	return s.output, nil
}

func (b *mockBackend) Interrupt(_ context.Context, id manager.SessionID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	return nil
}

func (b *mockBackend) GetState(_ context.Context, id manager.SessionID) (manager.StateResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[id]
	if !ok {
		return manager.StateResult{
			State:      manager.StateOffline,
			Confidence: 1.0,
			Evidence:   "session not found",
		}, nil
	}
	return manager.StateResult{
		State:      s.state,
		Confidence: 1.0,
		Evidence:   "mock backend",
	}, nil
}

func (b *mockBackend) CheckDelivery(_ context.Context, id manager.SessionID) (manager.CanReceive, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[id]
	if !ok {
		return manager.CanReceiveNotFound, nil
	}
	return s.canRecv, nil
}

func (b *mockBackend) HealthCheck(_ context.Context) error {
	return nil
}

// Compile-time check
var _ manager.Backend = (*mockBackend)(nil)

// --- Interface Compliance Tests ---
// These tests verify the contract that any Backend implementation must satisfy.
// Run against any backend by calling runComplianceSuite(t, backend).

func runComplianceSuite(t *testing.T, b manager.Backend) {
	t.Helper()
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		name := b.Name()
		if name == "" {
			t.Error("Name() returned empty string")
		}
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := b.Capabilities()
		// Just verify it doesn't panic; values are backend-specific
		_ = caps.SupportsAttach
		_ = caps.SupportsStructuredIO
		_ = caps.SupportsInterrupt
		_ = caps.MaxConcurrentSessions
	})

	t.Run("HealthCheck", func(t *testing.T) {
		if err := b.HealthCheck(ctx); err != nil {
			t.Errorf("HealthCheck failed: %v", err)
		}
	})

	t.Run("CreateAndGetSession", func(t *testing.T) {
		id, err := b.CreateSession(ctx, manager.SessionConfig{
			Name:    "compliance-test",
			Harness: "test",
		})
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
		if id == "" {
			t.Fatal("CreateSession returned empty ID")
		}

		info, err := b.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if info.Name != "compliance-test" {
			t.Errorf("expected name 'compliance-test', got %q", info.Name)
		}

		// Cleanup
		_ = b.TerminateSession(ctx, id)
	})

	t.Run("ListSessions", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "list-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		sessions, err := b.ListSessions(ctx, manager.SessionFilter{})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(sessions) == 0 {
			t.Error("ListSessions returned empty after CreateSession")
		}
	})

	t.Run("SendAndReadMessage", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "msg-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		result, err := b.SendMessage(ctx, id, "hello world")
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		if !result.Delivered {
			t.Error("SendMessage reported not delivered")
		}

		output, err := b.ReadOutput(ctx, id, 10)
		if err != nil {
			t.Fatalf("ReadOutput failed: %v", err)
		}
		if output == "" {
			t.Error("ReadOutput returned empty after SendMessage")
		}
	})

	t.Run("GetState", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "state-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		state, err := b.GetState(ctx, id)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		if state.State == "" {
			t.Error("GetState returned empty state")
		}
		if state.Confidence < 0 || state.Confidence > 1 {
			t.Errorf("GetState confidence out of range: %f", state.Confidence)
		}
	})

	t.Run("CheckDelivery", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "delivery-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		canRecv, err := b.CheckDelivery(ctx, id)
		if err != nil {
			t.Fatalf("CheckDelivery failed: %v", err)
		}
		// An idle session should be able to receive
		if canRecv != manager.CanReceiveYes {
			t.Errorf("expected CanReceiveYes for idle session, got %d", canRecv)
		}
	})

	t.Run("CheckDelivery_NotFound", func(t *testing.T) {
		canRecv, err := b.CheckDelivery(ctx, "nonexistent-session-xyz")
		if err != nil {
			t.Fatalf("CheckDelivery for nonexistent should not error: %v", err)
		}
		if canRecv != manager.CanReceiveNotFound {
			t.Errorf("expected CanReceiveNotFound, got %d", canRecv)
		}
	})

	t.Run("RenameSession", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "rename-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		err := b.RenameSession(ctx, id, "renamed-test")
		if err != nil {
			t.Fatalf("RenameSession failed: %v", err)
		}
	})

	t.Run("TerminateSession", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "terminate-test", Harness: "test"})

		err := b.TerminateSession(ctx, id)
		if err != nil {
			t.Fatalf("TerminateSession failed: %v", err)
		}

		// After termination, session should not be found
		_, err = b.GetSession(ctx, id)
		if err == nil {
			t.Error("GetSession should fail after TerminateSession")
		}
	})

	t.Run("Interrupt", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "interrupt-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		err := b.Interrupt(ctx, id)
		if err != nil {
			t.Fatalf("Interrupt failed: %v", err)
		}
	})

	t.Run("GetState_Offline", func(t *testing.T) {
		state, err := b.GetState(ctx, "nonexistent-session-xyz")
		if err != nil {
			t.Fatalf("GetState for nonexistent should not error: %v", err)
		}
		if state.State != manager.StateOffline {
			t.Errorf("expected StateOffline for nonexistent session, got %q", state.State)
		}
	})
}

// TestMockBackendCompliance verifies the mock itself passes compliance tests.
func TestMockBackendCompliance(t *testing.T) {
	b := newMockBackend("mock")
	runComplianceSuite(t, b)
}
