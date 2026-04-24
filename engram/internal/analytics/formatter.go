package analytics

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	outputformatter "github.com/vbonnet/dear-agent/pkg/output-formatter"
)

// Formatter formats sessions for output
type Formatter interface {
	Format(sessions []Session) (string, error)
}

// MarkdownFormatter formats sessions as human-readable Markdown tables
type MarkdownFormatter struct{}

// Format formats a list of sessions as a Markdown table
func (f *MarkdownFormatter) Format(sessions []Session) (string, error) {
	if len(sessions) == 0 {
		return "No Wayfinder sessions found.\n", nil
	}

	var sb strings.Builder

	sb.WriteString("# Wayfinder Sessions\n\n")

	// Prepare table data
	headers := []string{"Session ID", "Project", "Start Time", "Duration", "Phases", "Status"}
	var rows [][]string

	for _, session := range sessions {
		sessionIDShort := session.ID
		if len(sessionIDShort) > 12 {
			sessionIDShort = sessionIDShort[:12] + "..."
		}

		projectShort := session.ProjectPath
		if len(projectShort) > 30 {
			// Show last 30 chars (most meaningful part of path)
			projectShort = "..." + projectShort[len(projectShort)-27:]
		}

		startTime := session.StartTime.Format("2006-01-02 15:04")
		duration := formatDuration(session.Metrics.TotalDuration)
		phaseCount := fmt.Sprintf("%d", session.Metrics.PhaseCount)
		status := formatStatus(session.Status)

		rows = append(rows, []string{
			sessionIDShort, projectShort, startTime, duration, phaseCount, status,
		})
	}

	// Create and render lipgloss table
	tbl := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers(headers...).
		Rows(rows...)

	sb.WriteString(tbl.String())
	sb.WriteString("\n\n")

	return sb.String(), nil
}

// FormatSession formats a single session with detailed information
func (f *MarkdownFormatter) FormatSession(session *Session) (string, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Session: %s\n\n", session.ID))
	sb.WriteString(fmt.Sprintf("**Project**: %s\n", session.ProjectPath))
	sb.WriteString(fmt.Sprintf("**Start**: %s\n", session.StartTime.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**End**: %s\n", session.EndTime.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Duration**: %s\n", formatDuration(session.Metrics.TotalDuration)))
	sb.WriteString(fmt.Sprintf("**Status**: %s\n", formatStatus(session.Status)))
	sb.WriteString("\n")

	// Phase timeline
	if len(session.Phases) > 0 {
		sb.WriteString("## Phase Timeline\n\n")

		headers := []string{"Phase", "Duration", "Start", "End"}
		var rows [][]string

		for _, phase := range session.Phases {
			start := phase.StartTime.Format("15:04:05")
			end := phase.EndTime.Format("15:04:05")
			duration := formatDuration(phase.Duration)

			rows = append(rows, []string{phase.Name, duration, start, end})
		}

		tbl := table.New().
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
			Headers(headers...).
			Rows(rows...)

		sb.WriteString(tbl.String())
		sb.WriteString("\n\n")
	}

	// Metrics
	sb.WriteString("## Metrics\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Time**: %s\n", formatDuration(session.Metrics.TotalDuration)))
	if session.Metrics.AITime > 0 {
		aiPercent := float64(session.Metrics.AITime) / float64(session.Metrics.TotalDuration) * 100
		sb.WriteString(fmt.Sprintf("- **AI Time**: %s (%.0f%%)\n", formatDuration(session.Metrics.AITime), aiPercent))
	}
	if session.Metrics.WaitTime > 0 {
		waitPercent := float64(session.Metrics.WaitTime) / float64(session.Metrics.TotalDuration) * 100
		sb.WriteString(fmt.Sprintf("- **Wait Time**: %s (%.0f%%)\n", formatDuration(session.Metrics.WaitTime), waitPercent))
	}
	sb.WriteString(fmt.Sprintf("- **Phase Count**: %d\n", session.Metrics.PhaseCount))
	if session.Metrics.EstimatedCost > 0 {
		sb.WriteString(fmt.Sprintf("- **Estimated Cost**: $%.2f\n", session.Metrics.EstimatedCost))
	}

	return sb.String(), nil
}

