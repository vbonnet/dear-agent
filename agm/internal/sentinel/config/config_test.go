package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify defaults
	assert.NotEmpty(t, cfg.Patterns.Bash)
	assert.NotEmpty(t, cfg.Patterns.Beads)
	assert.NotEmpty(t, cfg.Patterns.Git)
	assert.NotEmpty(t, cfg.Violations.Directory)
	assert.Equal(t, "60s", cfg.Monitoring.Interval)
	assert.Equal(t, "10m", cfg.Monitoring.StuckThreshold)
	assert.True(t, cfg.Recovery.Enabled)
	assert.Equal(t, "escape", cfg.Recovery.Strategy)
	assert.Equal(t, 3, cfg.Recovery.MaxAttempts)
	assert.NotEmpty(t, cfg.Logging.IncidentsFile)
	assert.NotEmpty(t, cfg.Logging.DiagnosesDir)
	assert.False(t, cfg.Logging.Verbose)
	assert.False(t, cfg.EventBus.Enabled)
	assert.False(t, cfg.Temporal.Enabled)

	// Verify default exempt sessions
	assert.Equal(t, DefaultExemptSessions, cfg.Recovery.ExemptSessions)
	assert.Contains(t, cfg.Recovery.ExemptSessions, "human")
	assert.Contains(t, cfg.Recovery.ExemptSessions, "orchestrator")
	assert.Contains(t, cfg.Recovery.ExemptSessions, "orchestrator-v2")
	assert.Contains(t, cfg.Recovery.ExemptSessions, "meta-orchestrator")
}

func TestIsSessionExempt(t *testing.T) {
	tests := []struct {
		name       string
		session    string
		exempt     []string
		wantExempt bool
	}{
		{
			name:       "orchestrator is exempt by default",
			session:    "orchestrator",
			exempt:     DefaultExemptSessions,
			wantExempt: true,
		},
		{
			name:       "meta-orchestrator is exempt by default",
			session:    "meta-orchestrator",
			exempt:     DefaultExemptSessions,
			wantExempt: true,
		},
		{
			name:       "human is exempt by default",
			session:    "human",
			exempt:     DefaultExemptSessions,
			wantExempt: true,
		},
		{
			name:       "orchestrator-foo matches orchestrator prefix",
			session:    "orchestrator-foo",
			exempt:     DefaultExemptSessions,
			wantExempt: true,
		},
		{
			name:       "worker session is NOT exempt",
			session:    "fix-mass-detach",
			exempt:     DefaultExemptSessions,
			wantExempt: false,
		},
		{
			name:       "astrocyte-recovery is NOT exempt",
			session:    "astrocyte-recovery",
			exempt:     DefaultExemptSessions,
			wantExempt: false,
		},
		{
			name:       "empty exempt list exempts nothing",
			session:    "orchestrator",
			exempt:     []string{},
			wantExempt: false,
		},
		{
			name:       "custom exempt prefix matches",
			session:    "supervisor-main",
			exempt:     []string{"supervisor"},
			wantExempt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := RecoveryConfig{ExemptSessions: tt.exempt}
			assert.Equal(t, tt.wantExempt, cfg.IsSessionExempt(tt.session))
		})
	}
}

func TestLoadConfig_FileNotExists(t *testing.T) {
	// Loading non-existent file should return default config
	cfg, err := LoadConfig("/tmp/nonexistent-config.yaml")
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Should have default values
	assert.Equal(t, "escape", cfg.Recovery.Strategy)
}

