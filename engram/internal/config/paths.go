package config

import (
	"os"
	"path/filepath"
)

// ConfigTier represents a configuration tier in the hierarchy
type ConfigTier string

const (
	TierCore    ConfigTier = "core"
	TierCompany ConfigTier = "company"
	TierTeam    ConfigTier = "team"
	TierUser    ConfigTier = "user"
)

// DefaultPaths returns default configuration file paths for each tier
func DefaultPaths() map[ConfigTier]string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "~"
	}

	engramRoot := filepath.Join(homeDir, ".engram")

	return map[ConfigTier]string{
		// TierCore loads from embedded default.yaml (see loader.go)
		TierCore:    filepath.Join(engramRoot, "core", "config.yaml"),
		TierCompany: filepath.Join(engramRoot, "company", "config.yaml"),
		TierTeam:    filepath.Join(engramRoot, "team", "config.yaml"),
		TierUser:    filepath.Join(engramRoot, "user", "config.yaml"),
	}
}
