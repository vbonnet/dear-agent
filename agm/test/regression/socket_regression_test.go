package regression

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitSequenceUsesSocketDetection tests Regression 1: InitSequence Failure
//
// REGRESSION: StartControlModeWithTimeout() used hardcoded GetSocketPath()
// which always returns /tmp/agm.sock (write socket). With dual-socket support,
// sessions could exist on /tmp/csm.sock (legacy socket), causing control mode
// to fail when trying to attach to wrong socket.
//
// FIX: Added findSessionSocket() helper that checks all read sockets
// (GetReadSocketPaths) before starting control mode.
//
// This test verifies control.go uses socket detection not hardcoded paths.
func TestInitSequenceUsesSocketDetection(t *testing.T) {
	controlGoPath := filepath.Join("..", "..", "internal", "tmux", "control.go")

	content, err := os.ReadFile(controlGoPath)
	require.NoError(t, err, "Should be able to read control.go")

	contentStr := string(content)

	// Verify findSessionSocket() function exists
	assert.Contains(t, contentStr, "func findSessionSocket(",
		"control.go should define findSessionSocket() helper")

	// Verify it uses GetReadSocketPaths (not hardcoded GetSocketPath)
	assert.Contains(t, contentStr, "GetReadSocketPaths()",
		"findSessionSocket() should use GetReadSocketPaths() for dual-socket support")

	// Verify StartControlModeWithTimeout uses socket detection
	assert.Contains(t, contentStr, "findSessionSocket(sessionName)",
		"StartControlModeWithTimeout should call findSessionSocket() for socket detection")

	// Verify it does NOT use hardcoded GetSocketPath in control mode startup
	// (GetSocketPath is OK for fallback, but not for initial socket selection)
	lines := strings.Split(contentStr, "\n")
	inStartControlMode := false
	foundHardcodedGetSocketPath := false

	for i, line := range lines {
		// Track when we're inside StartControlModeWithTimeout function
		if strings.Contains(line, "func StartControlModeWithTimeout") {
			inStartControlMode = true
		}

		// If we're in the function and find GetSocketPath before findSessionSocket
		if inStartControlMode && strings.Contains(line, "GetSocketPath()") {
			// Check if this is the problematic hardcoded usage
			// (Not in findSessionSocket function, and before socket detection)
			if !strings.Contains(lines[i-1], "findSessionSocket") &&
				!strings.Contains(line, "// fallback") {
				foundHardcodedGetSocketPath = true
				t.Errorf("Line %d: StartControlModeWithTimeout should use findSessionSocket(), not hardcoded GetSocketPath(): %s",
					i+1, strings.TrimSpace(line))
			}
		}

		// Exit function scope
		if inStartControlMode && strings.HasPrefix(line, "}") &&
			!strings.Contains(line, "//") {
			inStartControlMode = false
		}
	}

	assert.False(t, foundHardcodedGetSocketPath,
		"StartControlModeWithTimeout should not use hardcoded GetSocketPath() for socket selection")
}

// TestNoDefaultSocketFallback tests Regression 5: Default Socket Fallback
//
// REGRESSION: Three tmux commands were missing -S socketPath flag:
// - prompt.go:62 - send-keys for prompt text
// - prompt.go:72 - send-keys for Enter key
// - health.go:66 - list-sessions health probe
//
// Without -S flag, tmux commands default to $TMUX_TMPDIR/default socket,
// bypassing AGM socket isolation.
//
// FIX: Added -S socketPath flag to all three commands.
//
// This test verifies all exec.Command("tmux", ...) calls include -S flag.
func TestNoDefaultSocketFallback(t *testing.T) {
	// Files that should have ALL tmux commands with -S flag
	criticalFiles := []struct {
		path        string
		description string
	}{
		{"../../internal/tmux/prompt.go", "Prompt sending"},
		{"../../internal/tmux/health.go", "Health checks"},
		{"../../internal/tmux/control.go", "Control mode"},
	}

	for _, file := range criticalFiles {
		t.Run(filepath.Base(file.path), func(t *testing.T) {
			content, err := os.ReadFile(file.path)
			require.NoError(t, err, "Should be able to read %s", file.path)

			// Find all exec.Command("tmux", ...) calls
			lines := strings.Split(string(content), "\n")

			for i, line := range lines {
				lineNum := i + 1

				// Skip comments and test files
				if strings.Contains(line, "//") || strings.Contains(file.path, "_test.go") {
					continue
				}

				// Check for tmux command invocations
				if strings.Contains(line, `exec.Command("tmux"`) ||
					strings.Contains(line, "exec.CommandContext(ctx, \"tmux\"") {

					// Verify -S flag is present on same line or next line
					hasSocketFlag := strings.Contains(line, `"-S"`) ||
						(i+1 < len(lines) && strings.Contains(lines[i+1], `"-S"`))

					// Allow exceptions for specific cases documented in comments
					if strings.Contains(line, "// no socket needed") ||
						strings.Contains(line, "tmux -V") { // version check
						continue
					}

					assert.True(t, hasSocketFlag,
						"%s:%d - tmux command missing -S socketPath flag: %s",
						file.path, lineNum, strings.TrimSpace(line))
				}
			}
		})
	}
}

