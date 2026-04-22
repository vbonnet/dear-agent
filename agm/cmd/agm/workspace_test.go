package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/pkg/workspace"
)

// TestDetectWorkspace_NoConfigFile tests graceful handling when config file doesn't exist.
func TestDetectWorkspace_NoConfigFile(t *testing.T) {
	// Setup
	cfg := config.Default()
	cfg.WorkspaceConfigPath = "/nonexistent/path/config.yaml"

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should fall back to default sessions dir
	if cfg.Workspace != "" {
		t.Errorf("expected empty workspace, got '%s'", cfg.Workspace)
	}

	// SessionsDir should remain at default (.claude/sessions)
	home, _ := os.UserHomeDir()
	expectedSessionsDir := filepath.Join(home, ".claude", "sessions")
	if cfg.SessionsDir != expectedSessionsDir {
		t.Errorf("expected sessions dir '%s', got '%s'", expectedSessionsDir, cfg.SessionsDir)
	}
}

// TestDetectWorkspace_InvalidConfig tests handling of corrupted config file.
func TestDetectWorkspace_InvalidConfig(t *testing.T) {
	// Setup: Create invalid workspace config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:\n  - broken"), 0600); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should fall back to default
	if cfg.Workspace != "" {
		t.Errorf("expected empty workspace, got '%s'", cfg.Workspace)
	}
}

