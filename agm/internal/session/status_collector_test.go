package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestCollectStatus(t *testing.T) {
	// Create test manifest — session doesn't exist in tmux, so
	// ResolveSessionState returns OFFLINE (ground truth: tmux existence)
	m := &manifest.Manifest{
		Name:           "test-session",
		SessionID:      "test-123",
		State:          manifest.StateDone,
		StateSource:    "hook",
		StateUpdatedAt: time.Now(),
		Workspace:      "test-workspace",
		Context: manifest.Context{
			Project: "/tmp/test-project",
		},
	}

	status, err := CollectStatus(m)
	require.NoError(t, err)

	assert.Equal(t, "test-session", status.Name)
	assert.Equal(t, "test-123", status.SessionID)
	// No tmux session → OFFLINE (ResolveSessionState checks tmux as ground truth)
	assert.Equal(t, manifest.StateOffline, status.State)
	assert.Equal(t, "hook", status.StateSource)
	assert.Equal(t, "test-workspace", status.Workspace)
	assert.Equal(t, "/tmp/test-project", status.WorktreePath)
}

func TestCollectStatus_EmptyState(t *testing.T) {
	// Test that empty state defaults to OFFLINE
	m := &manifest.Manifest{
		Name:      "test-session",
		SessionID: "test-123",
		State:     "", // Empty state
		Context: manifest.Context{
			Project: "/tmp/test-project",
		},
	}

	status, err := CollectStatus(m)
	require.NoError(t, err)

	assert.Equal(t, manifest.StateOffline, status.State, "Empty state should default to OFFLINE")
}

func TestCollectStatus_LastStateUpdate(t *testing.T) {
	now := time.Now()

	m := &manifest.Manifest{
		Name:           "test-session",
		SessionID:      "test-123",
		StateUpdatedAt: now,
		Context: manifest.Context{
			Project: "/tmp/test-project",
		},
	}

	status, err := CollectStatus(m)
	require.NoError(t, err)

	// Verify timestamp format (HH:MM:SS)
	expectedFormat := now.Format("15:04:05")
	assert.Equal(t, expectedFormat, status.LastStateUpdate)
}

func TestCollectStatus_NeverUpdated(t *testing.T) {
	m := &manifest.Manifest{
		Name:      "test-session",
		SessionID: "test-123",
		// StateUpdatedAt is zero value (never updated)
		Context: manifest.Context{
			Project: "/tmp/test-project",
		},
	}

	status, err := CollectStatus(m)
	require.NoError(t, err)

	assert.Equal(t, "never", status.LastStateUpdate)
}

func TestGetCurrentBranch_ValidRepo(t *testing.T) {
	// Test with ai-tools repository (known to exist)
	repoPath := "~/src/ws/oss/repos/ai-tools"
	expandedPath := os.ExpandEnv(repoPath)
	if expandedPath == repoPath {
		// Expand ~ manually
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		expandedPath = filepath.Join(homeDir, repoPath[1:])
	}

	// Skip test if repo doesn't exist or isn't a git repo
	if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
		t.Skip("ai-tools repository not found")
	}
	gitDir := filepath.Join(expandedPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Skip("ai-tools directory is not a git repository")
	}

	branch, err := getCurrentBranch(expandedPath)
	assert.NoError(t, err)
	assert.NotEmpty(t, branch, "Branch name should not be empty")
	assert.NotEqual(t, "unknown", branch)
}

func TestGetCurrentBranch_InvalidRepo(t *testing.T) {
	// Test with non-existent directory
	branch, err := getCurrentBranch("/nonexistent/directory")
	assert.Error(t, err)
	assert.Empty(t, branch)
}

func TestGetUncommittedCount_CleanRepo(t *testing.T) {
	// This test would need a clean git repo
	// For now, we'll just test error handling
	count, err := getUncommittedCount("/nonexistent/directory")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
}

func TestGetUncommittedCount_ValidRepo(t *testing.T) {
	// Use the worktree as a known git directory
	gitDir := "~/src/ws/oss/worktrees/ai-tools/agm-test-coverage"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Skip("worktree directory not found")
	}

	count, err := getUncommittedCount(gitDir)
	assert.NoError(t, err)
	// Count can be 0 or more; just check no error
	assert.GreaterOrEqual(t, count, 0)
}

