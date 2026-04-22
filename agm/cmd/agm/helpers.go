package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/claude"
)

const (
	// recentDays is the default filter for recently active sessions
	recentDays = 30
)

// filterRecentSessions filters sessions to those active within the last N days
func filterRecentSessions(sessions []claude.Session, days int) []claude.Session {
	cutoff := time.Now().AddDate(0, 0, -days)
	filtered := make([]claude.Session, 0, len(sessions))

	for _, s := range sessions {
		if s.LastActivity.After(cutoff) {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

// isRunningInVSCode detects if the current process is running in VS Code/Code-OSS
// by checking for VS Code-specific environment variables
func isRunningInVSCode() bool {
	// Check for VS Code environment variables
	// VSCODE_IPC_HOOK_CLI is set when running in VS Code terminal
	// TERM_PROGRAM is set to "vscode" by VS Code
	if os.Getenv("VSCODE_IPC_HOOK_CLI") != "" {
		return true
	}
	if os.Getenv("TERM_PROGRAM") == "vscode" {
		return true
	}
	if os.Getenv("VSCODE_GIT_ASKPASS_NODE") != "" {
		return true
	}
	return false
}

// setTerminalTitle sets the terminal title using ANSI escape sequences
// This works in most modern terminals including VS Code's integrated terminal
func setTerminalTitle(title string) {
	// Use OSC 0 (Operating System Command 0) to set both window and tab title
	// Format: ESC ] 0 ; title BEL
	// \033 = ESC, \007 = BEL (bell)
	fmt.Printf("\033]0;%s\007", title)
}

// updateVSCodeTabTitle updates the VS Code tab title if running in VS Code
// This is called after successfully creating or resuming a AGM session
func updateVSCodeTabTitle(sessionName string) {
	if !isRunningInVSCode() {
		return
	}

	// Set terminal title to session name
	title := fmt.Sprintf("AGM: %s", sessionName)
	setTerminalTitle(title)
}
