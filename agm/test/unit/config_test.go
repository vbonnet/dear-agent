package unit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/config"
)

// TestConfig_ValidParsing tests loading a valid config file
func TestConfig_ValidParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write valid config
	configContent := `sessions_dir: /tmp/test-sessions
log_level: debug
log_file: /tmp/test.log
timeout:
  tmux_commands: 10s
  enabled: true
lock:
  enabled: true
  path: /tmp/test.lock
health_check:
  enabled: true
  cache_duration: 5s
  probe_timeout: 2s
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := config.Load(configFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify all fields parsed correctly
	assert.Equal(t, "/tmp/test-sessions", cfg.SessionsDir)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "/tmp/test.log", cfg.LogFile)
	assert.Equal(t, 10*time.Second, cfg.Timeout.TmuxCommands)
	assert.True(t, cfg.Timeout.Enabled)
	assert.True(t, cfg.Lock.Enabled)
	assert.Equal(t, "/tmp/test.lock", cfg.Lock.Path)
	assert.True(t, cfg.HealthCheck.Enabled)
	assert.Equal(t, 5*time.Second, cfg.HealthCheck.CacheDuration)
	assert.Equal(t, 2*time.Second, cfg.HealthCheck.ProbeTimeout)
}

// TestConfig_MissingRequiredFields tests that missing fields get default values
func TestConfig_MissingRequiredFields(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write minimal config (no required fields specified)
	// All fields should get default values
	configContent := `# Empty config - test defaults
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := config.Load(configFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify default values are applied
	assert.NotEmpty(t, cfg.SessionsDir, "SessionsDir should have default value")
	assert.Equal(t, "info", cfg.LogLevel, "LogLevel should default to 'info'")
	assert.Equal(t, 5*time.Second, cfg.Timeout.TmuxCommands, "Timeout should default to 5s")
	assert.True(t, cfg.Timeout.Enabled, "Timeout should be enabled by default")
	assert.True(t, cfg.Lock.Enabled, "Lock should be enabled by default")
	assert.NotEmpty(t, cfg.Lock.Path, "Lock path should have default value")
	assert.True(t, cfg.HealthCheck.Enabled, "HealthCheck should be enabled by default")
	assert.Equal(t, 5*time.Second, cfg.HealthCheck.CacheDuration)
	assert.Equal(t, 2*time.Second, cfg.HealthCheck.ProbeTimeout)
}