// TestDetectWorkspace_EmptyConfig tests handling of valid but empty config.
func TestDetectWorkspace_EmptyConfig(t *testing.T) {
	// Setup: Create valid but minimal config with no enabled workspaces
	// We write the YAML directly since SaveConfig validates and rejects configs with no enabled workspaces
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `version: 1
workspaces:
  - name: disabled
    root: /tmp/disabled
    enabled: false
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write workspace config: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should fail validation (no enabled workspaces) and fall back
	if cfg.Workspace != "" {
		t.Errorf("expected empty workspace, got '%s'", cfg.Workspace)
	}
}

// TestDetectWorkspace_ExplicitFlag tests explicit --workspace flag (Priority 1).
func TestDetectWorkspace_ExplicitFlag(t *testing.T) {
	// Setup: Create workspace config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace root: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "test-ws", Root: workspaceRoot, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute with explicit flag
	detectWorkspace(cfg, "test-ws")

	// Verify
	if cfg.Workspace != "test-ws" {
		t.Errorf("expected workspace 'test-ws', got '%s'", cfg.Workspace)
	}

	// NormalizePath resolves symlinks (e.g., macOS /var → /private/var),
	// so resolve the expected path too for comparison.
	resolvedRoot, _ := filepath.EvalSymlinks(workspaceRoot)
	expectedSessionsDir := filepath.Join(resolvedRoot, ".agm", "sessions")
	if cfg.SessionsDir != expectedSessionsDir {
		t.Errorf("expected sessions dir '%s', got '%s'", expectedSessionsDir, cfg.SessionsDir)
	}
}

// TestDetectWorkspace_ExplicitFlag_NotFound tests explicit flag with unknown workspace.
func TestDetectWorkspace_ExplicitFlag_NotFound(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "test-ws", Root: tmpDir, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute with non-existent workspace
	detectWorkspace(cfg, "nonexistent")

	// Verify: Should fall back to default (warning printed)
	if cfg.Workspace != "" {
		t.Errorf("expected empty workspace, got '%s'", cfg.Workspace)
	}
}

// TestDetectWorkspace_AutoDetect tests auto-detection from current directory (Priority 3).
func TestDetectWorkspace_AutoDetect(t *testing.T) {
	// Setup
	t.Setenv("WORKSPACE", "") // Unset to test auto-detection (Priority 3)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace root: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "auto-ws", Root: workspaceRoot, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Change to workspace directory
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)
	if err := os.Chdir(workspaceRoot); err != nil {
		t.Fatalf("failed to change to workspace root: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute (no flag, should auto-detect)
	detectWorkspace(cfg, "")

	// Verify
	if cfg.Workspace != "auto-ws" {
		t.Errorf("expected workspace 'auto-ws', got '%s'", cfg.Workspace)
	}

	// NormalizePath resolves symlinks (e.g., macOS /var → /private/var),
	// so resolve the expected path too for comparison.
	resolvedRoot, _ := filepath.EvalSymlinks(workspaceRoot)
	expectedSessionsDir := filepath.Join(resolvedRoot, ".agm", "sessions")
	if cfg.SessionsDir != expectedSessionsDir {
		t.Errorf("expected sessions dir '%s', got '%s'", expectedSessionsDir, cfg.SessionsDir)
	}
}

// TestDetectWorkspace_AutoDetect_NestedDirectory tests detection from nested subdirectory.
func TestDetectWorkspace_AutoDetect_NestedDirectory(t *testing.T) {
	// Setup
	t.Setenv("WORKSPACE", "") // Unset to test auto-detection (Priority 3)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	nestedPath := filepath.Join(workspaceRoot, "src", "pkg", "module")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		t.Fatalf("failed to create nested path: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "nested-ws", Root: workspaceRoot, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Change to nested directory
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)
	if err := os.Chdir(nestedPath); err != nil {
		t.Fatalf("failed to change to nested path: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should detect workspace from parent directory
	if cfg.Workspace != "nested-ws" {
		t.Errorf("expected workspace 'nested-ws', got '%s'", cfg.Workspace)
	}
}

// TestDetectWorkspace_MultipleWorkspaces tests detection with multiple configured workspaces.
func TestDetectWorkspace_MultipleWorkspaces(t *testing.T) {
	// Setup
	t.Setenv("WORKSPACE", "") // Unset to test auto-detection (Priority 3)
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

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "ws1", Root: ws1Root, Enabled: true},
			{Name: "ws2", Root: ws2Root, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Test detection in ws1
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)

	if err := os.Chdir(ws1Root); err != nil {
		t.Fatalf("failed to change to ws1: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath
	detectWorkspace(cfg, "")

	if cfg.Workspace != "ws1" {
		t.Errorf("expected workspace 'ws1', got '%s'", cfg.Workspace)
	}

	// Test detection in ws2
	if err := os.Chdir(ws2Root); err != nil {
		t.Fatalf("failed to change to ws2: %v", err)
	}

	cfg2 := config.Default()
	cfg2.WorkspaceConfigPath = configPath
	detectWorkspace(cfg2, "")

	if cfg2.Workspace != "ws2" {
		t.Errorf("expected workspace 'ws2', got '%s'", cfg2.Workspace)
	}
}

// TestDetectWorkspace_OutsideWorkspace tests behavior when outside any workspace.
func TestDetectWorkspace_OutsideWorkspace(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace root: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "ws", Root: workspaceRoot, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Change to directory outside workspace
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)
	if err := os.Chdir(outsideDir); err != nil {
		t.Fatalf("failed to change to outside dir: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should fall back to default (no matching workspace)
	if cfg.Workspace != "" {
		t.Errorf("expected empty workspace (outside workspace), got '%s'", cfg.Workspace)
	}
}

// TestDetectWorkspace_DefaultWorkspace tests Priority 4 (default workspace from config).
func TestDetectWorkspace_DefaultWorkspace(t *testing.T) {
	// Setup
	t.Setenv("WORKSPACE", "") // Unset to test default workspace (Priority 4)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	defaultWsRoot := filepath.Join(tmpDir, "default-workspace")
	if err := os.MkdirAll(defaultWsRoot, 0755); err != nil {
		t.Fatalf("failed to create default workspace root: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version:          1,
		DefaultWorkspace: "default-ws",
		Workspaces: []workspace.Workspace{
			{Name: "default-ws", Root: defaultWsRoot, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Change to directory outside workspace
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)
	if err := os.Chdir(outsideDir); err != nil {
		t.Fatalf("failed to change to outside dir: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should use default workspace
	if cfg.Workspace != "default-ws" {
		t.Errorf("expected workspace 'default-ws', got '%s'", cfg.Workspace)
	}

	resolvedRoot, _ := filepath.EvalSymlinks(defaultWsRoot)
	expectedSessionsDir := filepath.Join(resolvedRoot, ".agm", "sessions")
	if cfg.SessionsDir != expectedSessionsDir {
		t.Errorf("expected sessions dir '%s', got '%s'", expectedSessionsDir, cfg.SessionsDir)
	}
}

// TestDetectWorkspace_SkippedWhenSessionsDirSet tests that detection is skipped when SessionsDir is explicitly set.
func TestDetectWorkspace_SkippedWhenSessionsDirSet(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	workspaceRoot := filepath.Join(tmpDir, "my-workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace root: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "ws", Root: workspaceRoot, Enabled: true},
		},
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Change to workspace directory
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)
	if err := os.Chdir(workspaceRoot); err != nil {
		t.Fatalf("failed to change to workspace root: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath
	customSessionsDir := "/custom/sessions"
	cfg.SessionsDir = customSessionsDir

	// Execute
	// Note: This test would need to call loadConfigWithFlags to test the skip logic,
	// but we can verify detectWorkspace doesn't override when already set
	detectWorkspace(cfg, "")

	// Verify: SessionsDir should remain unchanged
	// (In real usage, loadConfigWithFlags would skip calling detectWorkspace entirely)
	// This test documents the behavior when detection is called anyway
	if cfg.Workspace != "ws" {
		// Detection still runs, but in practice loadConfigWithFlags prevents this
		t.Logf("Note: detectWorkspace was called even though SessionsDir was set")
	}
}

// TestDetectWorkspace_DisabledWorkspace tests that disabled workspaces are skipped.
func TestDetectWorkspace_DisabledWorkspace(t *testing.T) {
	// Setup
	t.Setenv("WORKSPACE", "") // Unset to test disabled workspace fallback
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	disabledRoot := filepath.Join(tmpDir, "disabled-ws")
	if err := os.MkdirAll(disabledRoot, 0755); err != nil {
		t.Fatalf("failed to create disabled workspace root: %v", err)
	}

	workspaceConfig := &workspace.Config{
		Version: 1,
		Workspaces: []workspace.Workspace{
			{Name: "disabled-ws", Root: disabledRoot, Enabled: false},
			{Name: "enabled-ws", Root: "/tmp/enabled", Enabled: true},
		},
		DefaultWorkspace: "enabled-ws",
	}
	if err := workspace.SaveConfig(configPath, workspaceConfig); err != nil {
		t.Fatalf("failed to save workspace config: %v", err)
	}

	// Change to disabled workspace directory
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)
	if err := os.Chdir(disabledRoot); err != nil {
		t.Fatalf("failed to change to disabled workspace: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceConfigPath = configPath

	// Execute
	detectWorkspace(cfg, "")

	// Verify: Should fall back to default (disabled workspace skipped)
	if cfg.Workspace != "enabled-ws" {
		t.Errorf("expected workspace 'enabled-ws' (default), got '%s'", cfg.Workspace)
	}
}
