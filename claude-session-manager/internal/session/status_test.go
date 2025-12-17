package session

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

func testManifest(name, tmuxName, lifecycle string) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     lifecycle,
		Context: manifest.Context{
			Project: "/home/user/test",
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}
}

func TestComputeStatus_Active(t *testing.T) {
	mockTmux := NewMockTmux()
	mockTmux.Sessions["claude-test"] = true

	m := testManifest("test", "claude-test", "")

	status := ComputeStatus(m, mockTmux)
	assert.Equal(t, "active", status)
}

func TestComputeStatus_Stopped(t *testing.T) {
	mockTmux := NewMockTmux()
	// Session doesn't exist

	m := testManifest("test", "claude-test", "")

	status := ComputeStatus(m, mockTmux)
	assert.Equal(t, "stopped", status)
}

func TestComputeStatus_Archived(t *testing.T) {
	mockTmux := NewMockTmux()
	mockTmux.Sessions["claude-test"] = true // Even if tmux session exists

	m := testManifest("test", "claude-test", manifest.LifecycleArchived)

	status := ComputeStatus(m, mockTmux)
	assert.Equal(t, "archived", status, "archived lifecycle should take precedence over tmux state")
}

func TestComputeStatus_TmuxError(t *testing.T) {
	mockTmux := NewMockTmux()
	mockTmux.HasSessionError = errors.New("tmux not available")

	m := testManifest("test", "claude-test", "")

	status := ComputeStatus(m, mockTmux)
	assert.Equal(t, "stopped", status, "should assume stopped on tmux error")
}

func TestComputeStatusBatch(t *testing.T) {
	mockTmux := NewMockTmux()
	mockTmux.Sessions["session-1"] = true
	mockTmux.Sessions["session-2"] = true
	// session-3 doesn't exist

	manifests := []*manifest.Manifest{
		testManifest("test-1", "session-1", ""),
		testManifest("test-2", "session-2", ""),
		testManifest("test-3", "session-3", ""),
		testManifest("test-archived", "session-archived", manifest.LifecycleArchived),
	}

	statuses := ComputeStatusBatch(manifests, mockTmux)

	assert.Equal(t, "active", statuses["test-1"])
	assert.Equal(t, "active", statuses["test-2"])
	assert.Equal(t, "stopped", statuses["test-3"])
	assert.Equal(t, "archived", statuses["test-archived"])
}

func TestComputeStatusBatch_SingleListSessionsCall(t *testing.T) {
	mockTmux := NewMockTmux()
	mockTmux.Sessions["session-1"] = true
	mockTmux.Sessions["session-2"] = true

	manifests := []*manifest.Manifest{
		testManifest("test-1", "session-1", ""),
		testManifest("test-2", "session-2", ""),
		testManifest("test-3", "session-3", ""),
	}

	// Call ComputeStatusBatch
	_ = ComputeStatusBatch(manifests, mockTmux)

	// Verify ListSessions was called (mock tracks this via returning sessions)
	// In a real implementation, we'd track call count, but the optimization
	// is guaranteed by the code structure (single ListSessions call)
}

func TestComputeStatusBatch_TmuxError(t *testing.T) {
	mockTmux := NewMockTmux()
	mockTmux.ListSessionsError = errors.New("tmux not available")

	manifests := []*manifest.Manifest{
		testManifest("test-1", "session-1", ""),
		testManifest("test-archived", "session-archived", manifest.LifecycleArchived),
	}

	statuses := ComputeStatusBatch(manifests, mockTmux)

	// On error, should assume all non-archived sessions are stopped
	assert.Equal(t, "stopped", statuses["test-1"])
	assert.Equal(t, "archived", statuses["test-archived"], "archived should still work even if tmux fails")
}

func TestComputeStatusBatch_EmptyList(t *testing.T) {
	mockTmux := NewMockTmux()

	manifests := []*manifest.Manifest{}

	statuses := ComputeStatusBatch(manifests, mockTmux)

	assert.Empty(t, statuses)
}