// FormatSummary formats aggregate statistics
func (f *MarkdownFormatter) FormatSummary(summary SessionSummary) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Wayfinder Session Summary\n\n")

	sb.WriteString("## Session Counts\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Sessions**: %d\n", summary.TotalSessions))
	sb.WriteString(fmt.Sprintf("- **Completed**: %d\n", summary.CompletedSessions))
	sb.WriteString(fmt.Sprintf("- **Failed**: %d\n", summary.FailedSessions))

	if summary.TotalSessions > 0 {
		completionRate := float64(summary.CompletedSessions) / float64(summary.TotalSessions) * 100
		sb.WriteString(fmt.Sprintf("- **Completion Rate**: %.1f%%\n", completionRate))
	}

	sb.WriteString("\n## Time Statistics\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Time**: %s\n", formatDuration(summary.TotalDuration)))
	sb.WriteString(fmt.Sprintf("- **Average Duration**: %s\n", formatDuration(summary.AverageDuration)))
	if summary.TotalAITime > 0 {
		sb.WriteString(fmt.Sprintf("- **Total AI Time**: %s\n", formatDuration(summary.TotalAITime)))
	}
	if summary.TotalWaitTime > 0 {
		sb.WriteString(fmt.Sprintf("- **Total Wait Time**: %s\n", formatDuration(summary.TotalWaitTime)))
	}

	if summary.TotalCost > 0 {
		sb.WriteString("\n## Cost Statistics\n\n")
		sb.WriteString(fmt.Sprintf("- **Total Cost**: $%.2f\n", summary.TotalCost))
		sb.WriteString(fmt.Sprintf("- **Average Cost**: $%.2f\n", summary.AverageCost))
	}

	return sb.String(), nil
}

// JSONFormatter formats sessions as JSON
type JSONFormatter struct {
	Pretty bool // Enable pretty-printing
}

// Format formats sessions as JSON
func (f *JSONFormatter) Format(sessions []Session) (string, error) {
	var data []byte
	var err error

	if f.Pretty {
		data, err = json.MarshalIndent(sessions, "", "  ")
	} else {
		data, err = json.Marshal(sessions)
	}

	if err != nil {
		return "", fmt.Errorf("failed to marshal sessions to JSON: %w", err)
	}

	return string(data), nil
}

// CSVFormatter formats sessions as CSV (for spreadsheet import)
type CSVFormatter struct{}

// Format formats sessions as CSV
func (f *CSVFormatter) Format(sessions []Session) (string, error) {
	var sb strings.Builder
	writer := csv.NewWriter(&sb)

	// Write header
	header := []string{
		"session_id",
		"project_path",
		"start_time",
		"end_time",
		"duration_minutes",
		"ai_time_minutes",
		"wait_time_minutes",
		"phase_count",
		"cost_usd",
		"status",
	}
	if err := writer.Write(header); err != nil {
		return "", fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, session := range sessions {
		row := []string{
			session.ID,
			session.ProjectPath,
			session.StartTime.Format(time.RFC3339),
			session.EndTime.Format(time.RFC3339),
			fmt.Sprintf("%.2f", session.Metrics.TotalDuration.Minutes()),
			fmt.Sprintf("%.2f", session.Metrics.AITime.Minutes()),
			fmt.Sprintf("%.2f", session.Metrics.WaitTime.Minutes()),
			fmt.Sprintf("%d", session.Metrics.PhaseCount),
			fmt.Sprintf("%.2f", session.Metrics.EstimatedCost),
			session.Status,
		}
		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("CSV writer error: %w", err)
	}

	return sb.String(), nil
}

// Helper functions

// formatDuration formats a duration as human-readable string
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// formatStatus formats status with emoji using output-formatter library
func formatStatus(status string) string {
	iconMapper := outputformatter.NewIconMapper(false) // Use emoji icons

	switch status {
	case "completed", "success":
		return iconMapper.GetIcon(outputformatter.StatusSuccess)
	case "failed":
		return iconMapper.GetIcon(outputformatter.StatusFailed)
	case "incomplete":
		// Custom icon for incomplete status (domain-specific to Wayfinder)
		return "⏸️"
	default:
		return status
	}
}