func TestLoadConfig_ValidYAML(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
patterns:
  bash: /tmp/bash-patterns.yaml
  beads: /tmp/beads-patterns.yaml
  git: /tmp/git-patterns.yaml

violations:
  directory: /tmp/violations

monitoring:
  interval: 30s
  stuck_threshold: 5m

tmux:
  socket: /tmp/custom.sock

recovery:
  enabled: true
  strategy: ctrl_c
  max_attempts: 5

logging:
  incidents_file: /tmp/incidents.jsonl
  diagnoses_dir: /tmp/diagnoses
  verbose: true

eventbus:
  enabled: true
  broker: redis://localhost:6379

temporal:
  enabled: true
  address: localhost:7233
  namespace: test-namespace
`

	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Verify loaded values
	assert.Equal(t, "/tmp/bash-patterns.yaml", cfg.Patterns.Bash)
	assert.Equal(t, "/tmp/beads-patterns.yaml", cfg.Patterns.Beads)
	assert.Equal(t, "/tmp/git-patterns.yaml", cfg.Patterns.Git)
	assert.Equal(t, "/tmp/violations", cfg.Violations.Directory)
	assert.Equal(t, "30s", cfg.Monitoring.Interval)
	assert.Equal(t, 30*time.Second, cfg.Monitoring.IntervalDuration)
	assert.Equal(t, "5m", cfg.Monitoring.StuckThreshold)
	assert.Equal(t, 5*time.Minute, cfg.Monitoring.StuckThresholdDuration)
	assert.Equal(t, "/tmp/custom.sock", cfg.Tmux.Socket)
	assert.True(t, cfg.Recovery.Enabled)
	assert.Equal(t, "ctrl_c", cfg.Recovery.Strategy)
	assert.Equal(t, 5, cfg.Recovery.MaxAttempts)
	assert.Equal(t, "/tmp/incidents.jsonl", cfg.Logging.IncidentsFile)
	assert.Equal(t, "/tmp/diagnoses", cfg.Logging.DiagnosesDir)
	assert.True(t, cfg.Logging.Verbose)
	assert.True(t, cfg.EventBus.Enabled)
	assert.Equal(t, "redis://localhost:6379", cfg.EventBus.Broker)
	assert.True(t, cfg.Temporal.Enabled)
	assert.Equal(t, "localhost:7233", cfg.Temporal.Address)
	assert.Equal(t, "test-namespace", cfg.Temporal.Namespace)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
patterns:
  bash: /tmp/bash.yaml
  invalid yaml here: [
`

	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	// Loading invalid YAML should return error
	_, err = LoadConfig(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config YAML")
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name         string
		modifyConfig func(*Config)
		wantErr      string
	}{
		{
			name: "valid config",
			modifyConfig: func(cfg *Config) {
				// No modifications, use defaults
			},
			wantErr: "",
		},
		{
			name: "missing bash pattern path",
			modifyConfig: func(cfg *Config) {
				cfg.Patterns.Bash = ""
			},
			wantErr: "patterns.bash path is required",
		},
		{
			name: "missing beads pattern path",
			modifyConfig: func(cfg *Config) {
				cfg.Patterns.Beads = ""
			},
			wantErr: "patterns.beads path is required",
		},
		{
			name: "missing git pattern path",
			modifyConfig: func(cfg *Config) {
				cfg.Patterns.Git = ""
			},
			wantErr: "patterns.git path is required",
		},
		{
			name: "missing violations directory",
			modifyConfig: func(cfg *Config) {
				cfg.Violations.Directory = ""
			},
			wantErr: "violations.directory is required",
		},
		{
			name: "invalid recovery strategy",
			modifyConfig: func(cfg *Config) {
				cfg.Recovery.Strategy = "invalid_strategy"
			},
			wantErr: "invalid recovery.strategy",
		},
		{
			name: "zero interval duration",
			modifyConfig: func(cfg *Config) {
				cfg.Monitoring.IntervalDuration = 0
			},
			wantErr: "monitoring.interval must be positive",
		},
		{
			name: "zero stuck threshold",
			modifyConfig: func(cfg *Config) {
				cfg.Monitoring.StuckThresholdDuration = 0
			},
			wantErr: "monitoring.stuck_threshold must be positive",
		},
		{
			name: "missing incidents file",
			modifyConfig: func(cfg *Config) {
				cfg.Logging.IncidentsFile = ""
			},
			wantErr: "logging.incidents_file is required",
		},
		{
			name: "missing diagnoses dir",
			modifyConfig: func(cfg *Config) {
				cfg.Logging.DiagnosesDir = ""
			},
			wantErr: "logging.diagnoses_dir is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modifyConfig(cfg)

			err := cfg.Validate()

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseDurations(t *testing.T) {
	tests := []struct {
		name            string
		interval        string
		stuckThreshold  string
		wantIntervalDur time.Duration
		wantStuckDur    time.Duration
		wantErr         bool
	}{
		{
			name:            "valid durations",
			interval:        "60s",
			stuckThreshold:  "10m",
			wantIntervalDur: 60 * time.Second,
			wantStuckDur:    10 * time.Minute,
			wantErr:         false,
		},
		{
			name:            "minutes and hours",
			interval:        "2m",
			stuckThreshold:  "1h",
			wantIntervalDur: 2 * time.Minute,
			wantStuckDur:    1 * time.Hour,
			wantErr:         false,
		},
		{
			name:           "invalid interval",
			interval:       "invalid",
			stuckThreshold: "10m",
			wantErr:        true,
		},
		{
			name:           "invalid stuck threshold",
			interval:       "60s",
			stuckThreshold: "not-a-duration",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Monitoring.Interval = tt.interval
			cfg.Monitoring.StuckThreshold = tt.stuckThreshold

			err := cfg.parseDurations()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantIntervalDur, cfg.Monitoring.IntervalDuration)
				assert.Equal(t, tt.wantStuckDur, cfg.Monitoring.StuckThresholdDuration)
			}
		})
	}
}

