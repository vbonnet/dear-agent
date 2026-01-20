package tmux

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitSequence_CommandSequencing verifies that /rename and /csm-assoc
// are sent as separate commands, not concatenated together
//
// This test uses the actual CSM socket path and tests the real InitSequence implementation
func TestInitSequence_CommandSequencing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	sessionName := "test-cmd-seq-" + time.Now().Format("150405")
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create a new tmux session with a shell (using CSM socket)
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName, "bash")
	err := cmd.Run()
	require.NoError(t, err, "Failed to create tmux session")
	defer exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Give bash time to start
	time.Sleep(300 * time.Millisecond)

	// Use the actual InitSequence implementation
	seq := NewInitSequence(sessionName)

	// Run the sequence
	err = seq.Run()
	if err != nil {
		t.Logf("InitSequence.Run() returned error: %v", err)
		// Don't fail yet - we want to inspect the pane content to understand what happened
	}

	// Capture pane content after the sequence completes
	time.Sleep(500 * time.Millisecond)
	paneContent := capturePaneContent(t, socketPath, sessionName)
	t.Logf("Pane content after InitSequence:\n%s", paneContent)

	// Verify that the commands were sent separately:
	// If they're concatenated, we'd see:
	//   $ /rename test-cmd-seq-HHMMSS
	//   /csm-tools:csm-assoc test-cmd-seq-HHMMSS
	// (both typed but not executed)
	//
	// If they're separate (correct), we'd see:
	//   $ /rename test-cmd-seq-HHMMSS
	//   bash: /rename: command not found
	//   $ /csm-tools:csm-assoc test-cmd-seq-HHMMSS
	//   bash: /csm-tools:csm-assoc: command not found

	// Check that both commands appear in output
	assert.Contains(t, paneContent, "/rename", "Should contain /rename command")
	assert.Contains(t, paneContent, "/csm-tools:csm-assoc", "Should contain /csm-assoc command")

	// Check for bash error messages, which indicate commands were executed
	// (We expect errors because these are Claude commands, not bash commands)
	lines := strings.Split(paneContent, "\n")

	// Find the lines containing our commands
	renameFound := false
	assocFound := false
	renameLineIdx := -1
	assocLineIdx := -1

	for i, line := range lines {
		t.Logf("Line %d: %s", i, line)
		if strings.Contains(line, "/rename") && !strings.Contains(line, "/csm-tools") {
			renameFound = true
			renameLineIdx = i
		}
		if strings.Contains(line, "/csm-tools:csm-assoc") {
			assocFound = true
			assocLineIdx = i
		}
	}

	require.True(t, renameFound, "Should find /rename in output")
	require.True(t, assocFound, "Should find /csm-assoc in output")

	// The bug would manifest as both commands on the SAME line or consecutive lines
	// without any execution happening between them
	if renameLineIdx == assocLineIdx {
		t.Errorf("BUG DETECTED: Both commands on the same line (line %d)", renameLineIdx)
		t.Fatalf("Commands were concatenated together!")
	}

	// Check if they're on immediately consecutive lines (might indicate they were typed
	// together without execution)
	if assocLineIdx == renameLineIdx+1 {
		// This could be the bug - let's check if there's any output between them
		t.Logf("WARNING: Commands on consecutive lines %d and %d", renameLineIdx, assocLineIdx)

		// If bash executed the commands, we should see error messages
		// If they were just typed (not executed), there would be no error messages
		errorsBetween := false
		for i := renameLineIdx; i <= assocLineIdx; i++ {
			if strings.Contains(lines[i], "command not found") || strings.Contains(lines[i], "not recognized") {
				errorsBetween = true
				break
			}
		}

		if !errorsBetween {
			t.Errorf("BUG DETECTED: Commands on consecutive lines WITHOUT execution")
			t.Errorf("This indicates they were typed together but not executed separately")
			t.Fail()
		}
	}

	// Success case: commands are on different lines with execution between them
	t.Logf("SUCCESS: Commands sent separately (rename on line %d, assoc on line %d)", renameLineIdx, assocLineIdx)
}

// capturePaneContent captures the current pane content for debugging
func capturePaneContent(t *testing.T, socketPath, sessionName string) string {
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to capture pane content")
	return string(output)
}
