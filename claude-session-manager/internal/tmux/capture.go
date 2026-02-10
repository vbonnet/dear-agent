package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
)

// CapturePaneOutput captures last N lines from session's active pane
func CapturePaneOutput(sessionName string, lines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane",
		"-t", sessionName,
		"-p",                    // Print to stdout
		"-S", fmt.Sprintf("-%d", lines), // Start from N lines back
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}

	return stdout.String(), nil
}
