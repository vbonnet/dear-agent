package table

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	headers := []string{"Name", "Age", "City"}
	table := New(headers)

	require.NotNil(t, table)
	assert.Equal(t, headers, table.headers)
	assert.Empty(t, table.rows)
}

func TestAddRow(t *testing.T) {
	table := New([]string{"Name", "Age"})

	table.AddRow("Alice", "30")
	table.AddRow("Bob", "25")

	assert.Len(t, table.rows, 2)
	assert.Equal(t, []string{"Alice", "30"}, table.rows[0])
	assert.Equal(t, []string{"Bob", "25"}, table.rows[1])
}

func TestAddRowPadding(t *testing.T) {
	table := New([]string{"Name", "Age", "City"})

	// Too few columns - should pad with empty strings
	table.AddRow("Alice", "30")

	require.Len(t, table.rows, 1)
	assert.Equal(t, []string{"Alice", "30", ""}, table.rows[0])
}

func TestAddRowTruncate(t *testing.T) {
	table := New([]string{"Name", "Age"})

	// Too many columns - should truncate
	table.AddRow("Alice", "30", "NYC", "Extra")

	require.Len(t, table.rows, 1)
	assert.Equal(t, []string{"Alice", "30"}, table.rows[0])
}

func TestRender(t *testing.T) {
	table := New([]string{"Name", "Age"}).
		AddRow("Alice", "30").
		AddRow("Bob", "25")

	output := table.Render()

	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "Age")
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "Bob")
	assert.Contains(t, output, "│") // Border character
}

func TestRenderMarkdown(t *testing.T) {
	table := New([]string{"Name", "Age"}).
		AddRow("Alice", "30").
		AddRow("Bob", "25")

	output := table.RenderMarkdown()

	expected := `| Name | Age |
| --- | --- |
| Alice | 30 |
| Bob | 25 |
`

	assert.Equal(t, expected, output)
}

func TestRenderCSV(t *testing.T) {
	table := New([]string{"Name", "Age"}).
		AddRow("Alice", "30").
		AddRow("Bob", "25")

	output := table.RenderCSV()

	expected := `Name,Age
Alice,30
Bob,25
`

	assert.Equal(t, expected, output)
}

func TestRenderCSVEscaping(t *testing.T) {
	table := New([]string{"Name", "Note"}).
		AddRow("Alice", "Has a comma, in name").
		AddRow("Bob", `Has "quotes"`).
		AddRow("Charlie", "Has\nnewline")

	output := table.RenderCSV()

	assert.Contains(t, output, `"Has a comma, in name"`)
	assert.Contains(t, output, `"Has ""quotes"""`)
	assert.Contains(t, output, "\"Has\nnewline\"")
}

func TestWithWriter(t *testing.T) {
	var buf bytes.Buffer

	table := New([]string{"Name", "Age"}).
		WithWriter(&buf).
		AddRow("Alice", "30")

	err := table.Print()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "30")
}

func TestMinimalStyle(t *testing.T) {
	table := New([]string{"Name", "Age"}).
		WithStyle(MinimalStyle()).
		AddRow("Alice", "30")

	output := table.Render()

	// Minimal style should still render but without colors
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "Alice")
}

func TestCalculateColumnWidths(t *testing.T) {
	table := New([]string{"Short", "LongerHeader"}).
		AddRow("VeryLongCell", "X")

	table.calculateColumnWidths()

	assert.Equal(t, 12, table.columnWidths[0]) // "VeryLongCell" is 12 chars
	assert.Equal(t, 12, table.columnWidths[1]) // "LongerHeader" is 12 chars
}

func TestEmptyTable(t *testing.T) {
	table := New([]string{})

	assert.Empty(t, table.Render())
	assert.Empty(t, table.RenderMarkdown())
	assert.Empty(t, table.RenderCSV())
}

func TestSingleColumn(t *testing.T) {
	table := New([]string{"Name"}).
		AddRow("Alice").
		AddRow("Bob")

	output := table.Render()

	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "Bob")
}

func TestRenderSeparator(t *testing.T) {
	table := New([]string{"Col1", "Col2"}).
		AddRow("A", "B")

	table.calculateColumnWidths()
	separator := table.renderSeparator()

	// Should contain horizontal line characters
	assert.Contains(t, separator, "─")
	assert.Contains(t, separator, "┼")
}

func TestMultipleFormats(t *testing.T) {
	table := New([]string{"Name", "Age"}).
		AddRow("Alice", "30").
		AddRow("Bob", "25")

	// All formats should work without error
	assert.NotEmpty(t, table.Render())
	assert.NotEmpty(t, table.RenderMarkdown())
	assert.NotEmpty(t, table.RenderCSV())

	// Markdown and CSV should not contain ANSI codes
	markdown := table.RenderMarkdown()
	csv := table.RenderCSV()

	assert.NotContains(t, markdown, "\x1b") // No ANSI escape codes
	assert.NotContains(t, csv, "\x1b")
}

func TestChainedCalls(t *testing.T) {
	var buf bytes.Buffer

	err := New([]string{"Name", "Age"}).
		WithStyle(MinimalStyle()).
		WithWriter(&buf).
		AddRow("Alice", "30").
		AddRow("Bob", "25").
		Print()

	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "Bob")
}

func TestAlternatingRowStyles(t *testing.T) {
	table := New([]string{"Name"}).
		AddRow("Alice").
		AddRow("Bob").
		AddRow("Charlie")

	output := table.Render()

	// Should render all rows (style differences won't be visible in test)
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "Bob")
	assert.Contains(t, output, "Charlie")

	// Count newlines to ensure all rows rendered
	lines := strings.Split(output, "\n")
	assert.GreaterOrEqual(t, len(lines), 5) // Header + separator + 3 rows + trailing newline
}
