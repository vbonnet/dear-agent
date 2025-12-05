package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents csm configuration
type Config struct {
	SessionsDir string `yaml:"sessions_dir"`
	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"log_file"`
}

// Default returns default configuration
func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		SessionsDir: filepath.Join(homeDir, "sessions"),
		LogLevel:    "info",
		LogFile:     "",
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
	if dir := os.Getenv("CSM_SESSIONS_DIR"); dir != "" {
		cfg.SessionsDir = dir
	}
	if level := os.Getenv("CSM_LOG_LEVEL"); level != "" {
		cfg.LogLevel = level
	}
	if file := os.Getenv("CSM_LOG_FILE"); file != "" {
		cfg.LogFile = file
	}

	// Expand home directory in paths
	if cfg.SessionsDir != "" {
		cfg.SessionsDir = expandHome(cfg.SessionsDir)
	}
	if cfg.LogFile != "" {
		cfg.LogFile = expandHome(cfg.LogFile)
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