// TestAllTmuxCommandsUseSocketPath scans entire codebase for tmux commands
//
// This is a comprehensive check that ALL tmux exec.Command calls include
// socket path specification, preventing accidental default socket usage.
func TestAllTmuxCommandsUseSocketPath(t *testing.T) {
	tmuxPackagePath := filepath.Join("..", "..", "internal", "tmux")

	// Walk all .go files in tmux package
	err := filepath.Walk(tmuxPackagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip test files (they may use simplified commands for testing)
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Only check .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Read file
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Check each line
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// Skip comments
			if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}

			// Look for tmux command invocations
			if strings.Contains(line, `exec.Command("tmux"`) ||
				strings.Contains(line, "exec.CommandContext(ctx, \"tmux\"") {

				// Check if -S is present in this line or next few lines
				// (commands may span multiple lines)
				hasSocketFlag := strings.Contains(line, `"-S"`)

				// Allow specific exceptions
				isVersionCheck := strings.Contains(line, "tmux -V") || strings.Contains(line, "\"-V\"")
				hasNoSocketComment := strings.Contains(line, "// no socket needed")

				if !hasSocketFlag && !isVersionCheck && !hasNoSocketComment {
					t.Errorf("%s:%d - tmux command should include -S socketPath flag: %s",
						path, lineNum, strings.TrimSpace(line))
				}
			}
		}

		return scanner.Err()
	})

	require.NoError(t, err, "Should be able to walk tmux package")
}

// TestSocketDetectionHelperExists verifies findSessionSocket helper exists
func TestSocketDetectionHelperExists(t *testing.T) {
	controlGoPath := filepath.Join("..", "..", "internal", "tmux", "control.go")

	content, err := os.ReadFile(controlGoPath)
	require.NoError(t, err, "Should be able to read control.go")

	contentStr := string(content)

	// Verify helper function signature
	assert.Contains(t, contentStr, "func findSessionSocket(sessionName string) string",
		"control.go should define findSessionSocket(sessionName string) string")

	// Verify implementation details
	requiredLogic := []string{
		"GetReadSocketPaths()",       // Uses dual-socket paths
		"has-session",                // Checks if session exists
		"for _, socketPath := range", // Iterates all sockets
		"return socketPath",          // Returns found socket
		"return GetSocketPath()",     // Fallback to write socket
	}

	for _, logic := range requiredLogic {
		assert.Contains(t, contentStr, logic,
			"findSessionSocket should implement: %s", logic)
	}
}

// TestSocketRegressionDocumentation ensures socket regressions are documented
func TestSocketRegressionDocumentation(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "AGM-RENAME-REGRESSIONS.md")
	content, err := os.ReadFile(docPath)
	require.NoError(t, err, "Regression documentation should exist")

	contentStr := string(content)

	// Verify Regression 1 (InitSequence) is documented
	assert.Contains(t, contentStr, "Regression 1: InitSequence Failure",
		"Should document InitSequence socket detection regression")

	// Verify Regression 5 (Default Socket Fallback) is documented
	assert.Contains(t, contentStr, "Regression 5: Default Socket Fallback",
		"Should document default socket fallback regression")

	// Verify key technical details
	technicalDetails := []string{
		"findSessionSocket",
		"GetSocketPath()",
		"GetReadSocketPaths()",
		"-S socketPath",
		"prompt.go",
		"health.go",
		"control.go",
	}

	for _, detail := range technicalDetails {
		assert.Contains(t, contentStr, detail,
			"Documentation should mention: %s", detail)
	}
}

// TestDualSocketSupport verifies GetReadSocketPaths exists and returns AGM socket
func TestDualSocketSupport(t *testing.T) {
	socketGoPath := filepath.Join("..", "..", "internal", "tmux", "socket.go")

	content, err := os.ReadFile(socketGoPath)
	require.NoError(t, err, "Should be able to read socket.go")

	contentStr := string(content)

	// Verify GetReadSocketPaths function exists
	assert.Contains(t, contentStr, "func GetReadSocketPaths()",
		"socket.go should define GetReadSocketPaths()")

	// Verify it includes AGM socket
	assert.Contains(t, contentStr, "/tmp/agm.sock",
		"GetReadSocketPaths should include AGM socket")

	// Verify write operations use AGM socket
	assert.Contains(t, contentStr, "func GetSocketPath()",
		"socket.go should define GetSocketPath() for write operations")
}
