package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
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
	return capturePane(ctx, sessionName, lines, false)
}

// CapturePaneANSIOutputContext captures pane output with tmux's escaped style
// sequences intact. Consumers that distinguish rendered placeholder text from
// human-authored input must use this single-capture form to avoid a state race.
func CapturePaneANSIOutputContext(ctx context.Context, sessionName string, lines int) (string, error) {
	return capturePane(ctx, sessionName, lines, true)
}

// CapturePaneANSIOutputTargetContext captures one already-resolved pane target
// with styling intact. The caller owns target resolution so liveness, capture,
// and later delivery can remain pinned to the same pane.
func CapturePaneANSIOutputTargetContext(ctx context.Context, paneID string, lines int) (string, error) {
	if !isPaneID(paneID) {
		return "", fmt.Errorf("invalid tmux pane ID %q", paneID)
	}
	return capturePaneTarget(ctx, paneID, lines, true)
}

// CapturePaneLogicalANSIOutputTargetContext captures all available scrollback
// for one already-resolved pane, preserving styles and joining terminal-wrapped
// rows back into logical lines. Composer ownership uses this form so a narrow
// pane cannot split a queued marker or generated AGM header out of view.
func CapturePaneLogicalANSIOutputTargetContext(ctx context.Context, paneID string) (string, error) {
	if !isPaneID(paneID) {
		return "", fmt.Errorf("invalid tmux pane ID %q", paneID)
	}
	return capturePaneTargetWithOptions(ctx, paneID, 0, true, true)
}

// CapturePaneHistoryOutput captures all available scrollback from a session's
// active pane.
func CapturePaneHistoryOutput(sessionName string) (string, error) {
	return capturePane(context.Background(), sessionName, 0, false)
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

func capturePane(parent context.Context, sessionName string, lines int, preserveStyle bool) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	return capturePaneTarget(parent, NormalizeTmuxSessionName(sessionName), lines, preserveStyle)
}

func capturePaneTarget(parent context.Context, target string, lines int, preserveStyle bool) (string, error) {
	return capturePaneTargetWithOptions(parent, target, lines, preserveStyle, false)
}

func capturePaneTargetWithOptions(parent context.Context, target string, lines int, preserveStyle, joinWrapped bool) (string, error) {
	if parent == nil {
		parent = context.Background()
	}

	policy := CapturePanePolicy()
	ctx, cancel := context.WithTimeout(parent, policy.Timeout)
	defer cancel()
	cmd := newCapturePaneTargetCommandWithOptions(ctx, target, lines, policy, preserveStyle, joinWrapped)

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
	return newCapturePaneCommandWithStyle(ctx, sessionName, lines, policy, false)
}

func newCapturePaneCommandWithStyle(ctx context.Context, sessionName string, lines int, policy CapturePolicy, preserveStyle bool) *exec.Cmd {
	return newCapturePaneTargetCommandWithStyle(ctx, NormalizeTmuxSessionName(sessionName), lines, policy, preserveStyle)
}

func newCapturePaneTargetCommandWithStyle(ctx context.Context, target string, lines int, policy CapturePolicy, preserveStyle bool) *exec.Cmd {
	return newCapturePaneTargetCommandWithOptions(ctx, target, lines, policy, preserveStyle, false)
}

func newCapturePaneTargetCommandWithOptions(ctx context.Context, target string, lines int, policy CapturePolicy, preserveStyle, joinWrapped bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "tmux", capturePaneTargetCommandArgsWithOptions(target, lines, preserveStyle, joinWrapped)...)
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
	return capturePaneCommandArgs(sessionName, lines, false)
}

// CapturePaneANSICommandArgs returns the canonical pane-capture invocation
// with style escape sequences preserved for rendered-input classification.
func CapturePaneANSICommandArgs(sessionName string, lines int) []string {
	return capturePaneCommandArgs(sessionName, lines, true)
}

func capturePaneCommandArgs(sessionName string, lines int, preserveStyle bool) []string {
	return capturePaneTargetCommandArgs(NormalizeTmuxSessionName(sessionName), lines, preserveStyle)
}

func capturePaneTargetCommandArgs(target string, lines int, preserveStyle bool) []string {
	return capturePaneTargetCommandArgsWithOptions(target, lines, preserveStyle, false)
}

func capturePaneTargetCommandArgsWithOptions(target string, lines int, preserveStyle, joinWrapped bool) []string {
	start := "-"
	if lines > 0 {
		start = fmt.Sprintf("-%d", lines)
	}
	// capture-pane targets panes, so the exact-session "=" prefix is invalid.
	args := []string{
		"-S", GetSocketPath(),
		"capture-pane", "-t", target,
		"-p",
	}
	if preserveStyle {
		args = append(args, "-e")
	}
	if joinWrapped {
		args = append(args, "-J")
	}
	return append(args, "-S", start)
}

func isPaneID(target string) bool {
	number, ok := strings.CutPrefix(target, "%")
	if !ok || number == "" {
		return false
	}
	id, err := strconv.Atoi(number)
	return err == nil && id >= 0
}
