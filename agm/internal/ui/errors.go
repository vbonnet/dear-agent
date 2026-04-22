// Package ui provides ui functionality.
package ui

import "fmt"

// PrintSessionNotFoundError shows a standardized error for session not found
func PrintSessionNotFoundError(identifier, sessionsDir string) {
	PrintError(
		fmt.Errorf("session not found: %s", identifier),
		"Could not resolve identifier to a session",
		fmt.Sprintf("  • List sessions: agm session list --all\n"+
			"  • Check sessions directory: %s\n"+
			"  • Import orphaned sessions: agm admin sync", sessionsDir),
	)
}

// PrintManifestReadError shows a standardized error for manifest read failures
func PrintManifestReadError(err error, manifestPath string) {
	PrintError(err,
		"Failed to read session manifest",
		fmt.Sprintf("  • Check file exists: %s\n"+
			"  • Verify permissions: ls -la %s\n"+
			"  • Restore from backup: agm admin backup restore", manifestPath, manifestPath),
	)
}

// PrintManifestWriteError shows a standardized error for manifest write failures
func PrintManifestWriteError(err error) {
	PrintError(err,
		"Failed to write manifest",
		"  • Check disk space: df -h\n"+
			"  • Verify permissions on sessions directory\n"+
			"  • Check file is not locked: lsof manifest.yaml",
	)
}

// PrintTmuxNotFoundError shows a standardized error for tmux not found
func PrintTmuxNotFoundError() {
	PrintError(
		fmt.Errorf("tmux not found"),
		"AGM requires tmux to manage sessions",
		"  • Install: sudo apt install tmux (Ubuntu/Debian)\n"+
			"  • Install: brew install tmux (macOS)\n"+
			"  • Verify: tmux -V",
	)
}

// PrintClaudeNotFoundError shows a standardized error for Claude CLI not found
func PrintClaudeNotFoundError() {
	PrintError(
		fmt.Errorf("Claude CLI not found"),
		"AGM requires Claude CLI to be installed",
		"  • Install from: https://claude.com\n"+
			"  • Run at least once to create history\n"+
			"  • Verify: claude --version",
	)
}

// PrintActiveSessionError shows a standardized error for operations on active sessions
func PrintActiveSessionError(sessionName, tmuxName string) {
	PrintError(
		fmt.Errorf("session is active"),
		fmt.Sprintf("Cannot archive active session '%s'", sessionName),
		fmt.Sprintf("  • Stop tmux session: tmux kill-session -t %s\n"+
			"  • Then archive: agm session archive %s\n"+
			"  • Or force archive: agm session archive %s --force",
			tmuxName, sessionName, sessionName),
	)
}

// PrintArchivedSessionError shows a standardized error for operations on archived sessions
func PrintArchivedSessionError(sessionID string) {
	PrintError(
		fmt.Errorf("session is archived"),
		"Cannot resume archived sessions",
		fmt.Sprintf("  • Restore session: agm session unarchive %s\n"+
			"  • List archived: agm session list --all\n"+
			"  • View details: agm session list --all | grep %s",
			sessionID[:8], sessionID[:8]),
	)
}
