// Package output provides output-related functionality.
package output

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

var (
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	gray   = color.New(color.FgHiBlack)
)

// FormatStatus formats the progress status for display
func FormatStatus(prog *progress.Progress) string {
	noColor := os.Getenv("NO_COLOR") != ""

	var output string
	output += fmt.Sprintf("S5 Progress (%s):\n", prog.Project)

	passing := 0
	inProgress := 0
	var nextFeature string

	for _, f := range prog.Features {
		status := formatFeatureStatus(f, noColor)
		output += status + "\n"

		switch f.Status {
		case progress.StatusPassing:
			passing++
		case progress.StatusInProgress:
			inProgress++
			if nextFeature == "" {
				nextFeature = f.ID
			}
		default: // failing
			if nextFeature == "" && inProgress == 0 {
				nextFeature = f.ID
			}
		}
	}

	// Progress summary
	total := len(prog.Features)
	percentage := 0
	if total > 0 {
		percentage = (passing * 100) / total
	}

	output += fmt.Sprintf("\nNext: %s\n", formatNextFeature(nextFeature, inProgress))
	output += fmt.Sprintf("Progress: %d/%d features verified (%d%%)\n", passing, total, percentage)

	return output
}

func formatFeatureStatus(f progress.Feature, noColor bool) string {
	var icon, statusText, timestamp string

	switch f.Status {
	case progress.StatusPassing:
		if noColor {
			icon = "✅"
			statusText = "passing"
		} else {
			icon = green.Sprint("✅")
			statusText = green.Sprint("passing")
		}
		if f.VerifiedAt != nil {
			timestamp = fmt.Sprintf(" (verified %s)", formatTime(*f.VerifiedAt))
		}

	case progress.StatusInProgress:
		if noColor {
			icon = "🔄"
			statusText = "in_progress"
		} else {
			icon = yellow.Sprint("🔄")
			statusText = yellow.Sprint("in_progress")
		}
		if f.StartedAt != nil {
			timestamp = fmt.Sprintf(" (since %s)", formatTime(*f.StartedAt))
		}

	default: // failing
		if noColor {
			icon = "⏳"
			statusText = "not started"
		} else {
			icon = gray.Sprint("⏳")
			statusText = gray.Sprint("not started")
		}
	}

	return fmt.Sprintf("%s %s (%s)%s", icon, f.ID, statusText, timestamp)
}

func formatNextFeature(featureID string, inProgress int) string {
	if featureID == "" {
		return "All features complete! 🎉"
	}
	if inProgress > 0 {
		return fmt.Sprintf("Complete %s", featureID)
	}
	return fmt.Sprintf("Start %s", featureID)
}

func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%d min ago", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%d hours ago", hours)
	}
	// Format as date/time
	return t.Format("2006-01-02 15:04")
}

// Success prints a success message
func Success(message string) {
	if os.Getenv("NO_COLOR") != "" {
		fmt.Println("✅", message)
	} else {
		green.Println("✅", message)
	}
}

// Error prints an error message
func Error(message string) {
	if os.Getenv("NO_COLOR") != "" {
		fmt.Fprintln(os.Stderr, "❌", message)
	} else {
		color.New(color.FgRed).Fprintln(os.Stderr, "❌", message)
	}
}

// Info prints an info message
func Info(message string) {
	if os.Getenv("NO_COLOR") != "" {
		fmt.Println("ℹ️", message)
	} else {
		color.New(color.FgCyan).Println("ℹ️", message)
	}
}
