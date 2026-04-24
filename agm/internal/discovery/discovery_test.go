package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
)

// TestMatchToManifests_AllMatched tests when all sessions have manifests
func TestMatchToManifests_AllMatched(t *testing.T) {
	uuid1 := uuid.New().String()
	uuid2 := uuid.New().String()

	sessions := []*claude.Session{
		{UUID: uuid1, Project: "/project1"},
		{UUID: uuid2, Project: "/project2"},
	}

	manifests := []*manifest.Manifest{
		{SessionID: uuid1, Name: "session1"},
		{SessionID: uuid2, Name: "session2"},
	}

	result := MatchToManifests(sessions, manifests)

	assert.Len(t, result.Matched, 2, "Should match both sessions")
	assert.Empty(t, result.OrphanedClaude, "Should have no orphaned Claude sessions")
	assert.Empty(t, result.OrphanedManifest, "Should have no orphaned manifests")

	assert.Contains(t, result.Matched, uuid1)
	assert.Contains(t, result.Matched, uuid2)
}

// TestMatchToManifests_OrphanedClaude tests sessions without manifests
func TestMatchToManifests_OrphanedClaude(t *testing.T) {
	uuid1 := uuid.New().String()
	uuid2 := uuid.New().String()
	uuid3 := uuid.New().String() // No manifest for this

	sessions := []*claude.Session{
		{UUID: uuid1, Project: "/project1"},
		{UUID: uuid2, Project: "/project2"},
		{UUID: uuid3, Project: "/project3"}, // Orphaned
	}

	manifests := []*manifest.Manifest{
		{SessionID: uuid1, Name: "session1"},
		{SessionID: uuid2, Name: "session2"},
		// uuid3 has no manifest
	}

	result := MatchToManifests(sessions, manifests)

	assert.Len(t, result.Matched, 2, "Should match two sessions")
	assert.Len(t, result.OrphanedClaude, 1, "Should have one orphaned Claude session")
	assert.Empty(t, result.OrphanedManifest, "Should have no orphaned manifests")

	assert.Equal(t, uuid3, result.OrphanedClaude[0].UUID)
}

// TestMatchToManifests_OrphanedManifest tests manifests without sessions
func TestMatchToManifests_OrphanedManifest(t *testing.T) {
	uuid1 := uuid.New().String()
	uuid2 := uuid.New().String()
	uuid3 := uuid.New().String() // Manifest exists but session doesn't

	sessions := []*claude.Session{
		{UUID: uuid1, Project: "/project1"},
		{UUID: uuid2, Project: "/project2"},
		// uuid3 has no session
	}

	manifests := []*manifest.Manifest{
		{SessionID: uuid1, Name: "session1"},
		{SessionID: uuid2, Name: "session2"},
		{SessionID: uuid3, Name: "session3"}, // Orphaned
	}

	result := MatchToManifests(sessions, manifests)

	assert.Len(t, result.Matched, 2, "Should match two sessions")
	assert.Empty(t, result.OrphanedClaude, "Should have no orphaned Claude sessions")
	assert.Len(t, result.OrphanedManifest, 1, "Should have one orphaned manifest")

	assert.Equal(t, uuid3, result.OrphanedManifest[0].SessionID)
}

// TestMatchToManifests_EmptyInputs tests edge cases with empty inputs
func TestMatchToManifests_EmptyInputs(t *testing.T) {
	tests := []struct {
		name         string
		sessions     []*claude.Session
		manifests    []*manifest.Manifest
		wantMatched  int
		wantOrphaned int
	}{
		{
			name:         "no sessions, no manifests",
			sessions:     []*claude.Session{},
			manifests:    []*manifest.Manifest{},
			wantMatched:  0,
			wantOrphaned: 0,
		},
		{
			name: "sessions only",
			sessions: []*claude.Session{
				{UUID: uuid.New().String(), Project: "/project"},
			},
			manifests:    []*manifest.Manifest{},
			wantMatched:  0,
			wantOrphaned: 1,
		},
		{
			name:     "manifests only",
			sessions: []*claude.Session{},
			manifests: []*manifest.Manifest{
				{SessionID: uuid.New().String(), Name: "session"},
			},
			wantMatched:  0,
			wantOrphaned: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchToManifests(tt.sessions, tt.manifests)
			assert.Len(t, result.Matched, tt.wantMatched)

			totalOrphaned := len(result.OrphanedClaude) + len(result.OrphanedManifest)
			assert.Equal(t, tt.wantOrphaned, totalOrphaned)
		})
	}
}