func TestIsWorktree(t *testing.T) {
	t.Run("actual worktree", func(t *testing.T) {
		// The test is running in a worktree
		worktreeDir := "~/src/ws/oss/worktrees/ai-tools/agm-test-coverage"
		if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
			t.Skip("worktree directory not found")
		}
		result := IsWorktree(worktreeDir)
		// This may or may not be a worktree depending on setup
		// Just ensure it doesn't panic
		_ = result
	})

	t.Run("non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := IsWorktree(tmpDir)
		assert.False(t, result, "temp dir should not be a worktree")
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		result := IsWorktree("/nonexistent/dir/xyz-99999")
		assert.False(t, result, "nonexistent dir should not be a worktree")
	})
}

func TestAggregateWorkspaceStatus_EmptyWorkspace(t *testing.T) {
	// Use MockAdapter for testing
	adapter := dolt.NewMockAdapter()
	defer adapter.Close()

	ws, err := AggregateWorkspaceStatus(adapter, "test-workspace")
	require.NoError(t, err)

	assert.Equal(t, "test-workspace", ws.Workspace)
	assert.Equal(t, 0, ws.TotalSessions)
	assert.Equal(t, 0, ws.DoneSessions)
	assert.Equal(t, 0, ws.WorkingSessions)
	assert.Empty(t, ws.Sessions)
}

func TestAggregateWorkspaceStatus_MultipleWorkspaces(t *testing.T) {
	// Use MockAdapter for testing
	adapter := dolt.NewMockAdapter()
	defer adapter.Close()

	tmpDir := t.TempDir()

	// Create manifests for different workspaces
	manifests := []*manifest.Manifest{
		{
			SessionID: "session-1",
			Name:      "session-1",
			Workspace: "workspace-a",
			State:     manifest.StateDone,
			Context: manifest.Context{
				Project: tmpDir,
			},
			Tmux: manifest.Tmux{
				SessionName: "session-1",
			},
		},
		{
			SessionID: "session-2",
			Name:      "session-2",
			Workspace: "workspace-a",
			State:     manifest.StateWorking,
			Context: manifest.Context{
				Project: tmpDir,
			},
			Tmux: manifest.Tmux{
				SessionName: "session-2",
			},
		},
		{
			SessionID: "session-3",
			Name:      "session-3",
			Workspace: "workspace-b",
			State:     manifest.StateDone,
			Context: manifest.Context{
				Project: tmpDir,
			},
			Tmux: manifest.Tmux{
				SessionName: "session-3",
			},
		},
	}

	// Insert manifests into MockAdapter
	for _, m := range manifests {
		err := adapter.CreateSession(m)
		require.NoError(t, err)
	}

	// Aggregate workspace-a only
	ws, err := AggregateWorkspaceStatus(adapter, "workspace-a")
	require.NoError(t, err)

	assert.Equal(t, "workspace-a", ws.Workspace)
	assert.Equal(t, 2, ws.TotalSessions, "Should only include workspace-a sessions")
	// No tmux sessions exist → all resolve to OFFLINE via ResolveSessionState
	assert.Equal(t, 0, ws.DoneSessions)
	assert.Equal(t, 0, ws.WorkingSessions)
}

func TestAggregateWorkspaceStatus_AllWorkspaces(t *testing.T) {
	// Use MockAdapter for testing
	adapter := dolt.NewMockAdapter()
	defer adapter.Close()

	tmpDir := t.TempDir()

	// Create manifests for different workspaces
	manifests := []*manifest.Manifest{
		{
			SessionID: "session-1",
			Name:      "session-1",
			Workspace: "workspace-a",
			State:     manifest.StateDone,
			Context: manifest.Context{
				Project: tmpDir,
			},
			Tmux: manifest.Tmux{
				SessionName: "session-1",
			},
		},
		{
			SessionID: "session-2",
			Name:      "session-2",
			Workspace: "workspace-b",
			State:     manifest.StateDone,
			Context: manifest.Context{
				Project: tmpDir,
			},
			Tmux: manifest.Tmux{
				SessionName: "session-2",
			},
		},
	}

	// Insert manifests into MockAdapter
	for _, m := range manifests {
		err := adapter.CreateSession(m)
		require.NoError(t, err)
	}

	// Aggregate all workspaces (empty filter)
	ws, err := AggregateWorkspaceStatus(adapter, "")
	require.NoError(t, err)

	assert.Equal(t, "", ws.Workspace)
	assert.Equal(t, 2, ws.TotalSessions, "Should include all workspaces")
	// No tmux sessions exist → all resolve to OFFLINE
	assert.Equal(t, 0, ws.DoneSessions)
}

