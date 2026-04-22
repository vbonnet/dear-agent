package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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
	workspaces := []string{"oss", "acme", "personal"}
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
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Verify results
	if len(result.Locations) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(result.Locations))
	}

	// Should use legacy method (no config file)
	if result.Method != "legacy" {
		t.Errorf("Expected method 'legacy', got '%s'", result.Method)
	}

	// Should have searched directories
	if len(result.DirsSearched) == 0 {
		t.Error("Expected DirsSearched to be populated")
	}

	// Verify workspace names
	found := make(map[string]bool)
	for _, loc := range result.Locations {
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

	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should return empty slice, not error
	if len(result.Locations) != 0 {
		t.Errorf("Expected 0 sessions in empty workspace, got %d", len(result.Locations))
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

	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should skip corrupted manifest, not fail
	if len(result.Locations) != 0 {
		t.Errorf("Expected 0 sessions (corrupted skipped), got %d", len(result.Locations))
	}
}

// TestFindSessionsAcrossWorkspaces_DualDirectoryCheck verifies that discovery
// checks both .agm/sessions (new) and sessions (legacy) directories.
// This prevents regression of the bug where sessions in .agm/sessions were missed.
func TestFindSessionsAcrossWorkspaces_DualDirectoryCheck(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create OSS workspace with .agm/sessions (new location)
	ossSessionsDir := filepath.Join(tmpDir, "src", "ws", "oss", ".agm", "sessions")
	os.MkdirAll(ossSessionsDir, 0700)

	ossSessionDir := filepath.Join(ossSessionsDir, "oss-session")
	os.MkdirAll(ossSessionDir, 0700)

	ossManifest := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-oss",
		Name:          "oss-session",
	}
	data, _ := yaml.Marshal(ossManifest)
	os.WriteFile(filepath.Join(ossSessionDir, "manifest.yaml"), data, 0600)

	// Create Acme Corp workspace with legacy sessions/ directory
	acmeSessionsDir := filepath.Join(tmpDir, "src", "ws", "acme", "sessions")
	os.MkdirAll(acmeSessionsDir, 0700)

	acmeSessionDir := filepath.Join(acmeSessionsDir, "acme-session")
	os.MkdirAll(acmeSessionDir, 0700)

	acmeManifest := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-acme",
		Name:          "acme-session",
	}
	data, _ = yaml.Marshal(acmeManifest)
	os.WriteFile(filepath.Join(acmeSessionDir, "manifest.yaml"), data, 0600)

	// Create Personal workspace with BOTH locations (should find both)
	personalNewDir := filepath.Join(tmpDir, "src", "ws", "personal", ".agm", "sessions")
	os.MkdirAll(personalNewDir, 0700)

	personalNewSessionDir := filepath.Join(personalNewDir, "new-session")
	os.MkdirAll(personalNewSessionDir, 0700)

	newManifest := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-new",
		Name:          "new-session",
	}
	data, _ = yaml.Marshal(newManifest)
	os.WriteFile(filepath.Join(personalNewSessionDir, "manifest.yaml"), data, 0600)

	personalLegacyDir := filepath.Join(tmpDir, "src", "ws", "personal", "sessions")
	os.MkdirAll(personalLegacyDir, 0700)

	personalLegacySessionDir := filepath.Join(personalLegacyDir, "legacy-session")
	os.MkdirAll(personalLegacySessionDir, 0700)

	legacyManifest := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-legacy",
		Name:          "legacy-session",
	}
	data, _ = yaml.Marshal(legacyManifest)
	os.WriteFile(filepath.Join(personalLegacySessionDir, "manifest.yaml"), data, 0600)

	// Test discovery
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should find all 4 sessions (1 from oss, 1 from acme, 2 from personal)
	if len(result.Locations) != 4 {
		t.Errorf("Expected 4 sessions (oss .agm + acme legacy + personal both), got %d", len(result.Locations))
		for _, loc := range result.Locations {
			t.Logf("Found: workspace=%s name=%s path=%s", loc.Workspace, loc.Name, loc.ManifestPath)
		}
	}

	// Verify we found sessions from all expected locations
	foundNames := make(map[string]bool)
	for _, loc := range result.Locations {
		foundNames[loc.Name] = true
	}

	expectedNames := []string{"oss-session", "acme-session", "new-session", "legacy-session"}
	for _, name := range expectedNames {
		if !foundNames[name] {
			t.Errorf("Expected to find session %s, but it was missing", name)
		}
	}
}

