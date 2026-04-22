package session

import (
	"testing"
)

func TestNewRealTmux(t *testing.T) {
	rt := NewRealTmux()
	if rt == nil {
		t.Fatal("NewRealTmux() returned nil")
	}
}

func TestRealTmux_HasSession_NonExistent(t *testing.T) {
	rt := NewRealTmux()
	exists, err := rt.HasSession("agm-test-nonexistent-xyz-99999")
	if err != nil {
		// tmux may not be available in CI, just skip
		t.Skipf("tmux not available: %v", err)
	}
	if exists {
		t.Error("non-existent session should not exist")
	}
}

func TestRealTmux_ListSessions(t *testing.T) {
	rt := NewRealTmux()
	sessions, err := rt.ListSessions()
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	// sessions can be empty or have entries; just check no panic
	_ = sessions
}

func TestRealTmux_ListSessionsWithInfo(t *testing.T) {
	rt := NewRealTmux()
	sessions, err := rt.ListSessionsWithInfo()
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	_ = sessions
}

func TestRealTmux_ListClients(t *testing.T) {
	rt := NewRealTmux()
	// List clients for a nonexistent session - should not panic
	clients, err := rt.ListClients("agm-test-nonexistent-xyz-99999")
	if err != nil {
		// Expected - session doesn't exist
		return
	}
	_ = clients
}
