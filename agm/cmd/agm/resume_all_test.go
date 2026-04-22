package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// TestFilterNonArchived verifies archived sessions are filtered out
func TestFilterNonArchived(t *testing.T) {
	tests := []struct {
		name     string
		input    []*manifest.Manifest
		expected int // expected count after filtering
	}{
		{
			name: "filters archived sessions",
			input: []*manifest.Manifest{
				testManifest("session-1", "active"),
				testManifest("session-2", "active"),
				testManifestArchived("session-archived"),
			},
			expected: 2,
		},
		{
			name: "no archived sessions",
			input: []*manifest.Manifest{
				testManifest("session-1", "active"),
				testManifest("session-2", "active"),
			},
			expected: 2,
		},
		{
			name: "all archived",
			input: []*manifest.Manifest{
				testManifestArchived("session-1"),
				testManifestArchived("session-2"),
			},
			expected: 0,
		},
		{
			name:     "empty list",
			input:    []*manifest.Manifest{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterNonArchived(tt.input)
			assert.Equal(t, tt.expected, len(result), "unexpected number of sessions after filtering")

			// Verify no archived sessions in result
			for _, m := range result {
				assert.NotEqual(t, manifest.LifecycleArchived, m.Lifecycle,
					"archived session should not be in result: %s", m.Name)
			}
		})
	}
}

// TestFilterByWorkspace verifies workspace filtering
func TestFilterByWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		input     []*manifest.Manifest
		workspace string
		expected  []string // expected session names
	}{
		{
			name: "filters by workspace",
			input: []*manifest.Manifest{
				testManifestWorkspace("session-1", "alpha"),
				testManifestWorkspace("session-2", "beta"),
				testManifestWorkspace("session-3", "alpha"),
			},
			workspace: "alpha",
			expected:  []string{"session-1", "session-3"},
		},
		{
			name: "no matches",
			input: []*manifest.Manifest{
				testManifestWorkspace("session-1", "alpha"),
				testManifestWorkspace("session-2", "beta"),
			},
			workspace: "gamma",
			expected:  []string{},
		},
		{
			name: "all match",
			input: []*manifest.Manifest{
				testManifestWorkspace("session-1", "alpha"),
				testManifestWorkspace("session-2", "alpha"),
			},
			workspace: "alpha",
			expected:  []string{"session-1", "session-2"},
		},
		{
			name:      "empty list",
			input:     []*manifest.Manifest{},
			workspace: "alpha",
			expected:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterByWorkspace(tt.input, tt.workspace)
			assert.Equal(t, len(tt.expected), len(result), "unexpected number of sessions")

			names := make([]string, len(result))
			for i, m := range result {
				names[i] = m.Name
			}
			assert.ElementsMatch(t, tt.expected, names, "unexpected sessions in result")

			// Verify all results have correct workspace
			for _, m := range result {
				assert.Equal(t, tt.workspace, m.Workspace,
					"session %s has wrong workspace", m.Name)
			}
		})
	}
}

// TestResumeAllFiltering_Integration tests full filtering pipeline
func TestResumeAllFiltering_Integration(t *testing.T) {
	// Create mix of sessions: active, stopped, archived, different workspaces
	sessions := []*manifest.Manifest{
		testManifestWorkspace("active-alpha", "alpha"),
		testManifestWorkspace("stopped-alpha", "alpha"),
		testManifestWorkspace("active-beta", "beta"),
		testManifestWorkspace("stopped-beta", "beta"),
		testManifestArchivedWorkspace("archived-alpha", "alpha"),
	}

	// Set up mock tmux with active sessions
	mockTmux := session.NewMockTmux()
	mockTmux.Sessions["active-alpha"] = true
	mockTmux.Sessions["active-beta"] = true

	// Test 1: Filter archived, then workspace=alpha, then stopped
	filtered := filterNonArchived(sessions)
	assert.Equal(t, 4, len(filtered), "should filter out archived")

	filteredWorkspace := filterByWorkspace(filtered, "alpha")
	assert.Equal(t, 2, len(filteredWorkspace), "should filter to alpha workspace")

	// Compute status to find stopped
	statuses := session.ComputeStatusBatch(filteredWorkspace, mockTmux)
	var stopped []*manifest.Manifest
	for _, m := range filteredWorkspace {
		if statuses[m.Name] == "stopped" {
			stopped = append(stopped, m)
		}
	}

	assert.Equal(t, 1, len(stopped), "should have 1 stopped session")
	assert.Equal(t, "stopped-alpha", stopped[0].Name, "wrong session filtered")
}

// TestResumeAllFiltering_EdgeCases tests edge cases
func TestResumeAllFiltering_EdgeCases(t *testing.T) {
	t.Run("empty workspace filter matches empty workspace sessions", func(t *testing.T) {
		sessions := []*manifest.Manifest{
			testManifestWorkspace("session-1", ""),
			testManifestWorkspace("session-2", "alpha"),
		}

		result := filterByWorkspace(sessions, "")
		assert.Equal(t, 1, len(result), "should match empty workspace")
		assert.Equal(t, "session-1", result[0].Name)
	})

	t.Run("case sensitive workspace matching", func(t *testing.T) {
		sessions := []*manifest.Manifest{
			testManifestWorkspace("session-1", "Alpha"),
			testManifestWorkspace("session-2", "alpha"),
		}

		result := filterByWorkspace(sessions, "alpha")
		assert.Equal(t, 1, len(result), "workspace matching should be case-sensitive")
		assert.Equal(t, "session-2", result[0].Name)
	})

	t.Run("nil manifest list", func(t *testing.T) {
		result := filterNonArchived(nil)
		assert.Empty(t, result, "should handle nil gracefully")

		result = filterByWorkspace(nil, "alpha")
		assert.Empty(t, result, "should handle nil gracefully")
	})
}

