package config

import (
	"io"
	"os"
)

// Config represents application configuration
type Config struct {
	// API Configuration
	APIKey    string // Gemini API key
	ProjectID string // GCP project ID

	// Performance Configuration
	Timeout      int // Deep Research timeout in minutes
	PollInterval int // Status polling interval in seconds

	// Output Configuration
	OutputDir string // Output directory for results
	CacheDir  string // Cache directory for research results

	// I/O streams (for testing)
	Stdout io.Writer
	Stderr io.Writer
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// API key validation will be done when actually needed (in gdr-06)
	// For now, we just ensure basic fields are set

	// Ensure output directory is set
	if c.OutputDir == "" {
		c.OutputDir = DefaultOutputDir
	}

	// Ensure cache directory is set
	if c.CacheDir == "" {
		c.CacheDir = DefaultCacheDir
	}

	// Ensure timeout is set
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}

	// Ensure poll interval is set
	if c.PollInterval == 0 {
		c.PollInterval = DefaultPollInterval
	}

	// Ensure I/O streams are set
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}

	return nil
}
