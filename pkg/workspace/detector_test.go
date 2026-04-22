package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestDetect_Priority1_ExplicitFlag tests highest priority detection method.
func TestDetect_Priority1_ExplicitFlag(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test-ws", Root: tmpDir, Enabled: true},
			{Name: "other-ws", Root: "/tmp/other", Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Test explicit flag (should ignore all other detection methods)
	ws, err := detector.Detect(tmpDir, "test-ws")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if ws.Name != "test-ws" {
		t.Errorf("expected workspace 'test-ws', got '%s'", ws.Name)
	}
}

// TestDetect_Priority1_ExplicitFlag_NotFound tests explicit flag with unknown workspace.
func TestDetect_Priority1_ExplicitFlag_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "test-ws", Root: tmpDir, Enabled: true}},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	_, err = detector.Detect(tmpDir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace, got nil")
	}
}

// TestDetect_Priority2_EnvVar tests environment variable detection.
func TestDetect_Priority2_EnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "env-ws", Root: tmpDir, Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Set environment variable
	os.Setenv("TEST_WORKSPACE", "env-ws")
	defer os.Unsetenv("TEST_WORKSPACE")

	ws, err := detector.DetectWithEnv(tmpDir, "", "TEST_WORKSPACE")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if ws.Name != "env-ws" {
		t.Errorf("expected workspace 'env-ws', got '%s'", ws.Name)
	}
}

// TestDetect_Priority2_EnvVar_InvalidWorkspace tests env var with invalid workspace.
func TestDetect_Priority2_EnvVar_InvalidWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "env-ws", Root: tmpDir, Enabled: true}},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	os.Setenv("TEST_WORKSPACE", "invalid-ws")
	defer os.Unsetenv("TEST_WORKSPACE")

	_, err = detector.DetectWithEnv(tmpDir, "", "TEST_WORKSPACE")
	if err == nil {
		t.Fatal("expected error for invalid workspace from env, got nil")
	}
}

// TestDetect_Priority3_AutoDetect tests auto-detection from PWD.
func TestDetect_Priority3_AutoDetect(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace root: %v", err)
	}

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "auto-ws", Root: workspaceRoot, Enabled: true}},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Detect from workspace root
	ws, err := detector.Detect(workspaceRoot, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if ws.Name != "auto-ws" {
		t.Errorf("expected workspace 'auto-ws', got '%s'", ws.Name)
	}
}

// TestDetect_Priority3_AutoDetect_NestedDirectories tests auto-detection from subdirectories.
func TestDetect_Priority3_AutoDetect_NestedDirectories(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	nestedPath := filepath.Join(workspaceRoot, "src", "pkg", "module")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		t.Fatalf("failed to create nested path: %v", err)
	}

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "nested-ws", Root: workspaceRoot, Enabled: true}},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Detect from deeply nested directory
	ws, err := detector.Detect(nestedPath, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if ws.Name != "nested-ws" {
		t.Errorf("expected workspace 'nested-ws', got '%s'", ws.Name)
	}
}

// TestDetect_Priority4_Default tests default workspace fallback.
func TestDetect_Priority4_Default(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:          1,
		DefaultWorkspace: "default-ws",
		Workspaces: []Workspace{
			{Name: "default-ws", Root: "/tmp/default", Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Detect from path outside any workspace (should use default)
	ws, err := detector.Detect("/tmp/random", "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if ws.Name != "default-ws" {
		t.Errorf("expected workspace 'default-ws', got '%s'", ws.Name)
	}
}

// TestDetect_Priority6_Error tests error when no workspace found.
func TestDetect_Priority6_Error(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "ws", Root: "/tmp/ws", Enabled: true}},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// No flag, no env, no match, no default, no interactive -> should error
	_, err = detector.Detect("/tmp/random", "")
	if err == nil {
		t.Fatal("expected error when no workspace found, got nil")
	}

	if err != ErrNoWorkspaceFound {
		t.Errorf("expected ErrNoWorkspaceFound, got %v", err)
	}
}

