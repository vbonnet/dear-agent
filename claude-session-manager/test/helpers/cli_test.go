package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCLI_OutputCapture(t *testing.T) {
	// Use a simple command that we know exists (echo via sh)
	// This tests output capture without requiring AGM binary

	// Create a test script that prints to stdout and stderr
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-script.sh")
	script := `#!/bin/sh
echo "stdout message"
echo "stderr message" >&2
exit 0
`
	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	// Temporarily replace "agm" with our test script
	// We'll modify RunCLI to accept a binary path parameter in real implementation
	// For now, test the concept with a shell command

	// Since RunCLI hardcodes "agm", we'll test with a mock that uses sh -c
	// This is a limitation of the current implementation
	// For now, skip this test if AGM is not available
	t.Skip("Requires AGM binary or mock - will implement after AGM is in PATH")
}

func TestRunCLI_ExitCode_Success(t *testing.T) {
	// Test with a simple successful command
	t.Skip("Requires AGM binary or mock - will implement after AGM is in PATH")
}

func TestRunCLI_ExitCode_Failure(t *testing.T) {
	// Test with a command that exits with non-zero code
	t.Skip("Requires AGM binary or mock - will implement after AGM is in PATH")
}

func TestRunCLI_EnvironmentIsolation(t *testing.T) {
	// Verify that RunCLI creates isolated environment directories
	// This tests the isolation logic without requiring AGM binary

	// We can't test RunCLI directly without AGM, but we can test
	// the isolation setup by examining what directories would be created

	// For now, verify the implementation sets up correct env vars
	// by examining the source code

	// This test will be comprehensive once AGM is available
	t.Skip("Requires AGM binary to verify environment isolation")
}

func TestRunCLI_IsolatedHomeDirectory(t *testing.T) {
	// Test that HOME is set to isolated temp directory
	t.Skip("Requires AGM binary or mock - will implement after AGM is in PATH")
}

func TestGetPath(t *testing.T) {
	// Test the getPath helper function
	path := getPath()

	// Verify PATH is not empty
	assert.NotEmpty(t, path, "getPath should return non-empty PATH")

	// Verify PATH contains typical directories
	assert.True(t,
		strings.Contains(path, "/bin") || strings.Contains(path, "/usr/bin"),
		"getPath should include standard bin directories")
}

// TestRunCLI_Mock tests RunCLI with a mock command (sh)
// This verifies output capture and exit codes work correctly
func TestRunCLI_Mock(t *testing.T) {
	// Create a wrapper script that acts like AGM
	tmpDir := t.TempDir()
	mockBinary := filepath.Join(tmpDir, "agm")

	// Create mock AGM binary that prints to stdout/stderr
	mockScript := `#!/bin/sh
echo "mock stdout output"
echo "mock stderr output" >&2
exit 0
`
	err := os.WriteFile(mockBinary, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Modify PATH to include our mock binary
	// This is hacky but allows testing RunCLI without real AGM
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir+":"+oldPath)

	// Run the mock command
	result := RunCLI(t, "test-arg")

	// Verify output capture
	assert.Contains(t, result.Stdout, "mock stdout output",
		"Stdout should capture command output")
	assert.Contains(t, result.Stderr, "mock stderr output",
		"Stderr should capture error output")

	// Verify exit code
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0 for successful command")
}

// TestRunCLI_MockFailure tests exit code handling for failed commands
func TestRunCLI_MockFailure(t *testing.T) {
	// Create a wrapper script that exits with error
	tmpDir := t.TempDir()
	mockBinary := filepath.Join(tmpDir, "agm")

	// Create mock AGM binary that exits with code 1
	mockScript := `#!/bin/sh
echo "error: something went wrong" >&2
exit 1
`
	err := os.WriteFile(mockBinary, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Modify PATH to include our mock binary
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir+":"+oldPath)

	// Run the mock command
	result := RunCLI(t, "test-arg")

	// Verify error output
	assert.Contains(t, result.Stderr, "error: something went wrong",
		"Stderr should capture error message")

	// Verify exit code is non-zero
	assert.Equal(t, 1, result.ExitCode, "Exit code should be 1 for failed command")
}

// TestRunCLI_EnvironmentIsolation_Mock tests environment variable isolation
func TestRunCLI_EnvironmentIsolation_Mock(t *testing.T) {
	// Create a wrapper script that prints environment variables
	tmpDir := t.TempDir()
	mockBinary := filepath.Join(tmpDir, "agm")

	// Create mock AGM binary that prints HOME and XDG vars
	mockScript := `#!/bin/sh
echo "HOME=$HOME"
echo "XDG_CONFIG_HOME=$XDG_CONFIG_HOME"
echo "XDG_DATA_HOME=$XDG_DATA_HOME"
echo "XDG_STATE_HOME=$XDG_STATE_HOME"
echo "XDG_CACHE_HOME=$XDG_CACHE_HOME"
`
	err := os.WriteFile(mockBinary, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Modify PATH to include our mock binary
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir+":"+oldPath)

	// Save current HOME to verify isolation
	originalHome := os.Getenv("HOME")

	// Run the mock command
	result := RunCLI(t, "test-arg")

	// Verify HOME is isolated (different from current HOME)
	assert.Contains(t, result.Stdout, "HOME=", "Should print HOME variable")
	assert.NotContains(t, result.Stdout, "HOME="+originalHome,
		"HOME should be isolated from current user HOME")

	// Verify XDG directories are set
	assert.Contains(t, result.Stdout, "XDG_CONFIG_HOME=",
		"XDG_CONFIG_HOME should be set")
	assert.Contains(t, result.Stdout, "XDG_DATA_HOME=",
		"XDG_DATA_HOME should be set")
	assert.Contains(t, result.Stdout, "XDG_STATE_HOME=",
		"XDG_STATE_HOME should be set")
	assert.Contains(t, result.Stdout, "XDG_CACHE_HOME=",
		"XDG_CACHE_HOME should be set")

	// Verify XDG directories point to subdirectories of isolated HOME
	// Extract HOME from output
	lines := strings.Split(result.Stdout, "\n")
	var isolatedHome string
	for _, line := range lines {
		if home, found := strings.CutPrefix(line, "HOME="); found {
			isolatedHome = home
			break
		}
	}

	require.NotEmpty(t, isolatedHome, "Should have captured isolated HOME")

	// Verify all XDG vars are under isolated HOME
	for _, line := range lines {
		if strings.HasPrefix(line, "XDG_") {
			xdgPath := strings.SplitN(line, "=", 2)[1]
			assert.True(t, strings.HasPrefix(xdgPath, isolatedHome),
				"XDG directory %s should be under isolated HOME %s", xdgPath, isolatedHome)
		}
	}
}
