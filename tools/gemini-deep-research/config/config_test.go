package config

import (
	"bytes"
	"os"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		wantValid bool
		checkCfg  func(t *testing.T, cfg *Config)
	}{
		{
			name: "Valid config - all fields set",
			cfg: &Config{
				APIKey:       "test-api-key",
				ProjectID:    "test-project",
				Timeout:      60,
				PollInterval: 10,
				OutputDir:    "./output",
			},
			wantValid: true,
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.OutputDir != "./output" {
					t.Errorf("Expected OutputDir to remain ./output, got %s", cfg.OutputDir)
				}
			},
		},
		{
			name:      "Valid config - minimal fields",
			cfg:       &Config{},
			wantValid: true,
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.OutputDir != DefaultOutputDir {
					t.Errorf("Expected OutputDir to default to %s, got %s", DefaultOutputDir, cfg.OutputDir)
				}
				if cfg.Timeout != DefaultTimeout {
					t.Errorf("Expected Timeout to default to %d, got %d", DefaultTimeout, cfg.Timeout)
				}
				if cfg.PollInterval != DefaultPollInterval {
					t.Errorf("Expected PollInterval to default to %d, got %d", DefaultPollInterval, cfg.PollInterval)
				}
			},
		},
		{
			name: "Config with I/O streams set",
			cfg: &Config{
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			},
			wantValid: true,
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.Stdout == nil {
					t.Error("Expected Stdout to be set")
				}
				if cfg.Stderr == nil {
					t.Error("Expected Stderr to be set")
				}
			},
		},
		{
			name:      "Config with missing I/O streams - should default",
			cfg:       &Config{},
			wantValid: true,
			checkCfg: func(t *testing.T, cfg *Config) {
				if cfg.Stdout != os.Stdout {
					t.Error("Expected Stdout to default to os.Stdout")
				}
				if cfg.Stderr != os.Stderr {
					t.Error("Expected Stderr to default to os.Stderr")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantValid && err != nil {
				t.Errorf("Expected valid config, got error: %v", err)
			}
			if !tt.wantValid && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if tt.checkCfg != nil {
				tt.checkCfg(t, tt.cfg)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check that defaults are applied
	if cfg.OutputDir != DefaultOutputDir {
		t.Errorf("Expected OutputDir default %s, got %s", DefaultOutputDir, cfg.OutputDir)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Expected Timeout default %d, got %d", DefaultTimeout, cfg.Timeout)
	}
	if cfg.PollInterval != DefaultPollInterval {
		t.Errorf("Expected PollInterval default %d, got %d", DefaultPollInterval, cfg.PollInterval)
	}
}

func TestConfigIOStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer

	cfg := &Config{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Verify streams are preserved
	if cfg.Stdout != &stdout {
		t.Error("Stdout was not preserved")
	}
	if cfg.Stderr != &stderr {
		t.Error("Stderr was not preserved")
	}
}
