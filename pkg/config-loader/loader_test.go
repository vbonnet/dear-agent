package configloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test config types for various test cases
type SimpleConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type ConfigWithSettings struct {
	Name     string `yaml:"name"`
	Version  string `yaml:"version"`
	Settings struct {
		Enabled bool `yaml:"enabled"`
		Timeout int  `yaml:"timeout"`
	} `yaml:"settings"`
}

type ConfigWithHome struct {
	Name    string `yaml:"name"`
	LogPath string `yaml:"log_path"`
	DataDir string `yaml:"data_dir"`
}

type NestedConfig struct {
	App struct {
		Name     string `yaml:"name"`
		Database struct {
			Host    string `yaml:"host"`
			Port    int    `yaml:"port"`
			Timeout int    `yaml:"timeout"`
		} `yaml:"database"`
	} `yaml:"app"`
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		check   func(*testing.T, *SimpleConfig)
	}{
		{
			name:    "load valid YAML",
			path:    "testdata/valid.yaml",
			wantErr: false,
			check: func(t *testing.T, cfg *SimpleConfig) {
				if cfg.Name != "test-app" {
					t.Errorf("Name = %q, want %q", cfg.Name, "test-app")
				}
				if cfg.Version != "1.0" {
					t.Errorf("Version = %q, want %q", cfg.Version, "1.0")
				}
			},
		},
		{
			name:    "load non-existent file returns error",
			path:    "testdata/nonexistent.yaml",
			wantErr: true,
		},
		{
			name:    "load invalid YAML returns error",
			path:    "testdata/invalid.yaml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load[SimpleConfig](tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestLoadWithSettings(t *testing.T) {
	cfg, err := Load[ConfigWithSettings]("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Name != "test-app" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-app")
	}
	if !cfg.Settings.Enabled {
		t.Error("Settings.Enabled = false, want true")
	}
	if cfg.Settings.Timeout != 30 {
		t.Errorf("Settings.Timeout = %d, want %d", cfg.Settings.Timeout, 30)
	}
}

func TestLoadEmpty(t *testing.T) {
	cfg, err := Load[SimpleConfig]("testdata/empty.yaml")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	// Empty YAML should result in zero values
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty string", cfg.Name)
	}
	if cfg.Version != "" {
		t.Errorf("Version = %q, want empty string", cfg.Version)
	}
}

func TestLoadNested(t *testing.T) {
	cfg, err := Load[NestedConfig]("testdata/nested.yaml")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.App.Name != "test" {
		t.Errorf("App.Name = %q, want %q", cfg.App.Name, "test")
	}
	if cfg.App.Database.Host != "localhost" {
		t.Errorf("App.Database.Host = %q, want %q", cfg.App.Database.Host, "localhost")
	}
	if cfg.App.Database.Port != 5432 {
		t.Errorf("App.Database.Port = %d, want %d", cfg.App.Database.Port, 5432)
	}
}

func TestLoadWithDefaults(t *testing.T) {
	defaults := SimpleConfig{
		Name:    "default-name",
		Version: "0.0.0",
	}

	tests := []struct {
		name      string
		path      string
		wantErr   bool
		wantName  string
		wantVer   string
		checkFrom string
	}{
		{
			name:      "existing valid file loads from file",
			path:      "testdata/valid.yaml",
			wantErr:   false,
			wantName:  "test-app",
			wantVer:   "1.0",
			checkFrom: "file",
		},
		{
			name:      "non-existent file uses defaults",
			path:      "testdata/nonexistent.yaml",
			wantErr:   false,
			wantName:  "default-name",
			wantVer:   "0.0.0",
			checkFrom: "defaults",
		},
		{
			name:      "invalid YAML returns error",
			path:      "testdata/invalid.yaml",
			wantErr:   true,
			checkFrom: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadWithDefaults(tt.path, defaults)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadWithDefaults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if cfg.Name != tt.wantName {
					t.Errorf("Name = %q, want %q (from %s)", cfg.Name, tt.wantName, tt.checkFrom)
				}
				if cfg.Version != tt.wantVer {
					t.Errorf("Version = %q, want %q (from %s)", cfg.Version, tt.wantVer, tt.checkFrom)
				}
			}
		})
	}
}

func TestLoadOrDefault(t *testing.T) {
	defaults := SimpleConfig{
		Name:    "default-name",
		Version: "0.0.0",
	}

	tests := []struct {
		name      string
		path      string
		wantName  string
		wantVer   string
		checkFrom string
	}{
		{
			name:      "existing valid file loads from file",
			path:      "testdata/valid.yaml",
			wantName:  "test-app",
			wantVer:   "1.0",
			checkFrom: "file",
		},
		{
			name:      "non-existent file uses defaults",
			path:      "testdata/nonexistent.yaml",
			wantName:  "default-name",
			wantVer:   "0.0.0",
			checkFrom: "defaults",
		},
		{
			name:      "invalid YAML uses defaults (no error)",
			path:      "testdata/invalid.yaml",
			wantName:  "default-name",
			wantVer:   "0.0.0",
			checkFrom: "defaults",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// LoadOrDefault never returns error, always returns valid config
			cfg := LoadOrDefault(tt.path, defaults)
			if cfg == nil {
				t.Fatal("LoadOrDefault() returned nil")
			}
			if cfg.Name != tt.wantName {
				t.Errorf("Name = %q, want %q (from %s)", cfg.Name, tt.wantName, tt.checkFrom)
			}
			if cfg.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q (from %s)", cfg.Version, tt.wantVer, tt.checkFrom)
			}
		})
	}
}

// TestLoadWithHomePath tests loading config with home directory expansion
func TestLoadWithHomePath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	// Create a temporary config file in home directory
	tempFile := filepath.Join(homeDir, ".test-config-loader.yaml")
	defer os.Remove(tempFile)

	content := []byte("name: \"home-test\"\nversion: \"1.0\"")
	if err := os.WriteFile(tempFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Load using ~ path
	cfg, err := Load[SimpleConfig]("~/.test-config-loader.yaml")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Name != "home-test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "home-test")
	}
}

// TestLoadErrorMessages tests that error messages are informative
func TestLoadErrorMessages(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantContain string
	}{
		{
			name:        "non-existent file error contains path",
			path:        "testdata/nonexistent.yaml",
			wantContain: "testdata/nonexistent.yaml",
		},
		{
			name:        "invalid YAML error contains path",
			path:        "testdata/invalid.yaml",
			wantContain: "testdata/invalid.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load[SimpleConfig](tt.path)
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantContain) {
				t.Errorf("Error message %q does not contain %q", err.Error(), tt.wantContain)
			}
		})
	}
}
