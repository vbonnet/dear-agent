package common

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const stagingDir = ".perf-bench"

// ScenarioFileCount returns the number of files to stage for a given scenario
func ScenarioFileCount(scenario string) int {
	switch scenario {
	case "empty":
		return 0
	case "small":
		return 10
	case "medium":
		return 50
	default:
		return 10 // default to small
	}
}

// StageTestFiles creates and stages test files for the given scenario.
// Scenario can be "empty" (0 files), "small" (10 files), or "medium" (50 files).
func StageTestFiles(scenario string) error {
	count := ScenarioFileCount(scenario)

	if count == 0 {
		// Nothing to stage for empty scenario
		return nil
	}

	// Create staging directory
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return fmt.Errorf("failed to create staging directory: %w", err)
	}

	// Create and stage test files
	for i := 0; i < count; i++ {
		filename := filepath.Join(stagingDir, fmt.Sprintf("test-file-%03d.txt", i))

		// Write test content
		content := fmt.Sprintf("test file %d\n", i)
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to create test file %s: %w", filename, err)
		}

		// Stage the file with git
		cmd := exec.Command("git", "add", filename)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stage file %s: %w", filename, err)
		}
	}

	return nil
}

// UnstageTestFiles removes staged test files and the staging directory.
func UnstageTestFiles() error {
	// Unstage files (ignore errors if nothing staged)
	cmd := exec.Command("git", "reset", "HEAD", stagingDir)
	_ = cmd.Run() // Ignore error, directory might not be staged

	// Remove staging directory and all contents
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("failed to remove staging directory: %w", err)
	}

	return nil
}
