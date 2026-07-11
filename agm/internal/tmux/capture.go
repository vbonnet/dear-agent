package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// CapturePaneOutput captures the last N lines from a session's active pane.
func CapturePaneOutput(sessionName string, lines int) (string, error) {
	return capturePane(sessionName, lines)
}

// CapturePaneHistoryOutput captures all available scrollback from a session's
// active pane.
func CapturePaneHistoryOutput(sessionName string) (string, error) {
	return capturePane(sessionName, 0)
}

// CapturePaneLines captures pane output and returns it as individual lines.
// A non-positive line count captures all available history.
func CapturePaneLines(sessionName string, lines int) ([]string, error) {
	var (
		output string
		err    error
	)
	if lines > 0 {
		output, err = CapturePaneOutput(sessionName, lines)
	} else {
		output, err = CapturePaneHistoryOutput(sessionName)
	}
	if err != nil {
		return nil, err
	}

	output = strings.TrimRight(output, "\n")
	if output == "" {
		return []string{}, nil
	}
	return strings.Split(output, "\n"), nil
}

func capturePane(sessionName string, lines int) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}

	cmd := exec.Command("tmux", CapturePaneCommandArgs(sessionName, lines)...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}

	return stdout.String(), nil
}

// CapturePaneCommandArgs returns the canonical tmux arguments used for pane
// capture. It is exposed within AGM so parity checks can validate the command
// contract without starting a tmux server.
func CapturePaneCommandArgs(sessionName string, lines int) []string {
	start := "-"
	if lines > 0 {
		start = fmt.Sprintf("-%d", lines)
	}
	// capture-pane targets panes, so the exact-session "=" prefix is invalid.
	return []string{
		"-S", GetSocketPath(),
		"capture-pane", "-t", NormalizeTmuxSessionName(sessionName),
		"-p", "-S", start,
	}
}