// TestDetectFromPath_MultipleWorkspaces tests detection with multiple configured workspaces.
func TestDetectFromPath_MultipleWorkspaces(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	ws1Root := filepath.Join(tmpDir, "workspace1")
	ws2Root := filepath.Join(tmpDir, "workspace2")
	if err := os.MkdirAll(ws1Root, 0755); err != nil {
		t.Fatalf("failed to create ws1: %v", err)
	}
	if err := os.MkdirAll(ws2Root, 0755); err != nil {
		t.Fatalf("failed to create ws2: %v", err)
	}

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "ws1", Root: ws1Root, Enabled: true},
			{Name: "ws2", Root: ws2Root, Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Test detection in ws1
	ws, err := detector.Detect(ws1Root, "")
	if err != nil {
		t.Fatalf("Detect ws1 failed: %v", err)
	}
	if ws.Name != "ws1" {
		t.Errorf("expected workspace 'ws1', got '%s'", ws.Name)
	}

	// Test detection in ws2
	ws, err = detector.Detect(ws2Root, "")
	if err != nil {
		t.Fatalf("Detect ws2 failed: %v", err)
	}
	if ws.Name != "ws2" {
		t.Errorf("expected workspace 'ws2', got '%s'", ws.Name)
	}
}

// TestMatchWorkspace tests workspace matching function.
func TestMatchWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		wsRoot    string
		path      string
		wantMatch bool
	}{
		{
			name:      "exact match",
			wsRoot:    "/tmp/test/workspace",
			path:      "/tmp/test/workspace",
			wantMatch: true,
		},
		{
			name:      "subdirectory match",
			wsRoot:    "/tmp/test/workspace",
			path:      "/tmp/test/workspace/src",
			wantMatch: true,
		},
		{
			name:      "nested subdirectory match",
			wsRoot:    "/tmp/test/workspace",
			path:      "/tmp/test/workspace/src/pkg/module",
			wantMatch: true,
		},
		{
			name:      "no match - different path",
			wsRoot:    "/tmp/test/workspace",
			path:      "/tmp/test/other",
			wantMatch: false,
		},
		{
			name:      "no match - parent directory",
			wsRoot:    "/tmp/test/workspace",
			path:      "/home/user",
			wantMatch: false,
		},
		{
			name:      "no match - similar prefix",
			wsRoot:    "/tmp/test/workspace",
			path:      "/tmp/test/workspace-other",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &Workspace{Root: tt.wsRoot}
			got := matchWorkspace(tt.path, ws)
			if got != tt.wantMatch {
				t.Errorf("matchWorkspace(%q, %q) = %v, want %v",
					tt.path, tt.wsRoot, got, tt.wantMatch)
			}
		})
	}
}

// TestGetWorkspace_Found tests retrieving an existing workspace.
func TestGetWorkspace_Found(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test-ws", Root: tmpDir, Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetector(configPath)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	ws, err := detector.GetWorkspace("test-ws")
	if err != nil {
		t.Fatalf("GetWorkspace failed: %v", err)
	}

	if ws.Name != "test-ws" {
		t.Errorf("expected workspace 'test-ws', got '%s'", ws.Name)
	}
}

// TestGetWorkspace_NotFound tests retrieving a non-existent workspace.
func TestGetWorkspace_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "test-ws", Root: tmpDir, Enabled: true}},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetector(configPath)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	_, err = detector.GetWorkspace("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent workspace, got nil")
	}
}

// TestGetWorkspace_Disabled tests retrieving a disabled workspace.
func TestGetWorkspace_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "enabled-ws", Root: tmpDir, Enabled: true},
			{Name: "disabled-ws", Root: "/tmp/disabled", Enabled: false},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetector(configPath)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	_, err = detector.GetWorkspace("disabled-ws")
	if err == nil {
		t.Fatal("expected error for disabled workspace, got nil")
	}

	if !errors.Is(err, ErrWorkspaceNotEnabled) {
		t.Errorf("expected ErrWorkspaceNotEnabled, got %v", err)
	}
}

// TestListWorkspaces tests listing all workspaces.
func TestListWorkspaces(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
			{Name: "ws2", Root: "/tmp/ws2", Enabled: false},
			{Name: "ws3", Root: "/tmp/ws3", Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetector(configPath)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	workspaces := detector.ListWorkspaces()

	if len(workspaces) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(workspaces))
	}

	// Verify all workspaces are present (including disabled)
	names := make(map[string]bool)
	for _, ws := range workspaces {
		names[ws.Name] = true
	}

	if !names["ws1"] || !names["ws2"] || !names["ws3"] {
		t.Error("not all workspaces returned from ListWorkspaces")
	}
}

