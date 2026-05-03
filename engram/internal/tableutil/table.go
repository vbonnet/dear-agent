// Package tableutil provides tableutil-related functionality.
package tableutil

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// TableRenderer renders tables using lipgloss
type TableRenderer struct {
	title   string
	headers []string
	rows    [][]string
}

// NewTable creates a new table renderer
func NewTable(title string, headers []string, rows [][]string) *TableRenderer {
	return &TableRenderer{
		title:   title,
		headers: headers,
		rows:    rows,
	}
}

// RenderMarkdown renders the table as a markdown-style table using lipgloss
func (t *TableRenderer) RenderMarkdown() string {
	if len(t.rows) == 0 {
		return ""
	}

	var sb strings.Builder

	// Add title
	if t.title != "" {
		sb.WriteString(t.title + ":\n")
	}

	// Create lipgloss table
	tbl := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers(t.headers...).
		Rows(t.rows...)

	sb.WriteString(tbl.String())
	sb.WriteString("\n\n")

	return sb.String()
}

// RenderPlainMarkdown renders a plain markdown table (for compatibility)
func (t *TableRenderer) RenderPlainMarkdown() string {
	if len(t.rows) == 0 {
		return ""
	}

	var sb strings.Builder

	// Title
	if t.title != "" {
		sb.WriteString(t.title + ":\n")
	}

	// Header row
	sb.WriteString("| " + strings.Join(t.headers, " | ") + " |\n")

	// Separator row
	sb.WriteString("|")
	for range t.headers {
		sb.WriteString(" --- |")
	}
	sb.WriteString("\n")

	// Data rows
	for _, row := range t.rows {
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatCSV formats data as CSV
func FormatCSV(headers []string, rows [][]string, sb *strings.Builder) {
	// Write header
	sb.WriteString(strings.Join(headers, ",") + "\n")

	// Write data rows (basic CSV - no escaping for now)
	for _, row := range rows {
		sb.WriteString(strings.Join(row, ",") + "\n")
	}
}

// FormatJSON formats data as JSON
func FormatJSON(data interface{}) (string, error) {
	// This is a placeholder - actual JSON marshaling should be done by the caller
	// to maintain type safety
	return fmt.Sprintf("%v", data), nil
}
