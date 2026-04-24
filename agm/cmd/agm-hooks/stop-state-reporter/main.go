package main

import (
	"os"
	"os/exec"
	"strings"
)

// getSessionName returns the AGM session name from the CLAUDE_SESSION_NAME
// env var, falling back to tmux session detection. Returns empty string if
// neither source provides a name.
func getSessionName() string {
	if name := os.Getenv("CLAUDE_SESSION_NAME"); name != "" {
		return name
	}
	return detectTmuxSession()
}

// detectTmuxSession shells out to tmux to get the current session name.
// Returns empty string on any error (not in tmux, tmux not installed, etc.).
func detectTmuxSession() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// buildAgmCommand constructs the exec.Cmd that will update AGM state.
func buildAgmCommand(sessionName, state, source string) *exec.Cmd {
	cmd := exec.Command("agm", "session", "state", "set",
		sessionName, state, "--source", source)
	cmd.Stdout = os.Stderr // AGM output to stderr (non-blocking)
	cmd.Stderr = os.Stderr
	return cmd
}

// run contains the core logic of the stop-state-reporter hook.
// Returns true if a state update was attempted, false if skipped
// (no session name found).
func run() bool {
	sessionName := getSessionName()
	if sessionName == "" {
		return false
	}

	// Update state to READY via agm CLI
	cmd := buildAgmCommand(sessionName, "READY", "stop-hook")
	_ = cmd.Run() // Ignore errors - Stop hooks are advisory
	return true
}

func main() {
	run()
}
