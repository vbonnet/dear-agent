package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
)

// Lipgloss styles for table formatting
var (
	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")). // Bright green
			Bold(true)

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")) // Bright yellow

	staleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // Bright red
			Faint(true)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Underline(true).
			Foreground(lipgloss.Color("14")) // Bright cyan

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // Bright black (gray)
)

// FormatTable formats manifests with enhanced lipgloss styling and grouping
func FormatTable(manifests []*manifest.Manifest, tmux session.TmuxInterface) string {
	// Handle empty list
	if len(manifests) == 0 {
		return "No sessions found.\n\nCreate one:\n  csm new <project-name>\n"
	}

	// Compute status and attachment info for all manifests
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmux)

	// Group manifests by status (active/stopped/stale)
	groups := groupByStatus(manifests, statuses)

	// Check if we should show TMUX column (only if any session has NAME != TMUX)
	showTmuxColumn := shouldShowTmuxColumn(manifests)

	// Render each group
	var sections []string
	for _, statusKey := range []string{"active", "stopped", "stale"} {
		group := groups[statusKey]
		if len(group) == 0 {
			continue // Skip empty groups
		}

		// Render group header
		header := renderGroupHeader(statusKey, len(group))
		sections = append(sections, header)

		// Render group table
		table := renderGroupTable(group, statusKey, statuses, showTmuxColumn)
		sections = append(sections, table)
	}

	// Join sections with blank lines
	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
}

// FormatTableLegacy formats manifests as aligned table (v2 schema, old style)
// Preserved for backward compatibility with --legacy flag
func FormatTableLegacy(manifests []*manifest.Manifest, tmux session.TmuxInterface) string {
	// Compute status for all manifests
	statuses := session.ComputeStatusBatch(manifests, tmux)

	// First pass: format entire table without color to get proper alignment
	var tableBuf bytes.Buffer
	w := tabwriter.NewWriter(&tableBuf, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
		"NAME",
		"TMUX",
		"STATUS",
		"UPDATED",
		"PROJECT")

	// Rows
	for _, m := range manifests {
		status := statuses[m.Name]
		updated := formatTime(m.UpdatedAt)
		project := truncatePath(m.Context.Project, 40)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			m.Name,
			m.Tmux.SessionName,
			status,
			updated,
			project)
	}

	w.Flush()

	// Second pass: apply color to entire lines based on status
	var result bytes.Buffer
	lines := bytes.Split(tableBuf.Bytes(), []byte("\n"))

	// Color the header (first line)
	if len(lines) > 0 {
		result.WriteString(Bold(string(lines[0])))
		result.WriteString("\n")
	}

	// Color data rows based on status
	for i, m := range manifests {
		if i+1 >= len(lines) {
			break
		}
		line := string(lines[i+1])
		if line == "" {
			continue
		}

		status := statuses[m.Name]
		switch status {
		case "active":
			line = Green(line)
		case "stopped":
			line = Yellow(line)
		case "archived":
			line = Red(line)
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return result.String()
}

// groupByStatus groups manifests into active/stopped/stale categories
func groupByStatus(manifests []*manifest.Manifest, statuses map[string]session.StatusInfo) map[string][]*manifest.Manifest {
	groups := map[string][]*manifest.Manifest{
		"active":  {},
		"stopped": {},
		"stale":   {},
	}

	staleThreshold := 7 * 24 * time.Hour

	for _, m := range manifests {
		statusInfo := statuses[m.Name]
		if statusInfo.Status == "active" {
			groups["active"] = append(groups["active"], m)
		} else if statusInfo.Status == "stopped" {
			// Check if session is stale (stopped for more than 7 days)
			if time.Since(m.UpdatedAt) >= staleThreshold {
				groups["stale"] = append(groups["stale"], m)
			} else {
				groups["stopped"] = append(groups["stopped"], m)
			}
		}
		// "archived" sessions are not shown in list output
	}

	return groups
}

// shouldShowTmuxColumn returns true if any session has NAME != TMUX
func shouldShowTmuxColumn(manifests []*manifest.Manifest) bool {
	for _, m := range manifests {
		if m.Name != m.Tmux.SessionName {
			return true
		}
	}
	return false
}

// renderGroupHeader renders a styled group header (e.g., "Active Sessions (4)")
func renderGroupHeader(status string, count int) string {
	displayStatus := strings.Title(status) // "active" → "Active"
	text := fmt.Sprintf("%s Sessions (%d)", displayStatus, count)
	return headerStyle.Render(text)
}

// getStatusSymbol returns the Unicode symbol for a status
func getStatusSymbol(status string) string {
	cfg := GetGlobalConfig()

	symbolMap := map[string]string{
		"active":  "●", // U+25CF filled circle
		"stopped": "○", // U+25CB empty circle
		"stale":   "⊗", // U+2297 circled times
	}

	symbol := symbolMap[status]

	// Check screen-reader mode
	if cfg.UI.ScreenReader || os.Getenv("CSM_SCREEN_READER") != "" {
		// Update ScreenReaderText to handle our new symbols
		switch symbol {
		case "●":
			return "[ACTIVE]"
		case "○":
			return "[STOPPED]"
		case "⊗":
			return "[STALE]"
		default:
			return ScreenReaderText(symbol)
		}
	}

	return symbol
}

// renderGroupTable renders a table for a single status group
func renderGroupTable(group []*manifest.Manifest, status string, statuses map[string]session.StatusInfo, showTmuxColumn bool) string {
	// First pass: format entire table without color to get proper alignment
	var tableBuf bytes.Buffer
	w := tabwriter.NewWriter(&tableBuf, 0, 0, 2, ' ', 0)

	// Header
	if showTmuxColumn {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			"NAME",
			"TMUX",
			"STATUS",
			"PROJECT")
	} else {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			"NAME",
			"STATUS",
			"PROJECT")
	}

	// Rows
	for _, m := range group {
		statusInfo := statuses[m.Name]
		symbol := getStatusSymbol(status)

		// Format status text with attachment count if active and attached
		statusText := fmt.Sprintf("%s %s", symbol, strings.Title(statusInfo.Status))
		if statusInfo.Status == "active" && statusInfo.AttachedClients > 0 {
			statusText = fmt.Sprintf("%s %s (%d att.)", symbol, strings.Title(statusInfo.Status), statusInfo.AttachedClients)
		}

		project := compactPath(truncatePath(m.Context.Project, 50))

		if showTmuxColumn {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				m.Name,
				m.Tmux.SessionName,
				statusText,
				project)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				m.Name,
				statusText,
				project)
		}
	}

	w.Flush()

	// Second pass: apply lipgloss styling to entire lines
	var result bytes.Buffer
	lines := bytes.Split(tableBuf.Bytes(), []byte("\n"))

	// Color the header (first line)
	if len(lines) > 0 {
		result.WriteString(Bold(string(lines[0])))
		result.WriteString("\n")
	}

	// Choose style based on status
	var style lipgloss.Style
	switch status {
	case "active":
		style = activeStyle
	case "stopped":
		style = stoppedStyle
	case "stale":
		style = staleStyle
	default:
		style = lipgloss.NewStyle() // No styling
	}

	// Color data rows
	for i := range group {
		if i+1 >= len(lines) {
			break
		}
		line := string(lines[i+1])
		if line == "" {
			continue
		}

		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
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

// compactPath replaces /home/user/ with ~/ to make paths more compact
func compactPath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path // If we can't get home dir, return unchanged
	}
	if strings.HasPrefix(path, homeDir+"/") {
		return "~/" + path[len(homeDir)+1:]
	}
	if path == homeDir {
		return "~"
	}
	return path
}

