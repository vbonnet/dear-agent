package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
)

// CapturePaneOutput captures last N lines from session's active pane
func CapturePaneOutput(sessionName string, lines int) (string, error) {
	socketPath := GetSocketPath()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)
	// Note: capture-pane targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
	cmd := exec.Command("tmux", "-S", socketPath, // Use AGM-specific socket
		"capture-pane", "-t", normalizedName, "-p", "-S", fmt.Sprintf("-%d", lines))

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}

	return stdout.String(), nil
}
