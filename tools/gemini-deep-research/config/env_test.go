package config

import (
	"os"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

func TestLoad(t *testing.T) {
	// Save original environment variables and restore after test
	origAPIKey := os.Getenv("GEMINI_API_KEY")
	origProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	origOutputDir := os.Getenv("GEMINI_DR_OUTPUT_DIR")
	origTimeout := os.Getenv("GEMINI_DR_TIMEOUT")
	defer func() {
		os.Setenv("GEMINI_API_KEY", origAPIKey)
		os.Setenv("GOOGLE_CLOUD_PROJECT", origProject)
		os.Setenv("GEMINI_DR_OUTPUT_DIR", origOutputDir)
		os.Setenv("GEMINI_DR_TIMEOUT", origTimeout)
	}()

	tests := []struct {
		name     string
		envVars  map[string]string
		flags    *types.Flags
		checkCfg func(t *testing.T, cfg *Config)
	}{
		{
			name: "Load from environment variables",
			envVars: map[string]string{
				"GEMINI_API_KEY":       "test-api-key",
				"GOOGLE_CLOUD_PROJECT": "test-project",
				"GEMINI_DR_OUTPUT_DIR": "/tmp/output",
				"GEMINI_DR_TIMEOUT":    "120",
			},
			flags: &types.Flags{},
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.APIKey != "test-api-key" {
					t.Errorf("Expected APIKey test-api-key, got %s", cfg.APIKey)
				}
				if cfg.ProjectID != "test-project" {
					t.Errorf("Expected ProjectID test-project, got %s", cfg.ProjectID)
				}
				if cfg.OutputDir != "/tmp/output" {
					t.Errorf("Expected OutputDir /tmp/output, got %s", cfg.OutputDir)
				}
				if cfg.Timeout != 120 {
					t.Errorf("Expected Timeout 120, got %d", cfg.Timeout)
				}
			},
		},
		{
			name: "Flags override environment variables",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "env-project",
				"GEMINI_DR_OUTPUT_DIR": "/env/output",
				"GEMINI_DR_TIMEOUT":    "30",
			},
			flags: &types.Flags{
				Project:   "flag-project",
				OutputDir: "/flag/output",
				Timeout:   90,
			},
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.ProjectID != "flag-project" {
					t.Errorf("Expected ProjectID flag-project (flag override), got %s", cfg.ProjectID)
				}
				if cfg.OutputDir != "/flag/output" {
					t.Errorf("Expected OutputDir /flag/output (flag override), got %s", cfg.OutputDir)
				}
				if cfg.Timeout != 90 {
					t.Errorf("Expected Timeout 90 (flag override), got %d", cfg.Timeout)
				}
			},
		},
		{
			name:    "Use defaults when no env vars or flags",
			envVars: map[string]string{},
			flags:   &types.Flags{},
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.OutputDir != DefaultOutputDir {
					t.Errorf("Expected OutputDir %s (default), got %s", DefaultOutputDir, cfg.OutputDir)
				}
				if cfg.Timeout != DefaultTimeout {
					t.Errorf("Expected Timeout %d (default), got %d", DefaultTimeout, cfg.Timeout)
				}
				if cfg.PollInterval != DefaultPollInterval {
					t.Errorf("Expected PollInterval %d (default), got %d", DefaultPollInterval, cfg.PollInterval)
				}
			},
		},
		{
			name: "Invalid timeout in env var - use default",
			envVars: map[string]string{
				"GEMINI_DR_TIMEOUT": "invalid",
			},
			flags: &types.Flags{},
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.Timeout != DefaultTimeout {
					t.Errorf("Expected Timeout %d (default due to invalid env), got %d", DefaultTimeout, cfg.Timeout)
				}
			},
		},
		{
			name: "Negative timeout in env var - use default",
			envVars: map[string]string{
				"GEMINI_DR_TIMEOUT": "-10",
			},
			flags: &types.Flags{},
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.Timeout != DefaultTimeout {
					t.Errorf("Expected Timeout %d (default due to negative env), got %d", DefaultTimeout, cfg.Timeout)
				}
			},
		},
		{
			name:    "Empty project ID from both sources",
			envVars: map[string]string{},
			flags:   &types.Flags{},
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.ProjectID != "" {
					t.Errorf("Expected ProjectID to be empty, got %s", cfg.ProjectID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars
			os.Unsetenv("GEMINI_API_KEY")
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("GEMINI_DR_OUTPUT_DIR")
			os.Unsetenv("GEMINI_DR_TIMEOUT")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Load config
			cfg, err := Load(tt.flags)
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			// Check config
			if tt.checkCfg != nil {
				tt.checkCfg(t, cfg)
			}
		})
	}
}

func TestLoadPrecedence(t *testing.T) {
	// Clear environment
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GEMINI_DR_OUTPUT_DIR")
	os.Unsetenv("GEMINI_DR_TIMEOUT")

	// Set environment variables
	os.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")
	os.Setenv("GEMINI_DR_OUTPUT_DIR", "/env/output")
	os.Setenv("GEMINI_DR_TIMEOUT", "30")
	defer func() {
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("GEMINI_DR_OUTPUT_DIR")
		os.Unsetenv("GEMINI_DR_TIMEOUT")
	}()

	// Flags should take precedence
	flags := &types.Flags{
		Project:   "flag-project",
		OutputDir: "/flag/output",
		Timeout:   90,
	}

	cfg, err := Load(flags)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify precedence: flags > env vars > defaults
	if cfg.ProjectID != "flag-project" {
		t.Errorf("Expected ProjectID from flag, got %s", cfg.ProjectID)
	}
	if cfg.OutputDir != "/flag/output" {
		t.Errorf("Expected OutputDir from flag, got %s", cfg.OutputDir)
	}
	if cfg.Timeout != 90 {
		t.Errorf("Expected Timeout from flag, got %d", cfg.Timeout)
	}
}

func TestLoadEmptyFlags(t *testing.T) {
	// Clear environment
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GEMINI_DR_OUTPUT_DIR")
	os.Unsetenv("GEMINI_DR_TIMEOUT")

	// Set environment variables
	os.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")
	defer os.Unsetenv("GOOGLE_CLOUD_PROJECT")

	// Empty flags should use env vars
	flags := &types.Flags{}

	cfg, err := Load(flags)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ProjectID != "env-project" {
		t.Errorf("Expected ProjectID from env var, got %s", cfg.ProjectID)
	}
}