// Helper functions

func testManifest(name, tmuxName string) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-" + name,
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: filepath.Join("/tmp/test", name),
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
		Workspace: "",
		Harness:   "claude-code",
	}
}

func testManifestArchived(name string) *manifest.Manifest {
	m := testManifest(name, name)
	m.Lifecycle = manifest.LifecycleArchived
	return m
}

func testManifestWorkspace(name, workspace string) *manifest.Manifest {
	m := testManifest(name, name)
	m.Workspace = workspace
	return m
}

func testManifestArchivedWorkspace(name, workspace string) *manifest.Manifest {
	m := testManifest(name, name)
	m.Lifecycle = manifest.LifecycleArchived
	m.Workspace = workspace
	return m
}

// Benchmark tests

func BenchmarkFilterNonArchived(b *testing.B) {
	// Create 100 manifests (20% archived)
	manifests := make([]*manifest.Manifest, 100)
	for i := 0; i < 100; i++ {
		if i%5 == 0 {
			manifests[i] = testManifestArchived("session-" + string(rune(i)))
		} else {
			manifests[i] = testManifest("session-"+string(rune(i)), "session-"+string(rune(i)))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterNonArchived(manifests)
	}
}

func BenchmarkFilterByWorkspace(b *testing.B) {
	// Create 100 manifests across 5 workspaces
	manifests := make([]*manifest.Manifest, 100)
	workspaces := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	for i := 0; i < 100; i++ {
		manifests[i] = testManifestWorkspace("session-"+string(rune(i)), workspaces[i%5])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterByWorkspace(manifests, "alpha")
	}
}

// TestWriteResumeTimestamp verifies orchestrator integration (ADR-010)
func TestWriteResumeTimestamp(t *testing.T) {
	// Setup: temporary sessions directory
	tmpDir := t.TempDir()

	// Save original config and restore after test
	oldCfg := cfg
	cfg = &config.Config{
		SessionsDir: tmpDir,
	}
	defer func() { cfg = oldCfg }()

	sessionID := "test-session-123"
	sessionDir := filepath.Join(tmpDir, sessionID)
	assert.NoError(t, os.MkdirAll(sessionDir, 0755))

	// Test: Write resume timestamp
	err := writeResumeTimestamp(sessionID)
	assert.NoError(t, err, "writeResumeTimestamp should succeed")

	// Verify: Timestamp file exists
	timestampFile := filepath.Join(sessionDir, ".agm", "resume-timestamp")
	assert.FileExists(t, timestampFile, "resume-timestamp file should exist")

	// Verify: File contains valid RFC3339 timestamp
	data, err := os.ReadFile(timestampFile)
	assert.NoError(t, err, "should read timestamp file")

	timestamp := string(data)
	_, err = time.Parse(time.RFC3339, timestamp)
	assert.NoError(t, err, "timestamp should be valid RFC3339 format")

	// Verify: Timestamp is recent (within last 5 seconds)
	parsedTime, _ := time.Parse(time.RFC3339, timestamp)
	timeDiff := time.Since(parsedTime)
	assert.Less(t, timeDiff, 5*time.Second, "timestamp should be recent")
}

// TestWriteResumeTimestamp_ErrorHandling tests error scenarios
func TestWriteResumeTimestamp_ErrorHandling(t *testing.T) {
	t.Run("creates .agm directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldCfg := cfg
		cfg = &config.Config{
			SessionsDir: tmpDir,
		}
		defer func() { cfg = oldCfg }()

		sessionID := "test-session-456"
		sessionDir := filepath.Join(tmpDir, sessionID)
		assert.NoError(t, os.MkdirAll(sessionDir, 0755))

		// .agm directory doesn't exist yet
		agmDir := filepath.Join(sessionDir, ".agm")
		assert.NoDirExists(t, agmDir, ".agm directory should not exist yet")

		// Write timestamp (should create .agm directory)
		err := writeResumeTimestamp(sessionID)
		assert.NoError(t, err)

		// Verify directory was created
		assert.DirExists(t, agmDir, ".agm directory should be created")
	})

	t.Run("overwrites existing timestamp", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldCfg := cfg
		cfg = &config.Config{
			SessionsDir: tmpDir,
		}
		defer func() { cfg = oldCfg }()

		sessionID := "test-session-789"
		sessionDir := filepath.Join(tmpDir, sessionID)
		agmDir := filepath.Join(sessionDir, ".agm")
		assert.NoError(t, os.MkdirAll(agmDir, 0755))

		timestampFile := filepath.Join(agmDir, "resume-timestamp")

		// Write old timestamp
		oldTimestamp := "2020-01-01T00:00:00Z"
		assert.NoError(t, os.WriteFile(timestampFile, []byte(oldTimestamp), 0644))

		// Write new timestamp
		time.Sleep(10 * time.Millisecond) // Ensure time difference
		err := writeResumeTimestamp(sessionID)
		assert.NoError(t, err)

		// Verify new timestamp is different and recent
		data, _ := os.ReadFile(timestampFile)
		newTimestamp := string(data)
		assert.NotEqual(t, oldTimestamp, newTimestamp, "timestamp should be updated")

		parsedTime, _ := time.Parse(time.RFC3339, newTimestamp)
		timeDiff := time.Since(parsedTime)
		assert.Less(t, timeDiff, 5*time.Second, "new timestamp should be recent")
	})
}