func TestExpandPaths(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	cfg := &Config{
		Patterns: PatternConfig{
			Bash:  "~/patterns/bash.yaml",
			Beads: "~/patterns/beads.yaml",
			Git:   "~/patterns/git.yaml",
		},
		Violations: ViolationsConfig{
			Directory: "~/violations",
		},
		Logging: LoggingConfig{
			IncidentsFile: "~/.agm/incidents.jsonl",
			DiagnosesDir:  "~/.agm/diagnoses",
		},
	}

	cfg.ExpandPaths()

	// Verify ~ was expanded to home directory
	assert.Equal(t, filepath.Join(homeDir, "patterns/bash.yaml"), cfg.Patterns.Bash)
	assert.Equal(t, filepath.Join(homeDir, "patterns/beads.yaml"), cfg.Patterns.Beads)
	assert.Equal(t, filepath.Join(homeDir, "patterns/git.yaml"), cfg.Patterns.Git)
	assert.Equal(t, filepath.Join(homeDir, "violations"), cfg.Violations.Directory)
	assert.Equal(t, filepath.Join(homeDir, ".agm/incidents.jsonl"), cfg.Logging.IncidentsFile)
	assert.Equal(t, filepath.Join(homeDir, ".agm/diagnoses"), cfg.Logging.DiagnosesDir)
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	homeDir, _ := os.UserHomeDir()

	expected := filepath.Join(homeDir, ".config/astrocyte/config.yaml")
	assert.Equal(t, expected, path)
}

func TestLoadConfig_PartialOverride(t *testing.T) {
	// Config file with only some fields set should merge with defaults
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.yaml")

	partialYAML := `
monitoring:
  interval: 120s

recovery:
  strategy: ctrl_c
`

	err := os.WriteFile(configPath, []byte(partialYAML), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Overridden values
	assert.Equal(t, "120s", cfg.Monitoring.Interval)
	assert.Equal(t, 120*time.Second, cfg.Monitoring.IntervalDuration)
	assert.Equal(t, "ctrl_c", cfg.Recovery.Strategy)

	// Default values should still be present
	assert.NotEmpty(t, cfg.Patterns.Bash)
	assert.True(t, cfg.Recovery.Enabled)
	assert.Equal(t, 3, cfg.Recovery.MaxAttempts)
}

func TestLoadConfig_ExemptSessionsOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "exempt.yaml")

	exemptYAML := `
recovery:
  strategy: escape
  exempt_sessions:
    - human
    - supervisor
    - my-custom-prefix
`

	err := os.WriteFile(configPath, []byte(exemptYAML), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Overridden exempt sessions
	assert.Equal(t, []string{"human", "supervisor", "my-custom-prefix"}, cfg.Recovery.ExemptSessions)
	assert.True(t, cfg.Recovery.IsSessionExempt("human"))
	assert.True(t, cfg.Recovery.IsSessionExempt("supervisor-main"))
	assert.False(t, cfg.Recovery.IsSessionExempt("orchestrator")) // removed from override
}
