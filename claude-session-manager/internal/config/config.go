package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents csm configuration
type Config struct {
	SessionsDir string `yaml:"sessions_dir"`
	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"log_file"`

	// Resilience features
	Timeout     TimeoutConfig     `yaml:"timeout"`
	Lock        LockConfig        `yaml:"lock"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
}

// TimeoutConfig holds timeout configuration
type TimeoutConfig struct {
	TmuxCommands time.Duration `yaml:"tmux_commands"` // Default: 5s
	Enabled      bool          `yaml:"enabled"`       // Default: true
}

// LockConfig holds lock configuration
type LockConfig struct {
	Enabled bool   `yaml:"enabled"` // Default: true
	Path    string `yaml:"path"`    // Default: /tmp/csm-{UID}/csm.lock
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Enabled       bool          `yaml:"enabled"`        // Default: true
	CacheDuration time.Duration `yaml:"cache_duration"` // Default: 5s
	ProbeTimeout  time.Duration `yaml:"probe_timeout"`  // Default: 2s
}

// Default returns default configuration
func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	uid := os.Getuid()
	return &Config{
		SessionsDir: filepath.Join(homeDir, "sessions"),
		LogLevel:    "info",
		LogFile:     "",
		Timeout: TimeoutConfig{
			TmuxCommands: 5 * time.Second,
			Enabled:      true,
		},
		Lock: LockConfig{
			Enabled: true,
			Path:    fmt.Sprintf("/tmp/csm-%d/csm.lock", uid),
		},
		HealthCheck: HealthCheckConfig{
			Enabled:       true,
			CacheDuration: 5 * time.Second,
			ProbeTimeout:  2 * time.Second,
		},
	}
}

// Load loads configuration with precedence: defaults < file < env < flags
func Load(cfgFile string) (*Config, error) {
	cfg := Default()

	// Load from config file if exists
	if cfgFile == "" {
		homeDir, _ := os.UserHomeDir()
		cfgFile = filepath.Join(homeDir, ".config", "csm", "config.yaml")
	}

	if data, err := os.ReadFile(cfgFile); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables
	if dir := os.Getenv("AGM_SESSIONS_DIR"); dir != "" {
		cfg.SessionsDir = dir
	}
	if level := os.Getenv("AGM_LOG_LEVEL"); level != "" {
		cfg.LogLevel = level
	}
	if file := os.Getenv("AGM_LOG_FILE"); file != "" {
		cfg.LogFile = file
	}

	// Expand home directory in paths
	if cfg.SessionsDir != "" {
		cfg.SessionsDir = expandHome(cfg.SessionsDir)
	}
	if cfg.LogFile != "" {
		cfg.LogFile = expandHome(cfg.LogFile)
	}
	if cfg.Lock.Path != "" {
		cfg.Lock.Path = expandHome(cfg.Lock.Path)
	}

	return cfg, nil
}

// expandHome expands ~ to home directory
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if len(path) == 1 {
		return homeDir
	}

	return filepath.Join(homeDir, path[2:])
}
