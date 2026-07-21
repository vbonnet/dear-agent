package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

// CapturePaneOutput captures the last N lines from a session's active pane.
func CapturePaneOutput(sessionName string, lines int) (string, error) {
	return CapturePaneOutputContext(context.Background(), sessionName, lines)
}

// CapturePaneOutputContext captures pane output while bounding the tmux
// subprocess by both the caller context and the adaptive capture timeout.
func CapturePaneOutputContext(ctx context.Context, sessionName string, lines int) (string, error) {
	return capturePane(ctx, sessionName, lines)
}

// CapturePaneHistoryOutput captures all available scrollback from a session's
// active pane.
func CapturePaneHistoryOutput(sessionName string) (string, error) {
	return capturePane(context.Background(), sessionName, 0)
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

func capturePane(parent context.Context, sessionName string, lines int) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	if parent == nil {
		parent = context.Background()
	}

	policy := CapturePanePolicy()
	ctx, cancel := context.WithTimeout(parent, policy.Timeout)
	defer cancel()
	cmd := newCapturePaneCommand(ctx, sessionName, lines, policy)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}

	return stdout.String(), nil
}

// CapturePolicy describes the subprocess safety contract for pane capture.
type CapturePolicy struct {
	Timeout             time.Duration
	WaitDelay           time.Duration
	IsolateProcessGroup bool
}

// CapturePanePolicy returns the safety policy shared by every harness capture.
func CapturePanePolicy() CapturePolicy {
	return CapturePolicy{
		Timeout:             getAdaptiveTimeout(),
		WaitDelay:           time.Second,
		IsolateProcessGroup: true,
	}
}

func newCapturePaneCommand(ctx context.Context, sessionName string, lines int, policy CapturePolicy) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "tmux", CapturePaneCommandArgs(sessionName, lines)...)
	if policy.IsolateProcessGroup {
		cmd.SysProcAttr = procguard.ProcessGroupAttr()
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	cmd.WaitDelay = policy.WaitDelay
	return cmd
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