// TestConfig_InvalidFieldValues tests handling of invalid YAML values
func TestConfig_InvalidFieldValues(t *testing.T) {
	t.Run("invalid timeout duration", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		// Invalid duration format
		configContent := `timeout:
  tmux_commands: invalid-duration
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Load should fail with parse error
		_, err = config.Load(configFile)
		assert.Error(t, err, "Should fail to parse invalid duration")
		assert.Contains(t, err.Error(), "failed to parse config file")
	})

	t.Run("invalid YAML syntax", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		// Malformed YAML
		configContent := `sessions_dir: /tmp/sessions
log_level: debug
  invalid_indentation: value
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Load should fail with parse error
		_, err = config.Load(configFile)
		assert.Error(t, err, "Should fail to parse malformed YAML")
		assert.Contains(t, err.Error(), "failed to parse config file")
	})

	t.Run("invalid boolean value", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		// Invalid boolean (YAML accepts true/false/yes/no, this should work)
		// Use wrong type instead
		configContent := `lock:
  enabled: "not-a-boolean"
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// YAML will parse this as string, which fails type conversion
		_, err = config.Load(configFile)
		assert.Error(t, err, "Should fail to parse invalid boolean")
	})
}

// TestConfig_DefaultValueHandling tests default value precedence
func TestConfig_DefaultValueHandling(t *testing.T) {
	t.Run("partial config inherits defaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		// Override only some fields
		configContent := `sessions_dir: /tmp/custom-sessions
timeout:
  tmux_commands: 15s
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		cfg, err := config.Load(configFile)
		require.NoError(t, err)

		// Custom values should be used
		assert.Equal(t, "/tmp/custom-sessions", cfg.SessionsDir)
		assert.Equal(t, 15*time.Second, cfg.Timeout.TmuxCommands)

		// Unspecified fields should have defaults
		assert.Equal(t, "info", cfg.LogLevel)
		assert.True(t, cfg.Timeout.Enabled)
		assert.True(t, cfg.Lock.Enabled)
		assert.True(t, cfg.HealthCheck.Enabled)
	})

	t.Run("environment overrides config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		// Write config file with values
		configContent := `sessions_dir: /tmp/file-sessions
log_level: debug
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Set environment variables
		t.Setenv("AGM_SESSIONS_DIR", "/tmp/env-sessions")
		t.Setenv("AGM_LOG_LEVEL", "warn")
		defer os.Unsetenv("AGM_SESSIONS_DIR")
		defer os.Unsetenv("AGM_LOG_LEVEL")

		cfg, err := config.Load(configFile)
		require.NoError(t, err)

		// Environment should override file
		assert.Equal(t, "/tmp/env-sessions", cfg.SessionsDir)
		assert.Equal(t, "warn", cfg.LogLevel)
	})

	t.Run("missing config file uses defaults", func(t *testing.T) {
		// Load non-existent file
		cfg, err := config.Load("/nonexistent/path/config.yaml")
		require.NoError(t, err)

		// Should have all default values
		assert.NotEmpty(t, cfg.SessionsDir)
		assert.Equal(t, "info", cfg.LogLevel)
		assert.Equal(t, 5*time.Second, cfg.Timeout.TmuxCommands)
		assert.True(t, cfg.Timeout.Enabled)
		assert.True(t, cfg.Lock.Enabled)
		assert.True(t, cfg.HealthCheck.Enabled)
	})
}

// TestConfig_HomeExpansion tests ~ expansion in paths
func TestConfig_HomeExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Config with tilde paths
	configContent := `sessions_dir: ~/my-sessions
log_file: ~/logs/agm.log
lock:
  path: ~/agm.lock
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := config.Load(configFile)
	require.NoError(t, err)

	homeDir, _ := os.UserHomeDir()

	// Verify ~ was expanded
	assert.Equal(t, filepath.Join(homeDir, "my-sessions"), cfg.SessionsDir)
	assert.Equal(t, filepath.Join(homeDir, "logs", "agm.log"), cfg.LogFile)
	assert.Equal(t, filepath.Join(homeDir, "agm.lock"), cfg.Lock.Path)
	assert.NotContains(t, cfg.SessionsDir, "~", "Tilde should be expanded")
	assert.NotContains(t, cfg.LogFile, "~", "Tilde should be expanded")
	assert.NotContains(t, cfg.Lock.Path, "~", "Tilde should be expanded")
}

// TestConfig_IsolatedEnvironment tests that config loading doesn't affect global state
func TestConfig_IsolatedEnvironment(t *testing.T) {
	// Save original environment
	origSessionsDir := os.Getenv("AGM_SESSIONS_DIR")
	origLogLevel := os.Getenv("AGM_LOG_LEVEL")
	defer func() {
		if origSessionsDir != "" {
			t.Setenv("AGM_SESSIONS_DIR", origSessionsDir)
		} else {
			os.Unsetenv("AGM_SESSIONS_DIR")
		}
		if origLogLevel != "" {
			t.Setenv("AGM_LOG_LEVEL", origLogLevel)
		} else {
			os.Unsetenv("AGM_LOG_LEVEL")
		}
	}()

	// Set test environment
	t.Setenv("AGM_SESSIONS_DIR", "/tmp/test1")
	t.Setenv("AGM_LOG_LEVEL", "debug")

	cfg1, err := config.Load("/nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test1", cfg1.SessionsDir)

	// Change environment
	t.Setenv("AGM_SESSIONS_DIR", "/tmp/test2")
	t.Setenv("AGM_LOG_LEVEL", "info")

	cfg2, err := config.Load("/nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test2", cfg2.SessionsDir)

	// Verify first config unchanged
	assert.Equal(t, "/tmp/test1", cfg1.SessionsDir)
	assert.NotEqual(t, cfg1.SessionsDir, cfg2.SessionsDir)
}
