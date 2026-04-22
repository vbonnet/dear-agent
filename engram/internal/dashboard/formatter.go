package dashboard

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/cliframe"
)

// FormatSpecificityTable formats specificity metrics as a table.
func FormatSpecificityTable(metrics []SpecificityMetric, format string) (string, error) {
	headers := []string{"Specificity", "Total", "Successes", "Success Rate"}

	var rows [][]string
	for _, m := range metrics {
		rows = append(rows, []string{
			m.Level,
			fmt.Sprintf("%d", m.Total),
			fmt.Sprintf("%d", m.Successes),
			fmt.Sprintf("%.1f%%", m.SuccessRate),
		})
	}

	switch format {
	case "table", "markdown":
		return formatMarkdownTable(headers, rows, "Success Rate by Prompt Specificity"), nil
	case "csv":
		return formatCSV(headers, rows), nil
	case "json":
		return formatJSON(metrics)
	default:
		return formatMarkdownTable(headers, rows, "Success Rate by Prompt Specificity"), nil
	}
}

// FormatExampleTable formats example metrics as a table.
func FormatExampleTable(metrics []ExampleMetric, format string) (string, error) {
	headers := []string{"Example Status", "Total", "Successes", "Success Rate"}

	var rows [][]string
	for _, m := range metrics {
		rows = append(rows, []string{
			m.Status,
			fmt.Sprintf("%d", m.Total),
			fmt.Sprintf("%d", m.Successes),
			fmt.Sprintf("%.1f%%", m.SuccessRate),
		})
	}

	switch format {
	case "table", "markdown":
		return formatMarkdownTable(headers, rows, "Success Rate by Example Presence"), nil
	case "csv":
		return formatCSV(headers, rows), nil
	case "json":
		return formatJSON(metrics)
	default:
		return formatMarkdownTable(headers, rows, "Success Rate by Example Presence"), nil
	}
}

// FormatEfficiencyTable formats efficiency metrics as a table.
func FormatEfficiencyTable(metrics []EfficiencyMetric, format string) (string, error) {
	headers := []string{"Prompt Type", "Avg Tokens", "Avg Retries", "Success Rate"}

	var rows [][]string
	for _, m := range metrics {
		rows = append(rows, []string{
			m.PromptType,
			fmt.Sprintf("%.0f", m.AvgTokens),
			fmt.Sprintf("%.1f", m.AvgRetries),
			fmt.Sprintf("%.1f%%", m.SuccessRate),
		})
	}

	switch format {
	case "table", "markdown":
		return formatMarkdownTable(headers, rows, "Token Efficiency"), nil
	case "csv":
		return formatCSV(headers, rows), nil
	case "json":
		return formatJSON(metrics)
	default:
		return formatMarkdownTable(headers, rows, "Token Efficiency"), nil
	}
}

// FormatTrendTable formats trend metrics as a table.
func FormatTrendTable(metrics []TrendMetric, format string) (string, error) {
	headers := []string{"Date", "Total Launches", "Successes", "Success Rate"}

	var rows [][]string
	for _, m := range metrics {
		rows = append(rows, []string{
			m.Date,
			fmt.Sprintf("%d", m.TotalLaunches),
			fmt.Sprintf("%d", m.Successes),
			fmt.Sprintf("%.1f%%", m.SuccessRate),
		})
	}

	switch format {
	case "table", "markdown":
		return formatMarkdownTable(headers, rows, "Trends Over Time (Last 30 Days)"), nil
	case "csv":
		return formatCSV(headers, rows), nil
	case "json":
		return formatJSON(metrics)
	default:
		return formatMarkdownTable(headers, rows, "Trends Over Time (Last 30 Days)"), nil
	}
}

// formatMarkdownTable formats data as a Markdown table.
func formatMarkdownTable(headers []string, rows [][]string, title string) string {
	var buf strings.Builder

	// Title
	buf.WriteString(title + ":\n")

	// Header row
	buf.WriteString("| " + strings.Join(headers, " | ") + " |\n")

	// Separator row
	buf.WriteString("|")
	for range headers {
		buf.WriteString(" --- |")
	}
	buf.WriteString("\n")

	// Data rows
	for _, row := range rows {
		buf.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}

	buf.WriteString("\n")
	return buf.String()
}

// formatCSV formats data as CSV.
func formatCSV(headers []string, rows [][]string) string {
	var buf strings.Builder
	w := csv.NewWriter(&buf)

	// Write header
	w.Write(headers)

	// Write data rows
	for _, row := range rows {
		w.Write(row)
	}

	w.Flush()
	return buf.String()
}

// formatJSON formats data as JSON using cliframe.
func formatJSON(data interface{}) (string, error) {
	formatter, err := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
	if err != nil {
		return "", fmt.Errorf("create JSON formatter: %w", err)
	}

	bytes, err := formatter.Format(data)
	if err != nil {
		return "", fmt.Errorf("format JSON: %w", err)
	}
	return string(bytes), nil
}