// TestFindSessionsAcrossWorkspaces_ConfigBased verifies that discovery reads
// ~/.agm/config.yaml and uses output_dir to find sessions.
func TestFindSessionsAcrossWorkspaces_ConfigBased(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace config with custom output_dir
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	configYAML := `version: 1
default_workspace: oss
workspaces:
  - name: oss
    root: ` + filepath.Join(tmpDir, "src") + `
    output_dir: ` + filepath.Join(tmpDir, "src", "engram-research", ".agm") + `
    enabled: true
`
	os.WriteFile(filepath.Join(agmDir, "config.yaml"), []byte(configYAML), 0600)

	// Create sessions in the output_dir/sessions location
	sessionsDir := filepath.Join(tmpDir, "src", "engram-research", ".agm", "sessions")
	os.MkdirAll(sessionsDir, 0700)

	sessionDir := filepath.Join(sessionsDir, "my-session")
	os.MkdirAll(sessionDir, 0700)

	m := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-config-test",
		Name:          "my-session",
	}
	data, _ := yaml.Marshal(m)
	os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), data, 0600)

	// Test discovery
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should use config method
	if result.Method != "config" {
		t.Errorf("Expected method 'config', got '%s'", result.Method)
	}

	// Should report the config path
	if result.ConfigPath == "" {
		t.Error("Expected ConfigPath to be set")
	}

	// Should have searched output_dir/sessions + root/.agm/sessions + root/sessions
	if len(result.DirsSearched) < 2 {
		t.Errorf("Expected at least 2 dirs searched, got %d: %v", len(result.DirsSearched), result.DirsSearched)
	}

	if len(result.Locations) != 1 {
		t.Errorf("Expected 1 session, got %d", len(result.Locations))
		for _, loc := range result.Locations {
			t.Logf("Found: workspace=%s name=%s path=%s", loc.Workspace, loc.Name, loc.ManifestPath)
		}
	}

	if len(result.Locations) > 0 {
		if result.Locations[0].Workspace != "oss" {
			t.Errorf("Expected workspace 'oss', got '%s'", result.Locations[0].Workspace)
		}
		if result.Locations[0].Name != "my-session" {
			t.Errorf("Expected session name 'my-session', got '%s'", result.Locations[0].Name)
		}
	}
}

// TestFindSessionsAcrossWorkspaces_ConfigDisabledWorkspace verifies that
// disabled workspaces are skipped during config-based discovery.
func TestFindSessionsAcrossWorkspaces_ConfigDisabledWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace config with one disabled workspace
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	configYAML := `version: 1
workspaces:
  - name: active
    root: ` + filepath.Join(tmpDir, "active") + `
    enabled: true
  - name: disabled
    root: ` + filepath.Join(tmpDir, "disabled") + `
    enabled: false
`
	os.WriteFile(filepath.Join(agmDir, "config.yaml"), []byte(configYAML), 0600)

	// Create sessions in both workspaces
	for _, ws := range []string{"active", "disabled"} {
		sessionsDir := filepath.Join(tmpDir, ws, ".agm", "sessions", "test-"+ws)
		os.MkdirAll(sessionsDir, 0700)

		m := manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "uuid-" + ws,
			Name:          "test-" + ws,
		}
		data, _ := yaml.Marshal(m)
		os.WriteFile(filepath.Join(sessionsDir, "manifest.yaml"), data, 0600)
	}

	// Test discovery
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should only find the active workspace's session
	if len(result.Locations) != 1 {
		t.Errorf("Expected 1 session (disabled skipped), got %d", len(result.Locations))
		for _, loc := range result.Locations {
			t.Logf("Found: workspace=%s name=%s", loc.Workspace, loc.Name)
		}
	}

	if len(result.Locations) > 0 && result.Locations[0].Workspace != "active" {
		t.Errorf("Expected workspace 'active', got '%s'", result.Locations[0].Workspace)
	}
}