// TestStateConsistency_AllPathsAgree verifies that CollectStatus (used by
// `agm session status`), ComputeStatus (used by `agm session list`), and
// ResolveSessionState (used by `agm send msg`) all agree on state when
// given the same manifest data.
//
// This is the regression test for the P0 status inconsistency where:
// - list showed "active" / empty state
// - send_msg detected "DONE" but queued instead of delivering
// - status showed "OFFLINE" because empty m.State defaulted to OFFLINE
func TestStateConsistency_AllPathsAgree(t *testing.T) {
	t.Run("non-existent tmux session — all paths report OFFLINE", func(t *testing.T) {
		m := &manifest.Manifest{
			Name:      "consistency-test-offline",
			SessionID: "ct-offline",
			State:     manifest.StateDone, // Stale manifest state
			Tmux:      manifest.Tmux{SessionName: "agm-nonexistent-consistency-xyz"},
			Context:   manifest.Context{Project: "/tmp"},
		}

		// Path 1: CollectStatus (agm session status)
		collected, err := CollectStatus(m)
		require.NoError(t, err)
		assert.Equal(t, manifest.StateOffline, collected.State,
			"CollectStatus should report OFFLINE for non-existent tmux session")

		// Path 2: ComputeStatus (agm session list)
		mockTmux := NewMockTmux()
		computedStatus := ComputeStatus(m, mockTmux)
		assert.Equal(t, "stopped", computedStatus,
			"ComputeStatus should report 'stopped' for non-existent tmux session")

		// Path 3: ResolveSessionState (agm send msg, statusline)
		resolvedState := ResolveSessionState("agm-nonexistent-consistency-xyz",
			m.State, m.Claude.UUID, m.StateUpdatedAt)
		assert.Equal(t, manifest.StateOffline, resolvedState,
			"ResolveSessionState should report OFFLINE for non-existent tmux session")

		// Consistency: all paths agree the session is not running
		assert.Equal(t, collected.State, resolvedState,
			"CollectStatus and ResolveSessionState must agree")
	})

	t.Run("empty manifest state — resolves to OFFLINE without tmux", func(t *testing.T) {
		m := &manifest.Manifest{
			Name:      "consistency-test-empty",
			SessionID: "ct-empty",
			State:     "", // Empty state — was the root cause of the bug
			Tmux:      manifest.Tmux{SessionName: "agm-nonexistent-empty-xyz"},
			Context:   manifest.Context{Project: "/tmp"},
		}

		// All paths should agree: OFFLINE
		collected, err := CollectStatus(m)
		require.NoError(t, err)
		assert.Equal(t, manifest.StateOffline, collected.State,
			"Empty state should resolve to OFFLINE, not default blindly")

		resolved := ResolveSessionState("agm-nonexistent-empty-xyz", "", "", time.Time{})
		assert.Equal(t, manifest.StateOffline, resolved)

		assert.Equal(t, collected.State, resolved,
			"CollectStatus and ResolveSessionState must agree on empty state")
	})

	t.Run("stale manifest state — resolves consistently", func(t *testing.T) {
		staleTime := time.Now().Add(-5 * time.Minute)
		m := &manifest.Manifest{
			Name:           "consistency-test-stale",
			SessionID:      "ct-stale",
			State:          manifest.StateWorking, // Stale: 5 minutes old
			StateUpdatedAt: staleTime,
			Tmux:           manifest.Tmux{SessionName: "agm-nonexistent-stale-xyz"},
			Context:        manifest.Context{Project: "/tmp"},
		}

		collected, err := CollectStatus(m)
		require.NoError(t, err)

		resolved := ResolveSessionState("agm-nonexistent-stale-xyz",
			m.State, m.Claude.UUID, staleTime)

		assert.Equal(t, collected.State, resolved,
			"CollectStatus and ResolveSessionState must agree on stale state")
	})
}

// TestResolveSessionState_DONEMeansReady verifies that DONE state
// represents a session at an idle prompt, ready for message delivery.
// This is the semantic test for the send_msg fix.
func TestResolveSessionState_DONEMeansReady(t *testing.T) {
	// When ResolveSessionState returns DONE, it means:
	// 1. tmux session exists
	// 2. Session is at the prompt (idle)
	// 3. Messages should be sent directly (not queued)
	//
	// We can't create real tmux sessions in unit tests,
	// but we verify the contract via the non-tmux paths.

	// Without tmux, DONE is never returned (OFFLINE instead)
	state := ResolveSessionState("agm-nonexistent-done-test",
		manifest.StateDone, "", time.Now())
	assert.Equal(t, manifest.StateOffline, state,
		"DONE should not be returned without a tmux session")

	// The semantic: DONE is only returned when tmux session exists
	// and the session is at the prompt. This is enforced by
	// ResolveSessionState's tmux.HasSession check.
}
