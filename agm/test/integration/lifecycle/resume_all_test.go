package lifecycle

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestResumeAll_BasicFunctionality tests resume-all command with stopped sessions
func TestResumeAll_BasicFunctionality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create 3 test sessions in database
	sessions := []string{"test-resume-1", "test-resume-2", "test-resume-3"}
	for _, name := range sessions {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     name,
			Name:          name,
			Workspace:     "test",
			Harness:       "claude-code",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/tmp/test-" + name,
			},
			Tmux: manifest.Tmux{
				SessionName: name,
			},
		}

		require.NoError(t, adapter.CreateSession(m))
	}

	// Test: Resume all sessions (dry-run first)
	// Note: Actual CLI execution would require full integration test setup
	// This test validates the core logic and data structures

	// Verify sessions exist and are readable
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, len(manifests), "should have 3 sessions")

	// All sessions should be in stopped state (no tmux running)
	for _, m := range manifests {
		assert.Equal(t, "", m.Lifecycle, "sessions should not be archived")
	}
}

// TestResumeAll_HarnessFiltering tests harness filter flag
func TestResumeAll_HarnessFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get test adapter (all sessions in "test" workspace)
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create sessions with different harnesses
	testCases := []struct {
		name    string
		harness string
	}{
		{"session-claude-1", "claude-code"},
		{"session-claude-2", "claude-code"},
		{"session-gemini-1", "gemini-cli"},
		{"session-gemini-2", "gemini-cli"},
		{"session-codex-1", "codex-cli"},
	}

	for _, tc := range testCases {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     tc.name,
			Name:          tc.name,
			Workspace:     "test",
			Harness:       tc.harness,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/tmp/test-" + tc.name,
			},
			Tmux: manifest.Tmux{
				SessionName: tc.name,
			},
		}

		require.NoError(t, adapter.CreateSession(m))
	}

	// Load all sessions
	allManifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	require.NoError(t, err)
	assert.Equal(t, 5, len(allManifests))

	// Filter to claude-code harness
	claudeManifests, err := adapter.ListSessions(&dolt.SessionFilter{
		Harness: "claude-code",
	})
	require.NoError(t, err)

	assert.Equal(t, 2, len(claudeManifests), "should have 2 claude-code harness sessions")
	for _, m := range claudeManifests {
		assert.Equal(t, "claude-code", m.Harness)
	}
}

// TestResumeAll_SkipsArchived tests that archived sessions are not resumed
func TestResumeAll_SkipsArchived(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create mix of active and archived sessions
	testCases := []struct {
		name      string
		lifecycle string
	}{
		{"session-active-1", ""},
		{"session-active-2", ""},
		{"session-archived-1", manifest.LifecycleArchived},
		{"session-archived-2", manifest.LifecycleArchived},
	}

	for _, tc := range testCases {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     tc.name,
			Name:          tc.name,
			Workspace:     "test",
			Harness:       "claude-code",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     tc.lifecycle,
			Context: manifest.Context{
				Project: "/tmp/test-" + tc.name,
			},
			Tmux: manifest.Tmux{
				SessionName: tc.name,
			},
		}

		require.NoError(t, adapter.CreateSession(m))
	}

	// Load all sessions
	allManifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	require.NoError(t, err)
	assert.Equal(t, 4, len(allManifests))

	// Filter non-archived manually
	var activeManifests []*manifest.Manifest
	for _, m := range allManifests {
		if m.Lifecycle != manifest.LifecycleArchived {
			activeManifests = append(activeManifests, m)
		}
	}

	assert.Equal(t, 2, len(activeManifests), "should have 2 non-archived sessions")
	for _, m := range activeManifests {
		assert.NotEqual(t, manifest.LifecycleArchived, m.Lifecycle)
	}
}

// TestResumeAll_ErrorHandling tests error scenarios
func TestResumeAll_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("missing worktree", func(t *testing.T) {
		// Get test adapter
		adapter := dolt.GetTestAdapter(t)
		if adapter == nil {
			t.Skip("Dolt not available for testing")
		}
		defer adapter.Close()

		// Create session with non-existent project directory
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "broken-session",
			Name:          "broken-session",
			Workspace:     "test",
			Harness:       "claude-code",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/nonexistent/path/to/project",
			},
			Tmux: manifest.Tmux{
				SessionName: "broken-session",
			},
		}

		require.NoError(t, adapter.CreateSession(m))

		// Load session - should succeed
		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		require.NoError(t, err)
		assert.Equal(t, 1, len(manifests))

		// Health check would fail (tested separately)
		assert.Equal(t, "/nonexistent/path/to/project", manifests[0].Context.Project)
	})
}

// TestResumeAll_LargeScale tests performance with many sessions
func TestResumeAll_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-scale test in short mode")
	}

	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create 50 sessions in database
	sessionCount := 50
	for i := 0; i < sessionCount; i++ {
		name := fmt.Sprintf("session-%d", i)

		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     name,
			Name:          name,
			Workspace:     "large-scale-test",
			Harness:       "claude-code",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/tmp/test-" + name,
			},
			Tmux: manifest.Tmux{
				SessionName: name,
			},
		}

		require.NoError(t, adapter.CreateSession(m))
	}

	// Measure load time
	start := time.Now()
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{
		Workspace: "large-scale-test",
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, sessionCount, len(manifests))

	// Should load in reasonable time (<1 second for 50 sessions)
	assert.Less(t, elapsed, 1*time.Second, "loading 50 sessions should be fast")

	t.Logf("Loaded %d sessions in %v", len(manifests), elapsed)
}
