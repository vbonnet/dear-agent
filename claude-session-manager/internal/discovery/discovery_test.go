package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
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
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	session := &claude.Session{
		UUID:    uuid.New().String(),
		Project: "/home/user/project",
	}

	sessionID := uuid.New().String()
	tmuxName := "test-session"

	m, err := CreateManifest(session, sessionsDir, tmuxName, sessionID)
	require.NoError(t, err, "Should create manifest")
	require.NotNil(t, m)

	// Verify manifest fields
	assert.Equal(t, manifest.SchemaVersion, m.SchemaVersion)
	assert.Equal(t, sessionID, m.SessionID)
	assert.Equal(t, tmuxName, m.Name)
	assert.Equal(t, session.UUID, m.Claude.UUID)
	assert.Equal(t, session.Project, m.Context.Project)
	assert.Equal(t, tmuxName, m.Tmux.SessionName)

	// Verify manifest file was created
	manifestPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	_, err = os.Stat(manifestPath)
	assert.NoError(t, err, "Manifest file should exist")

	// Read back and verify
	readManifest, err := manifest.Read(manifestPath)
	require.NoError(t, err, "Should read manifest")
	assert.Equal(t, sessionID, readManifest.SessionID)
	assert.Equal(t, session.UUID, readManifest.Claude.UUID)
}

// TestCreateManifest_DirectoryCreation tests directory creation
func TestCreateManifest_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions", "nested", "deep")

	session := &claude.Session{
		UUID:    uuid.New().String(),
		Project: "/project",
	}

	sessionID := uuid.New().String()

	_, err := CreateManifest(session, sessionsDir, "test", sessionID)
	require.NoError(t, err, "Should create nested directories")

	// Verify directory structure was created
	expectedDir := filepath.Join(sessionsDir, sessionID)
	info, err := os.Stat(expectedDir)
	require.NoError(t, err, "Directory should exist")
	assert.True(t, info.IsDir(), "Should be a directory")
}

// TestGetTmuxMapping tests reading tmux mappings from manifests
func TestGetTmuxMapping(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create test manifests
	uuid1 := uuid.New().String()
	uuid2 := uuid.New().String()

	manifests := []struct {
		sessionID string
		tmuxName  string
		claudeUUID string
	}{
		{uuid1, "tmux-session-1", uuid.New().String()},
		{uuid2, "tmux-session-2", uuid.New().String()},
	}

	for _, m := range manifests {
		mf := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     m.sessionID,
			Name:          m.tmuxName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/test/project",
			},
			Claude: manifest.Claude{
				UUID: m.claudeUUID,
			},
			Tmux: manifest.Tmux{
				SessionName: m.tmuxName,
			},
		}

		dir := filepath.Join(sessionsDir, m.sessionID)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		err = manifest.Write(filepath.Join(dir, "manifest.yaml"), mf)
		require.NoError(t, err)
	}

	// Get mapping
	mapping, err := GetTmuxMapping(sessionsDir)
	require.NoError(t, err, "Should get mapping")

	// Verify mapping
	// Note: GetTmuxMapping maps SessionID -> Tmux.SessionName (not Claude.UUID -> Tmux.SessionName)
	assert.Len(t, mapping, 2, "Should have 2 mappings")
	assert.Equal(t, "tmux-session-1", mapping[uuid1])
	assert.Equal(t, "tmux-session-2", mapping[uuid2])
}

// TestGetTmuxMapping_EmptyDirectory tests empty sessions directory
func TestGetTmuxMapping_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "empty-sessions")

	// Create empty directory
	err := os.MkdirAll(sessionsDir, 0755)
	require.NoError(t, err)

	mapping, err := GetTmuxMapping(sessionsDir)
	require.NoError(t, err, "Should not error on empty directory")
	assert.Empty(t, mapping, "Should return empty mapping")
}

// TestGetTmuxMapping_NonExistentDirectory tests missing directory
func TestGetTmuxMapping_NonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "does-not-exist")

	mapping, err := GetTmuxMapping(sessionsDir)
	// Should either error or return empty mapping
	if err != nil {
		assert.Contains(t, err.Error(), "does-not-exist")
	} else {
		assert.Empty(t, mapping)
	}
}

// TestGetTmuxMapping_InvalidManifests tests handling of invalid manifest files
func TestGetTmuxMapping_InvalidManifests(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create a valid manifest
	validUUID := uuid.New().String()
	validDir := filepath.Join(sessionsDir, "valid")
	err := os.MkdirAll(validDir, 0755)
	require.NoError(t, err)

	validManifest := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     validUUID,
		Name:          "valid-session",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: uuid.New().String(),
		},
		Tmux: manifest.Tmux{
			SessionName: "valid-tmux",
		},
	}
	err = manifest.Write(filepath.Join(validDir, "manifest.yaml"), validManifest)
	require.NoError(t, err)

	// Create an invalid manifest (malformed YAML)
	invalidDir := filepath.Join(sessionsDir, "invalid")
	err = os.MkdirAll(invalidDir, 0755)
	require.NoError(t, err)

	invalidContent := []byte("invalid: yaml: content: [unclosed")
	err = os.WriteFile(filepath.Join(invalidDir, "manifest.yaml"), invalidContent, 0644)
	require.NoError(t, err)

	// GetTmuxMapping should skip invalid manifests and continue
	mapping, err := GetTmuxMapping(sessionsDir)
	require.NoError(t, err, "Should not error on invalid manifests")

	// Should have the valid one
	assert.Len(t, mapping, 1, "Should have 1 valid mapping (invalid skipped)")
	assert.Equal(t, "valid-tmux", mapping[validUUID])
}

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
