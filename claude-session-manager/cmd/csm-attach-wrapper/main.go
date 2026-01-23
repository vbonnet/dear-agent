package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Validate arguments
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: csm-attach-wrapper <session-name>")
	}
	sessionName := os.Args[1]

	// Get socket path from CSM configuration
	socketPath := tmux.GetSocketPath()

	// Attach to tmux session (blocks until user exits)
	if err := attachToSession(socketPath, sessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session %q:\n"+
			"  • Verify session exists: tmux -S %s list-sessions\n"+
			"  • Check tmux server: pgrep -a tmux\n"+
			"  • Create session: csm new %s\n"+
			"  Original error: %w",
			sessionName, socketPath, sessionName, err)
	}

	// Capture pane content after exit (best-effort)
	if err := captureAndPrint(socketPath, sessionName); err != nil {
		// Capture failure is non-fatal (attach succeeded)
		fmt.Fprintf(os.Stderr, "Warning: failed to capture pane content: %v\n", err)
	}

	return nil
}

// attachToSession attaches to the tmux session with full terminal passthrough
func attachToSession(socketPath, sessionName string) error {
	cmd := exec.Command("tmux", "-S", socketPath, "attach-session", "-t", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// captureAndPrint captures the pane content and prints to stdout
func captureAndPrint(socketPath, sessionName string) error {
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-p", "-S", "-", "-t", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("capture-pane failed: %w", err)
	}

	// Print captured output (only if non-empty)
	if len(output) > 0 {
		fmt.Print(string(output))
	}

	return nil
}
