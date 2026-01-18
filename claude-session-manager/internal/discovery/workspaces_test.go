package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"gopkg.in/yaml.v3"
)

func TestFindSessionsAcrossWorkspaces(t *testing.T) {
	// Create temporary test structure
	tmpDir := t.TempDir()

	// Mock HOME directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace structure
	workspaces := []string{"oss", "[REDACTED_EMPLOYER]", "personal"}
	for _, ws := range workspaces {
		sessionsDir := filepath.Join(tmpDir, "src", "ws", ws, "sessions")
		os.MkdirAll(sessionsDir, 0700)

		// Create test session
		sessionDir := filepath.Join(sessionsDir, "test-session-"+ws)
		os.MkdirAll(sessionDir, 0700)

		// Write manifest
		m := manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "uuid-" + ws,
			Name:          "test-session-" + ws,
		}
		data, _ := yaml.Marshal(m)
		os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), data, 0600)
	}

	// Test discovery
	locations, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Verify results
	if len(locations) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(locations))
	}

	// Verify workspace names
	found := make(map[string]bool)
	for _, loc := range locations {
		found[loc.Workspace] = true
	}

	for _, ws := range workspaces {
		if !found[ws] {
			t.Errorf("Workspace %s not found in results", ws)
		}
	}
}

func TestFindSessionsAcrossWorkspaces_EmptyWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace with no sessions directory
	emptyWsDir := filepath.Join(tmpDir, "src", "ws", "empty")
	os.MkdirAll(emptyWsDir, 0700)

	locations, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should return empty slice, not error
	if len(locations) != 0 {
		t.Errorf("Expected 0 sessions in empty workspace, got %d", len(locations))
	}
}

func TestFindSessionsAcrossWorkspaces_CorruptedManifest(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace with corrupted manifest
	sessionsDir := filepath.Join(tmpDir, "src", "ws", "oss", "sessions")
	os.MkdirAll(sessionsDir, 0700)

	sessionDir := filepath.Join(sessionsDir, "corrupted-session")
	os.MkdirAll(sessionDir, 0700)

	// Write invalid YAML
	os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), []byte("invalid: yaml: data:"), 0600)

	locations, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should skip corrupted manifest, not fail
	if len(locations) != 0 {
		t.Errorf("Expected 0 sessions (corrupted skipped), got %d", len(locations))
	}
}