// TestFindSessionsAcrossWorkspaces_ConfigDriven verifies that config-based
// discovery with multiple workspaces (e.g., "personal" and "oss") finds
// sessions from both workspaces and assigns correct workspace labels.
// This covers the cross-workspace discovery pattern used when brain-v2
// (personal) and engram-research (oss) are both configured in ~/.agm/config.yaml.
func TestFindSessionsAcrossWorkspaces_ConfigDriven(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace config with two workspaces (personal + oss)
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	personalRoot := filepath.Join(tmpDir, "src", "brain-v2")
	ossRoot := filepath.Join(tmpDir, "src", "engram-research")

	configYAML := `version: 1
default_workspace: oss
workspaces:
  - name: personal
    root: ` + personalRoot + `
    output_dir: ` + filepath.Join(personalRoot, ".agm") + `
    enabled: true
  - name: oss
    root: ` + ossRoot + `
    output_dir: ` + filepath.Join(ossRoot, ".agm") + `
    enabled: true
`
	os.WriteFile(filepath.Join(agmDir, "config.yaml"), []byte(configYAML), 0600)

	// Create personal workspace sessions
	personalSessions := []string{"dotfiles-setup", "interview-prep", "fortress-arch"}
	for _, name := range personalSessions {
		sessionDir := filepath.Join(personalRoot, ".agm", "sessions", name)
		os.MkdirAll(sessionDir, 0700)

		m := manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     name,
			Name:          name,
			Lifecycle:     "archived",
			Context:       manifest.Context{Project: "/Users/testuser"},
			Tmux:          manifest.Tmux{SessionName: name},
		}
		data, _ := yaml.Marshal(m)
		os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), data, 0600)
	}

	// Create oss workspace sessions
	ossSessions := []string{"engram-core", "agm-refactor"}
	for _, name := range ossSessions {
		sessionDir := filepath.Join(ossRoot, ".agm", "sessions", name)
		os.MkdirAll(sessionDir, 0700)

		m := manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     name,
			Name:          name,
			Context:       manifest.Context{Project: "/Users/testuser/src/engram"},
			Tmux:          manifest.Tmux{SessionName: name},
		}
		data, _ := yaml.Marshal(m)
		os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), data, 0600)
	}

	// Test discovery
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should use config method
	if result.Method != "config" {
		t.Errorf("Expected method 'config', got '%s'", result.Method)
	}

	// Should find all 5 sessions (3 personal + 2 oss)
	if len(result.Locations) != 5 {
		t.Errorf("Expected 5 sessions, got %d", len(result.Locations))
		for _, loc := range result.Locations {
			t.Logf("Found: workspace=%s name=%s path=%s", loc.Workspace, loc.Name, loc.ManifestPath)
		}
	}

	// Verify workspace labels are correctly assigned
	workspaceCounts := make(map[string]int)
	for _, loc := range result.Locations {
		workspaceCounts[loc.Workspace]++
	}

	if workspaceCounts["personal"] != 3 {
		t.Errorf("Expected 3 personal sessions, got %d", workspaceCounts["personal"])
	}
	if workspaceCounts["oss"] != 2 {
		t.Errorf("Expected 2 oss sessions, got %d", workspaceCounts["oss"])
	}

	// Verify specific session names are present
	foundNames := make(map[string]string) // name -> workspace
	for _, loc := range result.Locations {
		foundNames[loc.Name] = loc.Workspace
	}

	for _, name := range personalSessions {
		if ws, ok := foundNames[name]; !ok {
			t.Errorf("Expected to find personal session %s", name)
		} else if ws != "personal" {
			t.Errorf("Session %s should be in 'personal' workspace, got '%s'", name, ws)
		}
	}

	for _, name := range ossSessions {
		if ws, ok := foundNames[name]; !ok {
			t.Errorf("Expected to find oss session %s", name)
		} else if ws != "oss" {
			t.Errorf("Session %s should be in 'oss' workspace, got '%s'", name, ws)
		}
	}
}