// TestGetConfig tests retrieving the config.
func TestGetConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:          1,
		DefaultWorkspace: "test-ws",
		Workspaces: []Workspace{
			{Name: "test-ws", Root: tmpDir, Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetector(configPath)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	retrieved := detector.GetConfig()
	if retrieved.Version != 1 {
		t.Errorf("expected version 1, got %d", retrieved.Version)
	}
	if retrieved.DefaultWorkspace != "test-ws" {
		t.Errorf("expected default 'test-ws', got '%s'", retrieved.DefaultWorkspace)
	}
	if len(retrieved.Workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(retrieved.Workspaces))
	}
}

// TestNewDetector_ConfigNotFound tests creating detector with missing config.
func TestNewDetector_ConfigNotFound(t *testing.T) {
	_, err := NewDetector("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent config, got nil")
	}
}

// TestDetect_PriorityOrder tests that detection follows correct priority order.
func TestDetect_PriorityOrder(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	flagWsRoot := filepath.Join(tmpDir, "flag-ws")
	envWsRoot := filepath.Join(tmpDir, "env-ws")
	autoWsRoot := filepath.Join(tmpDir, "auto-ws")
	defaultWsRoot := filepath.Join(tmpDir, "default-ws")

	for _, root := range []string{flagWsRoot, envWsRoot, autoWsRoot, defaultWsRoot} {
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatalf("failed to create workspace root: %v", err)
		}
	}

	config := &Config{
		Version:          1,
		DefaultWorkspace: "default-ws",
		Workspaces: []Workspace{
			{Name: "flag-ws", Root: flagWsRoot, Enabled: true},
			{Name: "env-ws", Root: envWsRoot, Enabled: true},
			{Name: "auto-ws", Root: autoWsRoot, Enabled: true},
			{Name: "default-ws", Root: defaultWsRoot, Enabled: true},
		},
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Set env var and run from auto-detect path with explicit flag
	// Flag should win
	os.Setenv("TEST_WS", "env-ws")
	defer os.Unsetenv("TEST_WS")

	ws, err := detector.DetectWithEnv(autoWsRoot, "flag-ws", "TEST_WS")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if ws.Name != "flag-ws" {
		t.Errorf("expected flag-ws (priority 1), got %s", ws.Name)
	}

	// No flag, but env var set and in auto-detect path
	// Env should win over auto-detect
	ws, err = detector.DetectWithEnv(autoWsRoot, "", "TEST_WS")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if ws.Name != "env-ws" {
		t.Errorf("expected env-ws (priority 2), got %s", ws.Name)
	}

	// No flag, no env, but in auto-detect path
	os.Unsetenv("TEST_WS")
	ws, err = detector.Detect(autoWsRoot, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if ws.Name != "auto-ws" {
		t.Errorf("expected auto-ws (priority 3), got %s", ws.Name)
	}

	// No flag, no env, not in any workspace path
	// Should use default
	ws, err = detector.Detect("/tmp/random", "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if ws.Name != "default-ws" {
		t.Errorf("expected default-ws (priority 4), got %s", ws.Name)
	}
}

// TestDetector_SkipsDisabledWorkspaces tests that disabled workspaces are ignored in auto-detection.
func TestDetector_SkipsDisabledWorkspaces(t *testing.T) {
	t.Setenv("WORKSPACE", "") // Clear WORKSPACE env var for test isolation
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	disabledRoot := filepath.Join(tmpDir, "disabled-ws")
	if err := os.MkdirAll(disabledRoot, 0755); err != nil {
		t.Fatalf("failed to create disabled workspace root: %v", err)
	}

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "disabled-ws", Root: disabledRoot, Enabled: false},
			{Name: "enabled-ws", Root: "/tmp/enabled", Enabled: true},
		},
		DefaultWorkspace: "enabled-ws",
	}
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	detector, err := NewDetectorWithInteractive(configPath, false)
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}

	// Should not detect disabled workspace even though we're in its path
	ws, err := detector.Detect(disabledRoot, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should fall back to default since disabled workspace is skipped
	if ws.Name != "enabled-ws" {
		t.Errorf("expected enabled-ws (default fallback), got %s", ws.Name)
	}
}
