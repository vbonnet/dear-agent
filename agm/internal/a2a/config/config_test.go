package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
		env  map[string]string // env vars to set for the test
	}{
		{
			name: "tilde only",
			path: "~",
			want: home,
		},
		{
			name: "tilde with subpath",
			path: "~/src/projects",
			want: filepath.Join(home, "/src/projects"),
		},
		{
			name: "absolute path unchanged",
			path: "/usr/local/bin",
			want: "/usr/local/bin",
		},
		{
			name: "env var expansion",
			path: "$TEST_EXPAND_DIR/sub",
			env:  map[string]string{"TEST_EXPAND_DIR": "/opt/data"},
			want: "/opt/data/sub",
		},
		{
			name: "tilde and env var combined",
			path: "~/$TEST_EXPAND_SUBDIR",
			env:  map[string]string{"TEST_EXPAND_SUBDIR": "mydir"},
			want: filepath.Join(home, "/mydir"),
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := ExpandPath(tt.path)
			if got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("defaults when config file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string {
			return filepath.Join(tmpDir, "nonexistent", "config.yaml")
		}
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		cfg, err := Load(true)
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.Version != "1.0" {
			t.Errorf("expected default version 1.0, got %q", cfg.Version)
		}
		if cfg.A2A.RetentionDays != 30 {
			t.Errorf("expected default retention days 30, got %d", cfg.A2A.RetentionDays)
		}
		if cfg.Preferences.DefaultModel != "sonnet" {
			t.Errorf("expected default model sonnet, got %q", cfg.Preferences.DefaultModel)
		}
	})

	t.Run("forceReload returns fresh config", func(t *testing.T) {
		tmpDir := t.TempDir()
		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string {
			return filepath.Join(tmpDir, "no-such-file.yaml")
		}
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		cfg1, err := Load(true)
		if err != nil {
			t.Fatalf("first Load returned error: %v", err)
		}

		// Load again without force - should return cached value
		cfg2, err := Load(false)
		if err != nil {
			t.Fatalf("second Load returned error: %v", err)
		}
		if cfg1 != cfg2 {
			t.Error("expected cached config to be same pointer")
		}

		// Force reload should still work (returns a new config from defaults)
		cfg3, err := Load(true)
		if err != nil {
			t.Fatalf("third Load returned error: %v", err)
		}
		if cfg3.Version != "1.0" {
			t.Errorf("expected version 1.0 after force reload, got %q", cfg3.Version)
		}
	})

	t.Run("loads from YAML config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		yamlContent := `version: "1.0"
workspace:
  root: "/custom/workspace"
  swarm_dir: "/custom/swarm"
a2a:
  channels_dir: "/custom/channels"
  retention_days: 90
  poll_interval: 120
preferences:
  default_model: "opus"
  auto_commit: false
  git_remote: "gitlab"
`
		if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}

		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string { return configFile }
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		cfg, err := Load(true)
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}

		if cfg.Workspace.Root != "/custom/workspace" {
			t.Errorf("expected workspace root /custom/workspace, got %q", cfg.Workspace.Root)
		}
		if cfg.Workspace.SwarmDir != "/custom/swarm" {
			t.Errorf("expected swarm dir /custom/swarm, got %q", cfg.Workspace.SwarmDir)
		}
		if cfg.A2A.ChannelsDir != "/custom/channels" {
			t.Errorf("expected channels dir /custom/channels, got %q", cfg.A2A.ChannelsDir)
		}
		if cfg.A2A.RetentionDays != 90 {
			t.Errorf("expected retention days 90, got %d", cfg.A2A.RetentionDays)
		}
		if cfg.A2A.PollInterval != 120 {
			t.Errorf("expected poll interval 120, got %d", cfg.A2A.PollInterval)
		}
		if cfg.Preferences.DefaultModel != "opus" {
			t.Errorf("expected model opus, got %q", cfg.Preferences.DefaultModel)
		}
		if cfg.Preferences.AutoCommit != false {
			t.Error("expected auto_commit false")
		}
		if cfg.Preferences.GitRemote != "gitlab" {
			t.Errorf("expected git remote gitlab, got %q", cfg.Preferences.GitRemote)
		}
	})

	t.Run("merges with defaults for unset fields", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "partial.yaml")

		yamlContent := `workspace:
  root: "/my/workspace"
`
		if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}

		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string { return configFile }
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		cfg, err := Load(true)
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}

		// Overridden field
		if cfg.Workspace.Root != "/my/workspace" {
			t.Errorf("expected workspace root /my/workspace, got %q", cfg.Workspace.Root)
		}
		// Default fields preserved
		if cfg.A2A.RetentionDays != 30 {
			t.Errorf("expected default retention days 30, got %d", cfg.A2A.RetentionDays)
		}
		if cfg.Preferences.DefaultModel != "sonnet" {
			t.Errorf("expected default model sonnet, got %q", cfg.Preferences.DefaultModel)
		}
	})
}

func TestValidate(t *testing.T) {
	t.Run("passes with defaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string {
			return filepath.Join(tmpDir, "nonexistent.yaml")
		}
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		// Force reload to use defaults
		_, err := Load(true)
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}

		if err := Validate(); err != nil {
			t.Errorf("Validate failed with defaults: %v", err)
		}
	})

	t.Run("passes with valid custom config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		yamlContent := `version: "1.0"
workspace:
  swarm_dir: "/valid/swarm"
a2a:
  channels_dir: "/valid/channels"
`
		if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}

		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string { return configFile }
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		_, err := Load(true)
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}

		if err := Validate(); err != nil {
			t.Errorf("Validate failed with valid config: %v", err)
		}
	})

	t.Run("fails with unsupported version", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		yamlContent := `version: "2.0"
`
		if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}

		origGetConfigPath := GetConfigPath
		GetConfigPath = func() string { return configFile }
		t.Cleanup(func() { GetConfigPath = origGetConfigPath })

		_, err := Load(true)
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}

		err = Validate()
		if err == nil {
			t.Error("expected Validate to fail with unsupported version, but it passed")
		}
	})
}