// ScreenReaderText returns accessible text for symbols
func ScreenReaderText(symbol string) string {
	switch symbol {
	case "✓":
		return "[SUCCESS]"
	case "❌":
		return "[ERROR]"
	case "⚠", "⚠️":
		return "[WARNING]"
	case "○":
		return "[INFO]"
	default:
		return symbol
	}
}

// PrintError prints error message with Problem → Cause → Solution format
func PrintError(err error, cause, solution string) {
	cfg := GetGlobalConfig()
	symbol := "❌"
	// Check --screen-reader flag first
	if cfg.UI.ScreenReader {
		symbol = ScreenReaderText(symbol)
	} else if os.Getenv("CSM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n\n", Red(symbol), err.Error())
	if cause != "" {
		fmt.Printf("%s\n\n", cause)
	}
	if solution != "" {
		fmt.Printf("Try:\n%s\n", solution)
	}
}

// PrintSuccess prints success message
func PrintSuccess(message string) {
	cfg := GetGlobalConfig()
	symbol := "✓"
	// Check --screen-reader flag first
	if cfg.UI.ScreenReader {
		symbol = ScreenReaderText(symbol)
	} else if os.Getenv("CSM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n", Green(symbol), message)
}

// PrintSuccessWithDetail prints success with additional context
func PrintSuccessWithDetail(message, detail string) {
	PrintSuccess(message)
	if detail != "" {
		fmt.Printf("  %s\n", detail)
	}
}

// PrintProgressStep prints a step in a multi-step process
func PrintProgressStep(step int, total int, message string) {
	fmt.Printf("%s [%d/%d] %s\n", Blue("→"), step, total, message)
}

// PrintWarning prints warning message
func PrintWarning(message string) {
	cfg := GetGlobalConfig()
	symbol := "⚠"
	// Check --screen-reader flag first
	if cfg.UI.ScreenReader {
		symbol = ScreenReaderText(symbol)
	} else if os.Getenv("CSM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n", Yellow(symbol), message)
}
