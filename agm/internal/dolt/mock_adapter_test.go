package dolt

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestMockAdapter_CreateSession(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	session := NewTestManifest("test-session-1", "test-name-1")

	// Create session
	err := adapter.CreateSession(session)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Verify count
	if adapter.Count() != 1 {
		t.Errorf("expected 1 session, got %d", adapter.Count())
	}

	// Retrieve session
	retrieved, err := adapter.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if retrieved.SessionID != "test-session-1" {
		t.Errorf("expected session_id 'test-session-1', got '%s'", retrieved.SessionID)
	}
}

func TestMockAdapter_CreateSession_Duplicate(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	session := NewTestManifest("test-session-1", "test-name-1")

	// Create session
	err := adapter.CreateSession(session)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Try to create duplicate
	err = adapter.CreateSession(session)
	if err == nil {
		t.Error("expected error for duplicate session")
	}
}

func TestMockAdapter_GetSession_NotFound(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	_, err := adapter.GetSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestMockAdapter_UpdateSession(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	session := NewTestManifest("test-session-1", "test-name-1")
	err := adapter.CreateSession(session)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Update session
	session.Name = "updated-name"
	session.Lifecycle = manifest.LifecycleArchived
	err = adapter.UpdateSession(session)
	if err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	// Retrieve and verify
	retrieved, err := adapter.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if retrieved.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got '%s'", retrieved.Name)
	}

	if retrieved.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("expected lifecycle 'archived', got '%s'", retrieved.Lifecycle)
	}
}

func TestMockAdapter_DeleteSession(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	session := NewTestManifest("test-session-1", "test-name-1")
	err := adapter.CreateSession(session)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Delete session
	err = adapter.DeleteSession("test-session-1")
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify deleted
	if adapter.Count() != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", adapter.Count())
	}

	// Try to get deleted session
	_, err = adapter.GetSession("test-session-1")
	if err == nil {
		t.Error("expected error when getting deleted session")
	}
}

func TestMockAdapter_ListSessions_NoFilter(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	// Create test sessions
	adapter.CreateSession(NewTestManifest("session-1", "name-1"))
	adapter.CreateSession(NewTestManifest("session-2", "name-2"))
	adapter.CreateSession(NewTestManifest("session-3", "name-3"))

	// List all sessions
	sessions, err := adapter.ListSessions(&SessionFilter{})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestMockAdapter_ListSessions_LifecycleFilter(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	// Create active sessions
	adapter.CreateSession(NewTestManifest("session-1", "name-1"))
	adapter.CreateSession(NewTestManifest("session-2", "name-2"))

	// Create archived session
	archived := NewTestManifest("session-3", "name-3")
	archived.Lifecycle = manifest.LifecycleArchived
	adapter.CreateSession(archived)

	// No filter - returns all sessions
	sessions, err := adapter.ListSessions(&SessionFilter{})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("expected 3 total sessions, got %d", len(sessions))
	}

	// Filter for archived sessions only
	sessions, err = adapter.ListSessions(&SessionFilter{
		Lifecycle: manifest.LifecycleArchived,
	})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 archived session, got %d", len(sessions))
	}
}

func TestMockAdapter_ListSessions_AgentFilter(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	// Create claude sessions
	claude1 := NewTestManifest("session-1", "name-1")
	claude1.Harness = "claude-code"
	adapter.CreateSession(claude1)

	// Create gemini session
	gemini := NewTestManifest("session-2", "name-2")
	gemini.Harness = "gemini-cli"
	adapter.CreateSession(gemini)

	// Filter for claude sessions
	sessions, err := adapter.ListSessions(&SessionFilter{
		Harness: "claude-code",
	})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 claude session, got %d", len(sessions))
	}
}

func TestMockAdapter_ListSessions_WorkspaceFilter(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	// Create sessions with different workspaces
	oss1 := NewTestManifest("session-1", "name-1")
	oss1.Workspace = "oss"
	adapter.CreateSession(oss1)

	acme := NewTestManifest("session-2", "name-2")
	acme.Workspace = "acme"
	adapter.CreateSession(acme)

	// Filter for oss workspace
	sessions, err := adapter.ListSessions(&SessionFilter{
		Workspace: "oss",
	})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 oss session, got %d", len(sessions))
	}
}

