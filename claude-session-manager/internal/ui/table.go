package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// FormatTable formats manifests as aligned table
func FormatTable(manifests []*manifest.Manifest) string {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)

	// Header
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
		Blue("SESSION ID"),
		Blue("TMUX"),
		Blue("STATUS"),
		Blue("LAST ACTIVITY"),
		Blue("WORKTREE"))

	// Rows
	for _, m := range manifests {
		status := m.Status
		switch m.Status {
		case manifest.StatusActive:
			status = Green(m.Status)
		case manifest.StatusStale:
			status = Yellow(m.Status)
		case manifest.StatusArchived:
			status = Red(m.Status)
		}

		lastActivity := formatTime(m.LastActivity)
		worktree := truncatePath(m.Worktree.Path, 40)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			m.SessionID,
			m.Tmux.SessionName,
			status,
			lastActivity,
			worktree)
	}

	w.Flush()
	return buf.String()
}

// FormatJSON formats manifests as JSON
func FormatJSON(manifests []*manifest.Manifest) (string, error) {
	data, err := json.MarshalIndent(manifests, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// formatTime formats time as relative (e.g., "2h ago", "3d ago")
func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}

	return t.Format("2006-01-02")
}

// truncatePath truncates path with ... if too long
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

// PrintError prints error message with Problem → Cause → Solution format
func PrintError(err error, cause, solution string) {
	fmt.Printf("%s %s\n\n", Red("❌"), err.Error())
	if cause != "" {
		fmt.Printf("%s\n\n", cause)
	}
	if solution != "" {
		fmt.Printf("Try:\n%s\n", solution)
	}
}

// PrintSuccess prints success message
func PrintSuccess(message string) {
	fmt.Printf("%s %s\n", Green("✓"), message)
}

// PrintWarning prints warning message
func PrintWarning(message string) {
	fmt.Printf("%s %s\n", Yellow("⚠"), message)
}
