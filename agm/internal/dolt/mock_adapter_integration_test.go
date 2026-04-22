package dolt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestMockAdapterIntegration demonstrates using MockAdapter for integration testing.
// This serves as both a test and documentation for how to use MockAdapter in tests.
func TestMockAdapterIntegration(t *testing.T) {
	t.Run("session lifecycle with MockAdapter", func(t *testing.T) {
		// Setup: Create in-memory adapter (no database needed)
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()

		// Create a session
		session := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "test-session-123",
			Name:          "test-session",
			Workspace:     "test-workspace",
			Lifecycle:     "",
			Claude: manifest.Claude{
				UUID: "test-claude-uuid-456",
			},
			Tmux: manifest.Tmux{
				SessionName: "tmux-test-session",
			},
		}

		// Test: Create session
		err := adapter.CreateSession(session)
		require.NoError(t, err, "CreateSession should succeed")

		// Test: Retrieve session
		retrieved, err := adapter.GetSession("test-session-123")
		require.NoError(t, err, "GetSession should succeed")
		assert.Equal(t, "test-session", retrieved.Name)
		assert.Equal(t, "test-claude-uuid-456", retrieved.Claude.UUID)

		// Test: Update session (e.g., archive it)
		retrieved.Lifecycle = manifest.LifecycleArchived
		err = adapter.UpdateSession(retrieved)
		require.NoError(t, err, "UpdateSession should succeed")

		// Test: Verify update persisted
		updated, err := adapter.GetSession("test-session-123")
		require.NoError(t, err, "GetSession should succeed after update")
		assert.Equal(t, manifest.LifecycleArchived, updated.Lifecycle)

		// Test: List sessions with filter
		filter := &dolt.SessionFilter{
			Lifecycle: manifest.LifecycleArchived,
		}
		sessions, err := adapter.ListSessions(filter)
		require.NoError(t, err, "ListSessions should succeed")
		assert.Len(t, sessions, 1, "Should find 1 archived session")
		assert.Equal(t, "test-session-123", sessions[0].SessionID)
	})

	t.Run("concurrent operations", func(t *testing.T) {
		// MockAdapter is thread-safe - can be used in concurrent tests
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()

		// Create multiple sessions concurrently
		done := make(chan bool, 3)
		for i := 0; i < 3; i++ {
			go func(id int) {
				session := &manifest.Manifest{
					SessionID: string(rune('a' + id)),
					Name:      string(rune('a' + id)),
					Workspace: "test",
				}
				err := adapter.CreateSession(session)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 3; i++ {
			<-done
		}

		// Verify all sessions created
		sessions, err := adapter.ListSessions(&dolt.SessionFilter{})
		require.NoError(t, err)
		assert.Len(t, sessions, 3, "All 3 sessions should be created")
	})

	t.Run("error handling", func(t *testing.T) {
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()

		// Test: Missing session ID
		session := &manifest.Manifest{
			Name:      "no-id-session",
			Workspace: "test",
		}
		err := adapter.CreateSession(session)
		assert.Error(t, err, "CreateSession should fail without session_id")
		assert.Contains(t, err.Error(), "session_id is required")

		// Test: Duplicate session ID
		session.SessionID = "duplicate-test"
		err = adapter.CreateSession(session)
		require.NoError(t, err)

		err = adapter.CreateSession(session)
		assert.Error(t, err, "CreateSession should fail on duplicate")
		assert.Contains(t, err.Error(), "already exists")

		// Test: Get non-existent session
		_, err = adapter.GetSession("non-existent")
		assert.Error(t, err, "GetSession should fail for non-existent session")
		assert.Contains(t, err.Error(), "not found")

		// Test: Update non-existent session
		nonExistent := &manifest.Manifest{
			SessionID: "does-not-exist",
			Name:      "test",
		}
		err = adapter.UpdateSession(nonExistent)
		assert.Error(t, err, "UpdateSession should fail for non-existent session")
		assert.Contains(t, err.Error(), "not found")

		// Test: Operations after close
		adapter.Close()
		err = adapter.CreateSession(&manifest.Manifest{SessionID: "after-close"})
		assert.Error(t, err, "Operations should fail after close")
		assert.Contains(t, err.Error(), "closed")
	})
}

// TestMockAdapterFilterScenarios demonstrates various filtering scenarios
func TestMockAdapterFilterScenarios(t *testing.T) {
	t.Run("filter by lifecycle", func(t *testing.T) {
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()

		// Create sessions with different lifecycles
		sessions := []*manifest.Manifest{
			{SessionID: "active-1", Name: "active-1", Lifecycle: ""},
			{SessionID: "active-2", Name: "active-2", Lifecycle: ""},
			{SessionID: "archived-1", Name: "archived-1", Lifecycle: manifest.LifecycleArchived},
			{SessionID: "archived-2", Name: "archived-2", Lifecycle: manifest.LifecycleArchived},
		}

		for _, s := range sessions {
			err := adapter.CreateSession(s)
			require.NoError(t, err)
		}

		// Test: No filter returns all sessions
		noFilter := &dolt.SessionFilter{}
		allSessions, err := adapter.ListSessions(noFilter)
		require.NoError(t, err)
		assert.Len(t, allSessions, 4, "Should find all 4 sessions with no filter")

		// Test: Filter for archived sessions
		archivedFilter := &dolt.SessionFilter{
			Lifecycle: manifest.LifecycleArchived,
		}
		archivedSessions, err := adapter.ListSessions(archivedFilter)
		require.NoError(t, err)
		assert.Len(t, archivedSessions, 2, "Should find 2 archived sessions")

		// Verify only archived sessions returned
		for _, s := range archivedSessions {
			assert.Equal(t, manifest.LifecycleArchived, s.Lifecycle)
		}
	})

	t.Run("filter by workspace", func(t *testing.T) {
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()

		// Create sessions in different workspaces
		sessions := []*manifest.Manifest{
			{SessionID: "oss-1", Name: "oss-1", Workspace: "oss"},
			{SessionID: "oss-2", Name: "oss-2", Workspace: "oss"},
			{SessionID: "acme-1", Name: "acme-1", Workspace: "acme"},
		}

		for _, s := range sessions {
			err := adapter.CreateSession(s)
			require.NoError(t, err)
		}

		// Test: Filter by workspace
		ossFilter := &dolt.SessionFilter{
			Workspace: "oss",
		}
		ossSessions, err := adapter.ListSessions(ossFilter)
		require.NoError(t, err)
		assert.Len(t, ossSessions, 2, "Should find 2 oss sessions")
		for _, s := range ossSessions {
			assert.Equal(t, "oss", s.Workspace)
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()

		// Create diverse session set
		sessions := []*manifest.Manifest{
			{SessionID: "oss-active", Workspace: "oss", Lifecycle: ""},
			{SessionID: "oss-archived", Workspace: "oss", Lifecycle: manifest.LifecycleArchived},
			{SessionID: "acme-active", Workspace: "acme", Lifecycle: ""},
			{SessionID: "acme-archived", Workspace: "acme", Lifecycle: manifest.LifecycleArchived},
		}

		for _, s := range sessions {
			err := adapter.CreateSession(s)
			require.NoError(t, err)
		}

		// Test: Workspace + Lifecycle filter
		filter := &dolt.SessionFilter{
			Workspace: "oss",
			Lifecycle: manifest.LifecycleArchived,
		}
		results, err := adapter.ListSessions(filter)
		require.NoError(t, err)
		assert.Len(t, results, 1, "Should find 1 session matching both filters")
		assert.Equal(t, "oss-archived", results[0].SessionID)
	})
}

// TestMockAdapterPolymorphism demonstrates using Storage interface for polymorphic testing.
// This allows writing tests that work with both MockAdapter and real Adapter.
func TestMockAdapterPolymorphism(t *testing.T) {
	// This function works with any Storage implementation
	testStorage := func(t *testing.T, storage dolt.Storage) {
		session := &manifest.Manifest{
			SessionID: "poly-test",
			Name:      "polymorphic-test",
			Workspace: "test",
		}

		err := storage.CreateSession(session)
		require.NoError(t, err)

		retrieved, err := storage.GetSession("poly-test")
		require.NoError(t, err)
		assert.Equal(t, "polymorphic-test", retrieved.Name)
	}

	t.Run("with MockAdapter", func(t *testing.T) {
		adapter := dolt.NewMockAdapter()
		defer adapter.Close()
		testStorage(t, adapter)
	})

	// In real integration tests, you could also test with real Adapter:
	// t.Run("with real Adapter", func(t *testing.T) {
	//     adapter, err := dolt.New(&dolt.Config{...})
	//     require.NoError(t, err)
	//     defer adapter.Close()
	//     testStorage(t, adapter)
	// })
}
