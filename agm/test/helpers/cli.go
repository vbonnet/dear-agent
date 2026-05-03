package helpers

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

// CLIResult contains the output and exit status of a CLI command.
type CLIResult struct {
	Stdout   string // Standard output
	Stderr   string // Standard error
	ExitCode int    // Exit code (0 = success)
}

// RunCLI executes the AGM binary with isolated environment.
//
// Creates isolated HOME and XDG directories using t.TempDir() to prevent
// tests from interfering with each other or the user's real environment.
//
// Parameters:
//   - t: test context (for temp dir creation and error reporting)
//   - args: command-line arguments to pass to AGM
//
// Returns:
//   - CLIResult with stdout, stderr, and exit code
//
// Example:
//
//	result := helpers.RunCLI(t, "create", "--name", "test-session")
//	if result.ExitCode != 0 {
//	    t.Fatalf("AGM failed: %s", result.Stderr)
//	}
//	fmt.Println(result.Stdout)
//
// Environment Isolation:
//   - HOME: Set to isolated temp directory
//   - XDG_CONFIG_HOME: Set to isolated config directory
//   - XDG_DATA_HOME: Set to isolated data directory
//   - XDG_STATE_HOME: Set to isolated state directory
//   - XDG_CACHE_HOME: Set to isolated cache directory
//
// Requirements:
//   - AGM binary must be in PATH or specify full path as first arg
func RunCLI(t *testing.T, args ...string) CLIResult {
	t.Helper()

	// Create isolated environment directories
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	dataDir := filepath.Join(homeDir, ".local", "share")
	stateDir := filepath.Join(homeDir, ".local", "state")
	cacheDir := filepath.Join(homeDir, ".cache")

	// Prepare command
	cmd := exec.Command("agm", args...)

	// Set up isolated environment
	cmd.Env = []string{
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + configDir,
		"XDG_DATA_HOME=" + dataDir,
		"XDG_STATE_HOME=" + stateDir,
		"XDG_CACHE_HOME=" + cacheDir,
		"PATH=" + getPath(), // Preserve PATH for finding AGM binary
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err := cmd.Run()

	// Determine exit code
	exitCode := 0
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// Command failed to start (binary not found, etc.)
			// Report as exit code -1
			exitCode = -1
		}
	}

	return CLIResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// getPath returns the current PATH environment variable.
// Helper function to preserve PATH when creating isolated environment.
func getPath() string {
	// Use exec.LookPath to get current PATH
	// This ensures we can find the AGM binary
	cmd := exec.Command("sh", "-c", "echo $PATH")
	output, err := cmd.Output()
	if err != nil {
		return "/usr/local/bin:/usr/bin:/bin" // Fallback
	}
	path := string(output)
	// Trim trailing newline
	if len(path) > 0 && path[len(path)-1] == '\n' {
		path = path[:len(path)-1]
	}
	return path
}
