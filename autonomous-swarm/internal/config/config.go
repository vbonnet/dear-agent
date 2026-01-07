package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all configuration for the autonomous swarm
type Config struct {
	// Task Queue
	QueueFilePath string

	// Telemetry
	LogFilePath     string
	RoadmapFilePath string

	// Execution
	MaxIterations     int
	SessionTimeout    time.Duration
	HeartbeatInterval time.Duration

	// Validation
	TestCoverageThreshold float64
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		QueueFilePath:         "TASK-QUEUE.yaml",
		LogFilePath:           "EXECUTION-LOG.jsonl",
		RoadmapFilePath:       "ROADMAP.md",
		MaxIterations:         3,
		SessionTimeout:        1 * time.Hour,
		HeartbeatInterval:     5 * time.Minute,
		TestCoverageThreshold: 0.80, // 80%
	}
}

// LoadFromEnv loads configuration from environment variables
// Overlays env vars on top of defaults
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("SWARM_QUEUE_FILE"); v != "" {
		cfg.QueueFilePath = v
	}
	if v := os.Getenv("SWARM_LOG_FILE"); v != "" {
		cfg.LogFilePath = v
	}
	if v := os.Getenv("SWARM_ROADMAP_FILE"); v != "" {
		cfg.RoadmapFilePath = v
	}
	if v := os.Getenv("SWARM_MAX_ITERATIONS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.MaxIterations = i
		}
	}
	if v := os.Getenv("SWARM_SESSION_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.SessionTimeout = d
		}
	}
	if v := os.Getenv("SWARM_HEARTBEAT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.HeartbeatInterval = d
		}
	}
	if v := os.Getenv("SWARM_TEST_COVERAGE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.TestCoverageThreshold = f
		}
	}

	return cfg
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check file paths are not empty
	if c.QueueFilePath == "" {
		return fmt.Errorf("queue file path cannot be empty")
	}
	if c.LogFilePath == "" {
		return fmt.Errorf("log file path cannot be empty")
	}
	if c.RoadmapFilePath == "" {
		return fmt.Errorf("roadmap file path cannot be empty")
	}

	// Check numeric constraints
	if c.MaxIterations < 1 {
		return fmt.Errorf("max iterations must be at least 1, got %d", c.MaxIterations)
	}
	if c.SessionTimeout <= 0 {
		return fmt.Errorf("session timeout must be positive, got %v", c.SessionTimeout)
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive, got %v", c.HeartbeatInterval)
	}
	if c.TestCoverageThreshold < 0 || c.TestCoverageThreshold > 1 {
		return fmt.Errorf("test coverage threshold must be between 0 and 1, got %f", c.TestCoverageThreshold)
	}

	// Check if queue file exists (if path is relative, check in current dir)
	if !filepath.IsAbs(c.QueueFilePath) {
		// Relative path - will be resolved at runtime
		return nil
	}

	// Absolute path - check existence
	if _, err := os.Stat(c.QueueFilePath); os.IsNotExist(err) {
		return fmt.Errorf("queue file does not exist: %s", c.QueueFilePath)
	}

	return nil
}
