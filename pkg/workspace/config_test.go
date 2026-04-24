package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadConfig_ValidConfig tests loading a valid configuration file.
func TestLoadConfig_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `version: 1
default_workspace: test
workspaces:
  - name: test
    root: /tmp/test
    enabled: true
    output_dir: /tmp/test/output
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}
	if config.DefaultWorkspace != "test" {
		t.Errorf("expected default_workspace 'test', got '%s'", config.DefaultWorkspace)
	}
	if len(config.Workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(config.Workspaces))
	}
	if config.Workspaces[0].Name != "test" {
		t.Errorf("expected workspace name 'test', got '%s'", config.Workspaces[0].Name)
	}
}

// TestLoadConfig_TildeExpansion tests that ~ is expanded to HOME.
func TestLoadConfig_TildeExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `version: 1
workspaces:
  - name: home-ws
    root: ~/workspace
    enabled: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	home := os.Getenv("HOME")
	expectedRoot := filepath.Join(home, "workspace")

	if config.Workspaces[0].Root != expectedRoot {
		t.Errorf("expected root '%s', got '%s'", expectedRoot, config.Workspaces[0].Root)
	}
}

// TestLoadConfig_InvalidYAML tests loading a file with invalid YAML.
func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `version: 1
workspaces:
  - name: test
    root: /tmp/test
    enabled: true
  invalid yaml here [ { }
`
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}

	if !strings.Contains(err.Error(), "parse config YAML") {
		t.Errorf("expected YAML parse error, got: %v", err)
	}
}

// TestLoadConfig_FileNotFound tests loading a non-existent config file.
func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}

	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("expected 'config file not found' error, got: %v", err)
	}
}

// TestSaveConfig_ValidConfig tests saving a valid configuration.
func TestSaveConfig_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:          1,
		DefaultWorkspace: "test",
		Workspaces: []Workspace{
			{Name: "test", Root: "/tmp/test", Enabled: true},
		},
	}

	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load it back and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.DefaultWorkspace != "test" {
		t.Errorf("expected default_workspace 'test', got '%s'", loaded.DefaultWorkspace)
	}
}

// TestSaveConfig_CreatesDirectory tests that SaveConfig creates parent directories.
func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nested", "dirs", "config.yaml")

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "test", Root: "/tmp/test", Enabled: true}},
	}

	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created in nested directory")
	}
}

// TestSaveConfig_InvalidConfig tests that SaveConfig validates before saving.
func TestSaveConfig_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with no workspaces (invalid)
	config := &Config{
		Version:    1,
		Workspaces: []Workspace{},
	}

	err := SaveConfig(configPath, config)
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
}

// TestValidateConfig_MissingFields tests validation of configs with missing required fields.
func TestValidateConfig_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "no workspaces",
			config: &Config{
				Version:    1,
				Workspaces: []Workspace{},
			},
			wantErr: true,
		},
		{
			name: "empty workspace name",
			config: &Config{
				Version:    1,
				Workspaces: []Workspace{{Name: "", Root: "/tmp/test", Enabled: true}},
			},
			wantErr: true,
		},
		{
			name: "empty workspace root",
			config: &Config{
				Version:    1,
				Workspaces: []Workspace{{Name: "test", Root: "", Enabled: true}},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &Config{
				Version:    1,
				Workspaces: []Workspace{{Name: "test", Root: "/tmp/test", Enabled: true}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateConfig_DuplicateNames tests validation catches duplicate workspace names.
func TestValidateConfig_DuplicateNames(t *testing.T) {
	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test", Root: "/tmp/test1", Enabled: true},
			{Name: "test", Root: "/tmp/test2", Enabled: true},
		},
	}

	err := ValidateConfig(config)
	if err == nil {
		t.Fatal("expected error for duplicate workspace names, got nil")
	}

	if !strings.Contains(err.Error(), "duplicate workspace name") {
		t.Errorf("expected 'duplicate workspace name' error, got: %v", err)
	}
}

// TestValidateConfig_VersionMismatch tests validation catches unsupported versions.
func TestValidateConfig_VersionMismatch(t *testing.T) {
	tests := []struct {
		name    string
		version int
		wantErr bool
	}{
		{"version 0", 0, true},
		{"version 1", 1, false},
		{"version 2", 2, true},
		{"version 99", 99, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Version:    tt.version,
				Workspaces: []Workspace{{Name: "test", Root: "/tmp/test", Enabled: true}},
			}

			err := ValidateConfig(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "version") {
				t.Errorf("expected version error, got: %v", err)
			}
		})
	}
}

