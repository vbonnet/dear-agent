package config

import (
	"os"
	"strconv"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// Load loads configuration from flags and environment variables
func Load(flags *types.Flags) (*Config, error) {
	cfg := &Config{}

	// Load API key from environment
	cfg.APIKey = os.Getenv("GEMINI_API_KEY")

	// Load project ID (flag takes precedence over env var)
	if flags.Project != "" {
		cfg.ProjectID = flags.Project
	} else {
		cfg.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	// Load output directory (flag takes precedence over env var)
	if flags.OutputDir != "" {
		cfg.OutputDir = flags.OutputDir
	} else if envOutputDir := os.Getenv("GEMINI_DR_OUTPUT_DIR"); envOutputDir != "" {
		cfg.OutputDir = envOutputDir
	} else {
		cfg.OutputDir = DefaultOutputDir
	}

	// Load cache directory from env var
	if envCacheDir := os.Getenv("GEMINI_DR_CACHE_DIR"); envCacheDir != "" {
		cfg.CacheDir = envCacheDir
	} else {
		cfg.CacheDir = DefaultCacheDir
	}

	// Load timeout (flag takes precedence over env var)
	if flags.Timeout > 0 {
		cfg.Timeout = flags.Timeout
	} else if envTimeout := os.Getenv("GEMINI_DR_TIMEOUT"); envTimeout != "" {
		if timeout, err := strconv.Atoi(envTimeout); err == nil && timeout > 0 {
			cfg.Timeout = timeout
		} else {
			cfg.Timeout = DefaultTimeout
		}
	} else {
		cfg.Timeout = DefaultTimeout
	}

	// Set poll interval to default
	cfg.PollInterval = DefaultPollInterval

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
