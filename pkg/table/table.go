package table

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Style defines the visual appearance of a table.
type Style struct {
	HeaderStyle lipgloss.Style
	RowStyle    lipgloss.Style
	AltRowStyle lipgloss.Style
	BorderStyle lipgloss.Style
}

// DefaultStyle returns a clean, modern table style.
func DefaultStyle() Style {
	return Style{
		HeaderStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63")).
			Padding(0, 1),
		RowStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),
		AltRowStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1),
		BorderStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),
	}
}

// MinimalStyle returns a minimal table style without colors.
func MinimalStyle() Style {
	return Style{
		HeaderStyle: lipgloss.NewStyle().Bold(true).Padding(0, 1),
		RowStyle:    lipgloss.NewStyle().Padding(0, 1),
		AltRowStyle: lipgloss.NewStyle().Padding(0, 1),
		BorderStyle: lipgloss.NewStyle(),
	}
}

// Table represents a formatted table with rows and columns.
type Table struct {
	headers      []string
	rows         [][]string
	style        Style
	columnWidths []int
	writer       io.Writer
}

// New creates a new table with the given headers.
func New(headers []string) *Table {
	return &Table{
		headers: headers,
		rows:    [][]string{},
		style:   DefaultStyle(),
		writer:  nil,
	}
}

// WithStyle sets a custom style for the table.
func (t *Table) WithStyle(style Style) *Table {
	t.style = style
	return t
}

// WithWriter sets the output writer for the table.
func (t *Table) WithWriter(w io.Writer) *Table {
	t.writer = w
	return t
}

// AddRow adds a row of data to the table.
func (t *Table) AddRow(columns ...string) *Table {
	if len(columns) != len(t.headers) {
		// Pad or truncate to match header count
		if len(columns) < len(t.headers) {
			for i := len(columns); i < len(t.headers); i++ {
				columns = append(columns, "")
			}
		} else {
			columns = columns[:len(t.headers)]
		}
	}
	t.rows = append(t.rows, columns)
	return t
}

// calculateColumnWidths determines the width of each column.
func (t *Table) calculateColumnWidths() {
	t.columnWidths = make([]int, len(t.headers))

	// Start with header widths
	for i, header := range t.headers {
		t.columnWidths[i] = lipgloss.Width(header)
	}

	// Update with row widths
	for _, row := range t.rows {
		for i, cell := range row {
			width := lipgloss.Width(cell)
			if width > t.columnWidths[i] {
				t.columnWidths[i] = width
			}
		}
	}
}

// Render returns the formatted table as a string.
func (t *Table) Render() string {
	if len(t.headers) == 0 {
		return ""
	}

	t.calculateColumnWidths()

	var buf strings.Builder

	// Render header
	buf.WriteString(t.renderRow(t.headers, t.style.HeaderStyle))
	buf.WriteString("\n")

	// Render separator
	buf.WriteString(t.renderSeparator())
	buf.WriteString("\n")

	// Render rows
	for i, row := range t.rows {
		style := t.style.RowStyle
		if i%2 == 1 {
			style = t.style.AltRowStyle
		}
		buf.WriteString(t.renderRow(row, style))
		buf.WriteString("\n")
	}

	return buf.String()
}

// renderRow renders a single row with the given style.
func (t *Table) renderRow(columns []string, style lipgloss.Style) string {
	var cells []string

	for i, col := range columns {
		width := t.columnWidths[i]
		// Pad to column width
		padded := col + strings.Repeat(" ", width-lipgloss.Width(col))
		cells = append(cells, style.Render(padded))
	}

	return strings.Join(cells, t.style.BorderStyle.Render(" │ "))
}

// renderSeparator renders the separator line between header and rows.
func (t *Table) renderSeparator() string {
	var parts []string

	for _, width := range t.columnWidths {
		parts = append(parts, strings.Repeat("─", width+2)) // +2 for padding
	}

	return t.style.BorderStyle.Render(strings.Join(parts, "┼"))
}

// Print renders and prints the table to the configured writer (or stdout).
func (t *Table) Print() error {
	output := t.Render()

	if t.writer != nil {
		_, err := fmt.Fprint(t.writer, output)
		return err
	}

	fmt.Print(output)
	return nil
}

// RenderMarkdown renders the table as Markdown format.
func (t *Table) RenderMarkdown() string {
	if len(t.headers) == 0 {
		return ""
	}

	var buf strings.Builder

	// Header row
	buf.WriteString("| ")
	buf.WriteString(strings.Join(t.headers, " | "))
	buf.WriteString(" |\n")

	// Separator row
	buf.WriteString("|")
	for range t.headers {
		buf.WriteString(" --- |")
	}
	buf.WriteString("\n")

	// Data rows
	for _, row := range t.rows {
		buf.WriteString("| ")
		buf.WriteString(strings.Join(row, " | "))
		buf.WriteString(" |\n")
	}

	return buf.String()
}

// RenderCSV renders the table as CSV format.
func (t *Table) RenderCSV() string {
	if len(t.headers) == 0 {
		return ""
	}

	var buf strings.Builder

	// Header
	buf.WriteString(strings.Join(escapeCSVRow(t.headers), ","))
	buf.WriteString("\n")

	// Rows
	for _, row := range t.rows {
		buf.WriteString(strings.Join(escapeCSVRow(row), ","))
		buf.WriteString("\n")
	}

	return buf.String()
}

// escapeCSVRow escapes CSV cells that contain commas, quotes, or newlines.
func escapeCSVRow(row []string) []string {
	escaped := make([]string, len(row))
	for i, cell := range row {
		if strings.ContainsAny(cell, ",\"\n") {
			escaped[i] = `"` + strings.ReplaceAll(cell, `"`, `""`) + `"`
		} else {
			escaped[i] = cell
		}
	}
	return escaped
}