func TestMockAdapter_ListSessions_LimitOffset(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	// Create 5 sessions
	for i := 1; i <= 5; i++ {
		adapter.CreateSession(NewTestManifest("session-"+string(rune(i+48)), "name-"+string(rune(i+48))))
	}

	// Test limit
	sessions, err := adapter.ListSessions(&SessionFilter{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions with limit, got %d", len(sessions))
	}

	// Test offset
	sessions, err = adapter.ListSessions(&SessionFilter{
		Offset: 3,
	})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions with offset 3, got %d", len(sessions))
	}

	// Test limit + offset
	sessions, err = adapter.ListSessions(&SessionFilter{
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions with limit+offset, got %d", len(sessions))
	}
}

func TestMockAdapter_Reset(t *testing.T) {
	adapter := NewMockAdapter()

	// Create sessions
	adapter.CreateSession(NewTestManifest("session-1", "name-1"))
	adapter.CreateSession(NewTestManifest("session-2", "name-2"))

	if adapter.Count() != 2 {
		t.Errorf("expected 2 sessions before reset, got %d", adapter.Count())
	}

	// Reset
	adapter.Reset()

	if adapter.Count() != 0 {
		t.Errorf("expected 0 sessions after reset, got %d", adapter.Count())
	}

	// Verify can create sessions after reset
	err := adapter.CreateSession(NewTestManifest("session-3", "name-3"))
	if err != nil {
		t.Errorf("failed to create session after reset: %v", err)
	}
}

func TestMockAdapter_Close(t *testing.T) {
	adapter := NewMockAdapter()

	// Create session
	adapter.CreateSession(NewTestManifest("session-1", "name-1"))

	// Close adapter
	err := adapter.Close()
	if err != nil {
		t.Fatalf("failed to close adapter: %v", err)
	}

	// Verify operations fail after close
	err = adapter.CreateSession(NewTestManifest("session-2", "name-2"))
	if err == nil {
		t.Error("expected error when creating session after close")
	}

	_, err = adapter.GetSession("session-1")
	if err == nil {
		t.Error("expected error when getting session after close")
	}

	_, err = adapter.ListSessions(&SessionFilter{})
	if err == nil {
		t.Error("expected error when listing sessions after close")
	}
}

func TestMockAdapter_DeepCopy(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	session := NewTestManifest("test-session-1", "test-name-1")
	session.Context.Tags = []string{"tag1", "tag2"}

	// Create session
	err := adapter.CreateSession(session)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Modify original session
	session.Name = "modified-name"
	session.Context.Tags[0] = "modified-tag"

	// Retrieve session
	retrieved, err := adapter.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Verify original session was copied, not referenced
	if retrieved.Name == "modified-name" {
		t.Error("expected name to be unchanged (deep copy failed)")
	}

	if len(retrieved.Context.Tags) > 0 && retrieved.Context.Tags[0] == "modified-tag" {
		t.Error("expected tags to be unchanged (deep copy failed)")
	}
}

func TestMockAdapter_UpdatedAt(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	session := NewTestManifest("test-session-1", "test-name-1")
	originalUpdatedAt := session.UpdatedAt

	err := adapter.CreateSession(session)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update session
	session.Name = "updated-name"
	err = adapter.UpdateSession(session)
	if err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	// Retrieve and verify UpdatedAt changed
	retrieved, err := adapter.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if !retrieved.UpdatedAt.After(originalUpdatedAt) {
		t.Error("expected UpdatedAt to be updated after UpdateSession()")
	}
}

func TestMockAdapter_ListSessions_TagFilter(t *testing.T) {
	adapter := NewMockAdapter()
	defer adapter.Close()

	// Create sessions with different tags
	s1 := NewTestManifest("s1", "worker-1")
	s1.Context.Tags = []string{"role:worker", "cap:claude-code"}
	s2 := NewTestManifest("s2", "orchestrator-1")
	s2.Context.Tags = []string{"role:orchestrator"}
	s3 := NewTestManifest("s3", "worker-2")
	s3.Context.Tags = []string{"role:worker", "cap:web-search"}

	for _, s := range []*manifest.Manifest{s1, s2, s3} {
		if err := adapter.CreateSession(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
	}

	// Filter by single tag
	results, err := adapter.ListSessions(&SessionFilter{
		Tags: []string{"role:worker"},
	})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 workers, got %d", len(results))
	}

	// Filter by multiple tags (AND)
	results, err = adapter.ListSessions(&SessionFilter{
		Tags: []string{"role:worker", "cap:claude-code"},
	})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 session with both tags, got %d", len(results))
	}
	if len(results) > 0 && results[0].Name != "worker-1" {
		t.Errorf("expected worker-1, got %s", results[0].Name)
	}

	// Filter by non-existent tag
	results, err = adapter.ListSessions(&SessionFilter{
		Tags: []string{"role:reviewer"},
	})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(results))
	}
}
