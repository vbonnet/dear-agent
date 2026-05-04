package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

var (
	statusFormat string
)

// StatusCmd is the cobra command that shows feature progress.
var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show feature progress",
	Long: `Display current progress of all features tracked in S5-implementation/progress.json.

Shows:
- Feature IDs with status icons (✅ passing, 🔄 in_progress, ⏳ not started)
- Verification timestamps for completed features
- Next feature to work on
- Overall progress percentage

Output formats:
  --format table    Human-readable table (default)
  --format json     JSON output with full metadata

Example:
  wayfinder-features status
  wayfinder-features status --format json`,
	RunE: runStatus,
}

func init() {
	StatusCmd.Flags().StringVar(&statusFormat, "format", "table", "Output format: table or json")
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Find progress file
	progressPath, err := progress.FindProgressFile()
	if err != nil {
		return fmt.Errorf("failed to find progress file: %w", err)
	}

	// Read progress
	prog, err := progress.ReadProgress(progressPath)
	if err != nil {
		return fmt.Errorf("failed to read progress: %w", err)
	}

	// Output using cliframe
	return outputStatus(cmd, prog)
}

// FeatureRow represents a feature for table output
type FeatureRow struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp,omitempty"`
}

// StatusOutput represents the complete status output for JSON
type StatusOutput struct {
	Project    string       `json:"project"`
	Waypoint   string       `json:"waypoint"`
	Features   []FeatureRow `json:"features"`
	Summary    Summary      `json:"summary"`
	NextAction string       `json:"next_action"`
}

// Summary represents progress summary
type Summary struct {
	Total      int `json:"total"`
	Passing    int `json:"passing"`
	InProgress int `json:"in_progress"`
	Percentage int `json:"percentage"`
}

func outputStatus(cmd *cobra.Command, prog *progress.Progress) error {
	// Build output data
	passing := 0
	inProgress := 0
	var nextFeature string
	features := make([]FeatureRow, 0, len(prog.Features))

	for _, f := range prog.Features {
		row := FeatureRow{
			ID:     f.ID,
			Status: formatStatus(f.Status),
		}

		switch {
		case f.Status == progress.StatusPassing && f.VerifiedAt != nil:
			row.Timestamp = formatTimestamp(*f.VerifiedAt)
			passing++
		case f.Status == progress.StatusInProgress && f.StartedAt != nil:
			row.Timestamp = formatTimestamp(*f.StartedAt)
			inProgress++
			if nextFeature == "" {
				nextFeature = f.ID
			}
		default:
			if nextFeature == "" && inProgress == 0 {
				nextFeature = f.ID
			}
		}

		features = append(features, row)
	}

	// Calculate percentage
	total := len(prog.Features)
	percentage := 0
	if total > 0 {
		percentage = (passing * 100) / total
	}

	// Determine next action
	nextAction := "All features complete! 🎉"
	if nextFeature != "" {
		if inProgress > 0 {
			nextAction = fmt.Sprintf("Complete %s", nextFeature)
		} else {
			nextAction = fmt.Sprintf("Start %s", nextFeature)
		}
	}

	output := StatusOutput{
		Project:  prog.Project,
		Waypoint: prog.Waypoint,
		Features: features,
		Summary: Summary{
			Total:      total,
			Passing:    passing,
			InProgress: inProgress,
			Percentage: percentage,
		},
		NextAction: nextAction,
	}

	// Use cliframe for output
	var format cliframe.Format
	var data interface{}

	switch statusFormat {
	case "json":
		format = cliframe.FormatJSON
		data = output
	case "table":
		// For table format, we'll use custom text output for better UX
		return outputStatusText(cmd, output)
	default:
		return fmt.Errorf("unknown format: %s (supported: json, table)", statusFormat)
	}

	formatter, err := cliframe.NewFormatter(format, cliframe.WithPrettyPrint(true))
	if err != nil {
		return err
	}

	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	return writer.WithFormatter(formatter).Output(data)
}

func outputStatusText(cmd *cobra.Command, output StatusOutput) error {
	// Use custom text format for better readability
	fmt.Fprintf(cmd.OutOrStdout(), "S5 Progress (%s):\n", output.Project)

	for _, f := range output.Features {
		statusIcon := getStatusIcon(f.Status)
		line := fmt.Sprintf("%s %s (%s)", statusIcon, f.ID, f.Status)
		if f.Timestamp != "" {
			line += fmt.Sprintf(" %s", f.Timestamp)
		}
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nNext: %s\n", output.NextAction)
	fmt.Fprintf(cmd.OutOrStdout(), "Progress: %d/%d features verified (%d%%)\n",
		output.Summary.Passing, output.Summary.Total, output.Summary.Percentage)

	return nil
}

func formatStatus(status string) string {
	switch status {
	case progress.StatusPassing:
		return "passing"
	case progress.StatusInProgress:
		return "in_progress"
	default:
		return "not started"
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "passing":
		return "✅"
	case "in_progress":
		return "🔄"
	default:
		return "⏳"
	}
}

func formatTimestamp(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "(just now)"
	}
	if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("(%d min ago)", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("(%d hours ago)", hours)
	}
	// Format as date/time
	return fmt.Sprintf("(%s)", t.Format("2006-01-02 15:04"))
}