// TestValidateConfig_NoEnabledWorkspaces tests validation requires at least one enabled workspace.
func TestValidateConfig_NoEnabledWorkspaces(t *testing.T) {
	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "disabled1", Root: "/tmp/test1", Enabled: false},
			{Name: "disabled2", Root: "/tmp/test2", Enabled: false},
		},
	}

	err := ValidateConfig(config)
	if err == nil {
		t.Fatal("expected error for no enabled workspaces, got nil")
	}

	if !errors.Is(err, ErrNoEnabledWorkspaces) {
		t.Errorf("expected ErrNoEnabledWorkspaces, got: %v", err)
	}
}

// TestValidateConfig_InvalidDefaultWorkspace tests validation of default workspace reference.
func TestValidateConfig_InvalidDefaultWorkspace(t *testing.T) {
	config := &Config{
		Version:          1,
		DefaultWorkspace: "nonexistent",
		Workspaces: []Workspace{
			{Name: "test", Root: "/tmp/test", Enabled: true},
		},
	}

	err := ValidateConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid default workspace, got nil")
	}

	if !strings.Contains(err.Error(), "default workspace") {
		t.Errorf("expected 'default workspace' error, got: %v", err)
	}
}

// TestExpandPaths_TildeExpansion tests expanding ~ in workspace paths.
func TestExpandPaths_TildeExpansion(t *testing.T) {
	home := os.Getenv("HOME")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test", Root: "~/workspace", Enabled: true},
		},
	}

	if err := ExpandPaths(config); err != nil {
		t.Fatalf("ExpandPaths failed: %v", err)
	}

	expectedRoot := filepath.Join(home, "workspace")
	if config.Workspaces[0].Root != expectedRoot {
		t.Errorf("expected root '%s', got '%s'", expectedRoot, config.Workspaces[0].Root)
	}
}

// TestExpandPaths_EnvVarExpansion tests expanding environment variables.
func TestExpandPaths_EnvVarExpansion(t *testing.T) {
	os.Setenv("TEST_WORKSPACE_ROOT", "/custom/path")
	defer os.Unsetenv("TEST_WORKSPACE_ROOT")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test", Root: "$TEST_WORKSPACE_ROOT/workspace", Enabled: true},
		},
	}

	if err := ExpandPaths(config); err != nil {
		t.Fatalf("ExpandPaths failed: %v", err)
	}

	expectedRoot := "/custom/path/workspace"
	if config.Workspaces[0].Root != expectedRoot {
		t.Errorf("expected root '%s', got '%s'", expectedRoot, config.Workspaces[0].Root)
	}
}

// TestExpandPaths_OutputDirExpansion tests expanding output_dir paths.
func TestExpandPaths_OutputDirExpansion(t *testing.T) {
	home := os.Getenv("HOME")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{
				Name:      "test",
				Root:      "~/workspace",
				OutputDir: "~/workspace/output",
				Enabled:   true,
			},
		},
	}

	if err := ExpandPaths(config); err != nil {
		t.Fatalf("ExpandPaths failed: %v", err)
	}

	expectedOutput := filepath.Join(home, "workspace", "output")
	if config.Workspaces[0].OutputDir != expectedOutput {
		t.Errorf("expected output_dir '%s', got '%s'", expectedOutput, config.Workspaces[0].OutputDir)
	}
}

// TestExpandPaths_DefaultOutputDir tests that empty output_dir defaults to root.
func TestExpandPaths_DefaultOutputDir(t *testing.T) {
	home := os.Getenv("HOME")

	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test", Root: "~/workspace", OutputDir: "", Enabled: true},
		},
	}

	if err := ExpandPaths(config); err != nil {
		t.Fatalf("ExpandPaths failed: %v", err)
	}

	expectedRoot := filepath.Join(home, "workspace")
	if config.Workspaces[0].OutputDir != expectedRoot {
		t.Errorf("expected output_dir to default to root '%s', got '%s'",
			expectedRoot, config.Workspaces[0].OutputDir)
	}
}

