package progress

import (
	"os"

	"golang.org/x/term"
)

// IsTTY returns true if stdout is connected to a terminal (interactive mode).
// Returns false for pipes, file redirects, and CI/CD environments.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// GetTerminalWidth returns the terminal width in characters.
// If not in TTY mode, returns 0.
// Caps maximum width at 100 characters (respects user preference).
func GetTerminalWidth() int {
	if !IsTTY() {
		return 0
	}

	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		return 100 // Default to 100 chars
	}

	// Cap at 100 chars (user preference from ~/.claude/CLAUDE.md)
	if width > 100 {
		return 100
	}

	return width
}