// TestFindSessionsAcrossWorkspaces_ClaudeFallback verifies that discovery
// scans ~/.claude/sessions for non-workspace sessions (new fallback path).
func TestFindSessionsAcrossWorkspaces_ClaudeFallback(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create sessions in ~/.claude/sessions (no workspace)
	claudeSessionsDir := filepath.Join(tmpDir, ".claude", "sessions")
	os.MkdirAll(claudeSessionsDir, 0700)

	sessionNames := []string{"default-session", "no-workspace-session"}
	for _, name := range sessionNames {
		sessionDir := filepath.Join(claudeSessionsDir, name)
		os.MkdirAll(sessionDir, 0700)

		m := manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "uuid-" + name,
			Name:          name,
		}
		data, _ := yaml.Marshal(m)
		os.WriteFile(filepath.Join(sessionDir, "manifest.yaml"), data, 0600)
	}

	// Test discovery (legacy mode, no config)
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should find sessions in ~/.claude/sessions
	if len(result.Locations) != 2 {
		t.Errorf("Expected 2 sessions from ~/.claude/sessions, got %d", len(result.Locations))
	}

	// Verify ~/.claude/sessions was searched
	claudeSessionsDirFound := false
	for _, dir := range result.DirsSearched {
		if dir == claudeSessionsDir {
			claudeSessionsDirFound = true
			break
		}
	}
	if !claudeSessionsDirFound {
		t.Errorf("Expected ~/.claude/sessions in DirsSearched, got: %v", result.DirsSearched)
	}

	// Verify workspace is empty (no workspace assigned)
	for _, loc := range result.Locations {
		if loc.Workspace != "" {
			t.Errorf("Expected empty workspace for ~/.claude/sessions, got '%s'", loc.Workspace)
		}
	}
}

// TestFindSessionsAcrossWorkspaces_ConfigWithClaudeFallback verifies that
// config-based discovery also scans ~/.claude/sessions in addition to workspace paths.
func TestFindSessionsAcrossWorkspaces_ConfigWithClaudeFallback(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create workspace config
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	ossRoot := filepath.Join(tmpDir, "src", "ws", "oss")
	configYAML := `version: 1
workspaces:
  - name: oss
    root: ` + ossRoot + `
    enabled: true
`
	os.WriteFile(filepath.Join(agmDir, "config.yaml"), []byte(configYAML), 0600)

	// Create oss workspace session
	ossSessionDir := filepath.Join(ossRoot, ".agm", "sessions", "oss-session")
	os.MkdirAll(ossSessionDir, 0700)

	ossManifest := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-oss",
		Name:          "oss-session",
	}
	data, _ := yaml.Marshal(ossManifest)
	os.WriteFile(filepath.Join(ossSessionDir, "manifest.yaml"), data, 0600)

	// Create session in ~/.claude/sessions
	claudeSessionDir := filepath.Join(tmpDir, ".claude", "sessions", "default-session")
	os.MkdirAll(claudeSessionDir, 0700)

	defaultManifest := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "uuid-default",
		Name:          "default-session",
	}
	data, _ = yaml.Marshal(defaultManifest)
	os.WriteFile(filepath.Join(claudeSessionDir, "manifest.yaml"), data, 0600)

	// Test discovery
	result, err := FindSessionsAcrossWorkspaces()
	if err != nil {
		t.Fatalf("FindSessionsAcrossWorkspaces failed: %v", err)
	}

	// Should use config method
	if result.Method != "config" {
		t.Errorf("Expected method 'config', got '%s'", result.Method)
	}

	// Should find both workspace session and fallback session
	if len(result.Locations) != 2 {
		t.Errorf("Expected 2 sessions (1 oss + 1 fallback), got %d", len(result.Locations))
		for _, loc := range result.Locations {
			t.Logf("Found: workspace=%s name=%s", loc.Workspace, loc.Name)
		}
	}

	// Verify workspace assignments
	workspaceCounts := make(map[string]int)
	for _, loc := range result.Locations {
		workspaceCounts[loc.Workspace]++
	}

	if workspaceCounts["oss"] != 1 {
		t.Errorf("Expected 1 oss session, got %d", workspaceCounts["oss"])
	}
	if workspaceCounts[""] != 1 {
		t.Errorf("Expected 1 fallback session (empty workspace), got %d", workspaceCounts[""])
	}
}
