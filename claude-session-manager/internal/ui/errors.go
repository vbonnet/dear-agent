package ui

import "fmt"

// PrintSessionNotFoundError shows a standardized error for session not found
func PrintSessionNotFoundError(identifier, sessionsDir string) {
	PrintError(
		fmt.Errorf("session not found: %s", identifier),
		"Could not resolve identifier to a session",
		fmt.Sprintf("  • List sessions: csm list --all\n"+
			"  • Check sessions directory: %s\n"+
			"  • Import orphaned sessions: csm sync", sessionsDir),
	)
}

// PrintManifestReadError shows a standardized error for manifest read failures
func PrintManifestReadError(err error, manifestPath string) {
	PrintError(err,
		"Failed to read session manifest",
		fmt.Sprintf("  • Check file exists: %s\n"+
			"  • Verify permissions: ls -la %s\n"+
			"  • Restore from backup: csm backup restore", manifestPath, manifestPath),
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
		"CSM requires tmux to manage sessions",
		"  • Install: sudo apt install tmux (Ubuntu/Debian)\n"+
			"  • Install: brew install tmux (macOS)\n"+
			"  • Verify: tmux -V",
	)
}

// PrintClaudeNotFoundError shows a standardized error for Claude CLI not found
func PrintClaudeNotFoundError() {
	PrintError(
		fmt.Errorf("Claude CLI not found"),
		"CSM requires Claude CLI to be installed",
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
			"  • Then archive: csm archive %s\n"+
			"  • Or force archive: csm archive %s --force",
			tmuxName, sessionName, sessionName),
	)
}

// PrintArchivedSessionError shows a standardized error for operations on archived sessions
func PrintArchivedSessionError(sessionID string) {
	PrintError(
		fmt.Errorf("session is archived"),
		"Cannot resume archived sessions",
		fmt.Sprintf("  • Restore session: csm unarchive %s\n"+
			"  • List archived: csm list --all\n"+
			"  • View details: csm list --all | grep %s",
			sessionID[:8], sessionID[:8]),
	)
}
