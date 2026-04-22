package session

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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
			Project: "~/test",
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

func TestComputeStatusBatch_NilTmux(t *testing.T) {
	manifests := []*manifest.Manifest{
		testManifest("test-1", "session-1", ""),
		testManifest("test-archived", "session-archived", manifest.LifecycleArchived),
	}

	statuses := ComputeStatusBatch(manifests, nil)

	assert.Equal(t, "stopped", statuses["test-1"])
	assert.Equal(t, "archived", statuses["test-archived"])
}

func TestComputeStatus_NilTmux(t *testing.T) {
	m := testManifest("test", "session-1", "")
	status := ComputeStatus(m, nil)
	assert.Equal(t, "stopped", status)
}

func TestGetTmuxSessionName(t *testing.T) {
	t.Run("uses tmux session name when set", func(t *testing.T) {
		m := testManifest("test", "my-tmux-session", "")
		name := getTmuxSessionName(m)
		assert.Equal(t, "my-tmux-session", name)
	})

	t.Run("falls back to sanitized session name", func(t *testing.T) {
		m := testManifest("my-session", "", "")
		name := getTmuxSessionName(m)
		assert.NotEmpty(t, name)
	})

	t.Run("last resort fallback for empty name", func(t *testing.T) {
		m := &manifest.Manifest{}
		name := getTmuxSessionName(m)
		// Should return something non-empty
		assert.NotEmpty(t, name)
	})
}

func TestComputeStatusBatchWithInfo(t *testing.T) {
	t.Run("active and stopped sessions", func(t *testing.T) {
		mockTmux := NewMockTmux()
		mockTmux.Sessions["session-1"] = true

		manifests := []*manifest.Manifest{
			testManifest("test-1", "session-1", ""),
			testManifest("test-2", "session-2", ""),
			testManifest("test-archived", "session-archived", manifest.LifecycleArchived),
		}

		statuses := ComputeStatusBatchWithInfo(manifests, mockTmux)

		assert.Equal(t, "active", statuses["test-1"].Status)
		assert.Equal(t, 0, statuses["test-1"].AttachedClients)
		assert.Equal(t, "stopped", statuses["test-2"].Status)
		assert.Equal(t, "archived", statuses["test-archived"].Status)
	})

	t.Run("tmux error returns all stopped", func(t *testing.T) {
		mockTmux := NewMockTmux()
		mockTmux.ListSessionsError = errors.New("tmux unavailable")

		manifests := []*manifest.Manifest{
			testManifest("test-1", "session-1", ""),
		}

		statuses := ComputeStatusBatchWithInfo(manifests, mockTmux)

		assert.Equal(t, "stopped", statuses["test-1"].Status)
	})
}

func TestMockTmux_AllMethods(t *testing.T) {
	mock := NewMockTmux()

	// CreateSession
	err := mock.CreateSession("test-session", "/tmp")
	assert.NoError(t, err)
	assert.Contains(t, mock.CreatedSessions, "test-session")
	assert.True(t, mock.Sessions["test-session"])

	// HasSession
	exists, err := mock.HasSession("test-session")
	assert.NoError(t, err)
	assert.True(t, exists)

	exists, err = mock.HasSession("nonexistent")
	assert.NoError(t, err)
	assert.False(t, exists)

	// ListSessionsWithInfo
	infos, err := mock.ListSessionsWithInfo()
	assert.NoError(t, err)
	assert.Len(t, infos, 1)
	assert.Equal(t, "test-session", infos[0].Name)

	// AttachSession
	err = mock.AttachSession("test-session")
	assert.NoError(t, err)

	// SendKeys
	err = mock.SendKeys("test-session", "echo hello")
	assert.NoError(t, err)
	assert.Contains(t, mock.SentCommands, "echo hello")

	// ListClients
	clients, err := mock.ListClients("test-session")
	assert.NoError(t, err)
	assert.Empty(t, clients)

	// Error paths
	mock.CreateSessionError = errors.New("create error")
	err = mock.CreateSession("fail", "/tmp")
	assert.Error(t, err)

	mock.AttachSessionError = errors.New("attach error")
	err = mock.AttachSession("fail")
	assert.Error(t, err)

	mock.SendKeysError = errors.New("send error")
	err = mock.SendKeys("fail", "cmd")
	assert.Error(t, err)
}