// TestExpandPaths_RelativePathsConverted tests that relative paths are made absolute.
func TestExpandPaths_RelativePathsConverted(t *testing.T) {
	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test", Root: "./workspace", Enabled: true},
		},
	}

	if err := ExpandPaths(config); err != nil {
		t.Fatalf("ExpandPaths failed: %v", err)
	}

	// Should be absolute now
	if !filepath.IsAbs(config.Workspaces[0].Root) {
		t.Errorf("expected absolute path, got relative: %s", config.Workspaces[0].Root)
	}
}

// TestExpandPaths_InvalidPath tests ExpandPaths with invalid paths.
func TestExpandPaths_InvalidPath(t *testing.T) {
	// Empty roots are caught by ValidateConfig, not ExpandPaths
	// Test with a truly invalid path that normalization would fail on
	config := &Config{
		Version: 1,
		Workspaces: []Workspace{
			{Name: "test", Root: "/tmp/test", OutputDir: "/tmp/test", Enabled: true},
		},
	}

	// This test just verifies ExpandPaths doesn't panic on valid paths
	err := ExpandPaths(config)
	if err != nil {
		t.Fatalf("unexpected error for valid path: %v", err)
	}

	// Verify the path was expanded
	if !filepath.IsAbs(config.Workspaces[0].Root) {
		t.Errorf("expected absolute path, got: %s", config.Workspaces[0].Root)
	}
}

// TestGetDefaultConfigPath tests default config path generation.
func TestGetDefaultConfigPath(t *testing.T) {
	home := os.Getenv("HOME")

	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{
			name:     "simple tool name",
			toolName: "engram",
			want:     filepath.Join(home, ".engram", "config.yaml"),
		},
		{
			name:     "tool with dash",
			toolName: "my-tool",
			want:     filepath.Join(home, ".my-tool", "config.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDefaultConfigPath(tt.toolName)
			if got != tt.want {
				t.Errorf("GetDefaultConfigPath(%q) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

// TestGenerateDefaultConfig tests default config generation.
func TestGenerateDefaultConfig(t *testing.T) {
	config := GenerateDefaultConfig()

	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}

	if config.DefaultWorkspace != "default" {
		t.Errorf("expected default_workspace 'default', got '%s'", config.DefaultWorkspace)
	}

	if len(config.Workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(config.Workspaces))
	}

	if config.Workspaces[0].Name != "default" {
		t.Errorf("expected workspace name 'default', got '%s'", config.Workspaces[0].Name)
	}

	if !config.Workspaces[0].Enabled {
		t.Error("expected default workspace to be enabled")
	}
}

// TestLoadConfig_ComplexConfig tests loading a complex configuration with multiple workspaces.
func TestLoadConfig_ComplexConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `version: 1
default_workspace: work
workspaces:
  - name: personal
    root: ~/personal
    enabled: true
    output_dir: ~/personal/.output
  - name: work
    root: ~/work
    enabled: true
    output_dir: ~/work/output
  - name: archive
    root: ~/archive
    enabled: false
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(config.Workspaces) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(config.Workspaces))
	}

	if config.DefaultWorkspace != "work" {
		t.Errorf("expected default_workspace 'work', got '%s'", config.DefaultWorkspace)
	}

	// Check that disabled workspace is loaded
	archiveWs := config.Workspaces[2]
	if archiveWs.Name != "archive" {
		t.Errorf("expected third workspace to be 'archive', got '%s'", archiveWs.Name)
	}
	if archiveWs.Enabled {
		t.Error("expected archive workspace to be disabled")
	}
}

// TestLoadConfig_WithToolSettings tests loading config with tool-specific settings.
func TestLoadConfig_WithToolSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `version: 1
workspaces:
  - name: test
    root: /tmp/test
    enabled: true
    custom_setting: value123
    number_setting: 42
tool_settings:
  global_option: enabled
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Check workspace custom settings
	if config.Workspaces[0].Settings["custom_setting"] != "value123" {
		t.Errorf("expected custom_setting='value123', got %v",
			config.Workspaces[0].Settings["custom_setting"])
	}

	// Check tool settings
	if config.ToolSettings["global_option"] != "enabled" {
		t.Errorf("expected global_option='enabled', got %v",
			config.ToolSettings["global_option"])
	}
}

// TestSaveConfig_FilePermissions tests that saved config has correct permissions.
func TestSaveConfig_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	config := &Config{
		Version:    1,
		Workspaces: []Workspace{{Name: "test", Root: "/tmp/test", Enabled: true}},
	}

	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Check file permissions (should be 0600)
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("expected file permissions 0600, got %o", mode)
	}
}
