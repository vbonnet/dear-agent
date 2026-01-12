package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
)

// FormatTable formats manifests as aligned table (v2 schema)
func FormatTable(manifests []*manifest.Manifest, tmux session.TmuxInterface) string {
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