// TestCreateManifest tests manifest creation for orphaned sessions
func TestCreateManifest(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	sessionID := "test-create-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sessionID)

	session := &claude.Session{
		UUID:    uuid.New().String(),
		Project: "~/project",
	}

	tmuxName := "test-session"

	m, err := CreateManifest(session, sessionsDir, tmuxName, sessionID, adapter)
	require.NoError(t, err, "Should create manifest")
	require.NotNil(t, m)

	// Verify manifest fields
	assert.Equal(t, manifest.SchemaVersion, m.SchemaVersion)
	assert.Equal(t, sessionID, m.SessionID)
	assert.Equal(t, tmuxName, m.Name)
	assert.Equal(t, session.UUID, m.Claude.UUID)
	assert.Equal(t, session.Project, m.Context.Project)
	assert.Equal(t, tmuxName, m.Tmux.SessionName)

	// Verify session was persisted in Dolt
	retrieved, err := adapter.GetSession(sessionID)
	require.NoError(t, err, "Should retrieve session from Dolt")
	assert.Equal(t, sessionID, retrieved.SessionID)
	assert.Equal(t, session.UUID, retrieved.Claude.UUID)
}

// TestCreateManifest_DirectoryCreation tests that CreateManifest works with nested paths
func TestCreateManifest_DirectoryCreation(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions", "nested", "deep")

	sessionID := "test-dircreat-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sessionID)

	session := &claude.Session{
		UUID:    uuid.New().String(),
		Project: "/project",
	}

	m, err := CreateManifest(session, sessionsDir, "test", sessionID, adapter)
	require.NoError(t, err, "Should create manifest in Dolt")
	require.NotNil(t, m)

	// Verify session was persisted in Dolt
	retrieved, err := adapter.GetSession(sessionID)
	require.NoError(t, err, "Should retrieve session from Dolt")
	assert.Equal(t, sessionID, retrieved.SessionID)
}

// TestGetTmuxMapping tests reading tmux mappings from Dolt
func TestGetTmuxMapping(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	sid1 := "test-tmux1-" + uuid.New().String()[:8]
	sid2 := "test-tmux2-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)

	sessions := []struct {
		sessionID string
		tmuxName  string
	}{
		{sid1, "tmux-session-1"},
		{sid2, "tmux-session-2"},
	}

	for _, s := range sessions {
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     s.sessionID,
			Name:          s.tmuxName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Workspace:     "oss",
			Context:       manifest.Context{Project: "/test/project"},
			Claude:        manifest.Claude{UUID: uuid.New().String()},
			Tmux:          manifest.Tmux{SessionName: s.tmuxName},
		}
		require.NoError(t, adapter.CreateSession(m))
	}

	// Get mapping using adapter
	mapping, err := GetTmuxMappingWithAdapter(sessionsDir, adapter)
	require.NoError(t, err, "Should get mapping")

	// Verify our test sessions are in the mapping
	assert.Equal(t, "tmux-session-1", mapping[sid1])
	assert.Equal(t, "tmux-session-2", mapping[sid2])
}

// TestGetTmuxMapping_EmptyDirectory tests empty sessions directory
func TestGetTmuxMapping_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "empty-sessions")

	// Create empty directory
	err := os.MkdirAll(sessionsDir, 0755)
	require.NoError(t, err)

	mapping, err := GetTmuxMapping(sessionsDir, nil)
	require.NoError(t, err, "Should not error on empty directory")
	assert.Empty(t, mapping, "Should return empty mapping")
}

// TestGetTmuxMapping_NonExistentDirectory tests missing directory
func TestGetTmuxMapping_NonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "does-not-exist")

	mapping, err := GetTmuxMapping(sessionsDir, nil)
	// Should either error or return empty mapping
	if err != nil {
		assert.Contains(t, err.Error(), "does-not-exist")
	} else {
		assert.Empty(t, mapping)
	}
}

// Phase 6: TestGetTmuxMapping_InvalidManifests deleted - YAML parsing test no longer relevant with Dolt-only architecture

// Benchmark tests
func BenchmarkMatchToManifests(b *testing.B) {
	// Create test data
	sessions := make([]*claude.Session, 100)
	manifests := make([]*manifest.Manifest, 100)

	for i := 0; i < 100; i++ {
		id := uuid.New().String()
		sessions[i] = &claude.Session{UUID: id, Project: "/project"}
		manifests[i] = &manifest.Manifest{SessionID: id, Name: "session"}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MatchToManifests(sessions, manifests)
	}
}
