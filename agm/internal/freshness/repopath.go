package freshness

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindRepoPath locates the agm source repository.
// It checks AGM_SOURCE_DIR env var, then a known default path.
func FindRepoPath() (string, error) {
	// 1. Env var override
	if dir := os.Getenv("AGM_SOURCE_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		return "", fmt.Errorf("AGM_SOURCE_DIR=%s does not contain go.mod", dir)
	}

	// 2. Known default location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	knownPath := filepath.Join(home, "src", "ws", "oss", "repos", "ai-tools", "agm")
	if _, err := os.Stat(filepath.Join(knownPath, "go.mod")); err == nil {
		return knownPath, nil
	}

	return "", fmt.Errorf("agm source repository not found (set AGM_SOURCE_DIR to override)")
}
