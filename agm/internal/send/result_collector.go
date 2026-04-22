package send

import (
	"fmt"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// DeliveryReport aggregates delivery results
type DeliveryReport struct {
	TotalRecipients int
	SuccessCount    int
	FailureCount    int
	Results         []*DeliveryResult
	TotalDuration   time.Duration
}

// GenerateReport creates summary from results
func GenerateReport(results []*DeliveryResult) *DeliveryReport {
	if len(results) == 0 {
		return &DeliveryReport{}
	}

	report := &DeliveryReport{
		TotalRecipients: len(results),
		Results:         results,
	}

	// Count successes and failures
	var totalDuration time.Duration
	for _, result := range results {
		if result.Success {
			report.SuccessCount++
		} else {
			report.FailureCount++
		}
		totalDuration += result.Duration
	}
	report.TotalDuration = totalDuration

	// Sort results: successes first, then failures (alphabetical within each group)
	sort.Slice(report.Results, func(i, j int) bool {
		// Sort by success status first (true before false)
		if report.Results[i].Success != report.Results[j].Success {
			return report.Results[i].Success
		}
		// Then alphabetically by recipient name
		return report.Results[i].Recipient < report.Results[j].Recipient
	})

	return report
}

// PrintReport displays formatted, color-coded output
func (r *DeliveryReport) PrintReport() {
	if r.TotalRecipients == 0 {
		fmt.Println("No deliveries to report")
		return
	}

	// Print summary header
	summary := fmt.Sprintf("Sent to %d recipient", r.TotalRecipients)
	if r.TotalRecipients > 1 {
		summary += "s"
	}
	summary += fmt.Sprintf(" (%d succeeded, %d failed) [%s]",
		r.SuccessCount, r.FailureCount, formatDuration(r.TotalDuration))

	if r.FailureCount > 0 {
		fmt.Println(ui.Yellow(summary))
	} else {
		fmt.Println(ui.Green(summary))
	}
	fmt.Println()

	// Print successes
	if r.SuccessCount > 0 {
		fmt.Printf("%s (%d):\n", ui.Green("Success"), r.SuccessCount)
		for _, result := range r.Results {
			if result.Success {
				checkmark := ui.Green("✓")
				fmt.Printf("  %s %s [ID: %s] [%s]\n",
					checkmark, result.Recipient, result.MessageID,
					formatDuration(result.Duration))
			}
		}
		fmt.Println()
	}

	// Print failures
	if r.FailureCount > 0 {
		fmt.Printf("%s (%d):\n", ui.Red("Failed"), r.FailureCount)
		for _, result := range r.Results {
			if !result.Success {
				xmark := ui.Red("✗")
				errorMsg := "unknown error"
				if result.Error != nil {
					errorMsg = result.Error.Error()
				}
				fmt.Printf("  %s %s [Error: %s] [%s]\n",
					xmark, result.Recipient, errorMsg,
					formatDuration(result.Duration))
			}
		}
		fmt.Println()
	}
}

// GetFailedRecipients extracts failed recipient names
func (r *DeliveryReport) GetFailedRecipients() []string {
	var failed []string
	for _, result := range r.Results {
		if !result.Success {
			failed = append(failed, result.Recipient)
		}
	}
	return failed
}

// HasFailures returns true if any deliveries failed
func (r *DeliveryReport) HasFailures() bool {
	return r.FailureCount > 0
}

// formatDuration formats duration for display
// Examples: "0.4s", "1.2s", "45ms"
func formatDuration(d time.Duration) string {
	if d < time.Second {
		// Show milliseconds for sub-second durations
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	// Show seconds with 1 decimal place
	return fmt.Sprintf("%.1fs", d.Seconds())
}
