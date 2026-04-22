package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var metricsWindow time.Duration
var metricsCheck bool
var metricsAlertFormat string

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Show system metrics and health status",
	Long: `Collect and display system-wide metrics including session counts,
throughput rates, resource usage, and alert conditions.

Output is JSON by default (--output json), suitable for piping to jq
or consumption by other sessions. Use --output text for human-readable output.

With --check flag, only alerts are shown and exit code is 0 if healthy, 1 if any alerts.
This is useful for integration with monitoring loops.`,
	RunE: runMetrics,
}

func init() {
	metricsCmd.Flags().DurationVar(&metricsWindow, "window", contracts.Load().ScanLoop.MetricsWindow.Duration, "Lookback window for throughput metrics (e.g. 1h, 30m)")
	metricsCmd.Flags().BoolVar(&metricsCheck, "check", false, "Health check mode: only show alerts, exit 0 if healthy, 1 if alerts")
	metricsCmd.Flags().StringVar(&metricsAlertFormat, "alert-format", "text", "Alert output format: text or json (only used with --check)")
	rootCmd.AddCommand(metricsCmd)
}

func runMetrics(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return handleError(err)
	}
	defer cleanup()

	result, err := ops.GetMetrics(opCtx, &ops.MetricsRequest{
		Window: metricsWindow,
	})
	if err != nil {
		return handleError(err)
	}

	// Fill in commits per hour from git log (requires shell access)
	commits := countRecentCommits(metricsWindow)
	result.Throughput.CommitsPerHour = commits

	// Populate cost-per-commit now that we have real commit count
	result.Cost.CommitCount = commits
	if commits > 0 && result.Cost.TotalSpend > 0 {
		result.Cost.CostPerCommit = result.Cost.TotalSpend / float64(commits)
	} else {
		result.Cost.CostPerCommit = 0
	}

	// Re-run alert check for throughput now that we have the real value
	if result.Throughput.CommitsPerHour == 0 {
		// Check if alert already exists
		hasAlert := false
		for _, a := range result.Alerts {
			if a.Type == "throughput" {
				hasAlert = true
				break
			}
		}
		if !hasAlert {
			result.Alerts = append(result.Alerts, ops.Alert{
				Level:   "warning",
				Type:    "throughput",
				Message: "No commits in the last hour",
				Value:   "0",
			})
		}
	} else {
		// Remove sentinel throughput alert if commits were found
		filtered := result.Alerts[:0]
		for _, a := range result.Alerts {
			if a.Type != "throughput" {
				filtered = append(filtered, a)
			}
		}
		result.Alerts = filtered
	}

	// Handle --check mode for health monitoring
	if metricsCheck {
		return runMetricsCheck(result)
	}

	return printResult(result, func() {
		printMetricsText(result)
	})
}

// countRecentCommits scans git log across all branches for recent commits.
func countRecentCommits(window time.Duration) int {
	since := time.Now().Add(-window).Format("2006-01-02T15:04:05")
	out, err := exec.Command("git", "log", "--all", "--oneline", "--since="+since).Output()
	if err != nil {
		return 0
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return 0
	}
	return len(strings.Split(lines, "\n"))
}

// printMetricsText renders metrics in a human-readable format.
func printMetricsText(m *ops.MetricsResult) {
	fmt.Printf("AGM Metrics — %s\n\n", m.Timestamp)

	// Sessions
	fmt.Printf("Sessions: %d total (%d active, %d stopped, %d archived)\n",
		m.Sessions.Total, m.Sessions.Active, m.Sessions.Stopped, m.Sessions.Archived)
	if len(m.Sessions.ByState) > 0 {
		fmt.Printf("  States:")
		for state, count := range m.Sessions.ByState {
			fmt.Printf(" %s=%d", state, count)
		}
		fmt.Println()
	}

	// Throughput
	fmt.Printf("\nThroughput (last %s):\n", formatWindowDuration(m.Throughput.WindowSeconds))
	fmt.Printf("  Commits:  %s\n", formatCount(m.Throughput.CommitsPerHour))
	fmt.Printf("  Workers:  %d launched\n", m.Throughput.WorkersLaunched)

	// Cost
	fmt.Printf("\nCost:\n")
	fmt.Printf("  Total spend:    %s\n", formatMetricsCost(m.Cost.TotalSpend))
	fmt.Printf("  Cost/worker:    %s (%d workers)\n", formatMetricsCost(m.Cost.CostPerWorker), m.Cost.WorkerCount)
	fmt.Printf("  Cost/commit:    %s (%s commits)\n", formatMetricsCost(m.Cost.CostPerCommit), formatCount(m.Cost.CommitCount))

	// Resources
	fmt.Printf("\nResources:\n")
	fmt.Printf("  Load:   %.1f / %.1f / %.1f (1m/5m/15m)\n",
		m.Resources.Load.Load1, m.Resources.Load.Load5, m.Resources.Load.Load15)
	fmt.Printf("  Memory: %d MB / %d MB (%.1f%% used)\n",
		m.Resources.Memory.UsedMB, m.Resources.Memory.TotalMB, m.Resources.Memory.UsedPercent)
	for _, d := range m.Resources.Disk {
		fmt.Printf("  Disk %s: %.1f GB / %.1f GB (%.1f%% used)\n",
			d.Mount, d.UsedGB, d.TotalGB, d.UsedPercent)
	}

	// Workflow (last run)
	if m.Workflow != nil {
		fmt.Printf("\nWorkflow (%s):\n", m.Workflow.WorkflowName)
		fmt.Printf("  Tasks:     %d total, %d completed, %d failed, %d skipped\n",
			m.Workflow.TasksTotal, m.Workflow.TasksCompleted, m.Workflow.TasksFailed, m.Workflow.TasksSkipped)
		fmt.Printf("  Duration:  %.2fs\n", m.Workflow.DurationSecs)
		fmt.Printf("  DAG depth: %d\n", m.Workflow.DAGDepth)
	} else {
		fmt.Printf("\nWorkflow: no executions recorded\n")
	}

	// Batch
	if m.Batch != nil {
		fmt.Printf("\nBatch:\n")
		fmt.Printf("  Sessions: %d spawned, %d completed, %d failed\n",
			m.Batch.SessionsSpawned, m.Batch.SessionsCompleted, m.Batch.SessionsFailed)
		if m.Batch.MergeDurationSecs > 0 {
			fmt.Printf("  Last merge: %.2fs\n", m.Batch.MergeDurationSecs)
		} else {
			fmt.Printf("  Last merge: N/A\n")
		}
	}

	// Alerts
	if len(m.Alerts) > 0 {
		fmt.Printf("\nAlerts (%d):\n", len(m.Alerts))
		for _, a := range m.Alerts {
			icon := "!"
			if a.Level == "critical" {
				icon = "X"
			}
			fmt.Printf("  [%s] %s: %s\n", icon, strings.ToUpper(a.Type), a.Message)
		}
	} else {
		fmt.Printf("\nAlerts: none\n")
	}
}

func formatWindowDuration(seconds int) string {
	d := time.Duration(seconds) * time.Second
	if d >= time.Hour {
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Minutes())) + "m"
}

func formatCount(n int) string {
	if n < 0 {
		return "N/A"
	}
	return strconv.Itoa(n)
}

func formatMetricsCost(c float64) string {
	if c <= 0 {
		return "N/A"
	}
	return fmt.Sprintf("$%.2f", c)
}

// runMetricsCheck handles the --check mode for health monitoring.
// Returns exit code 0 if healthy (no alerts), 1 if there are alerts.
// Output format is controlled by --alert-format flag (text or json).
func runMetricsCheck(result *ops.MetricsResult) error {
	if len(result.Alerts) == 0 {
		// Healthy - output nothing and exit 0
		return nil
	}

	// There are alerts - output them and exit 1
	switch metricsAlertFormat {
	case "json":
		outputAlertsJSON(result.Alerts)
	case "text":
		outputAlertsText(result.Alerts)
	default:
		return fmt.Errorf("invalid alert format: %s (use 'text' or 'json')", metricsAlertFormat)
	}

	// Exit with error code to signal alerts
	os.Exit(1)
	return nil // Unreachable, but needed for type signature
}

// outputAlertsJSON outputs alerts in compact JSON format.
func outputAlertsJSON(alerts []ops.Alert) {
	data, _ := json.Marshal(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
	fmt.Println(string(data))
}

// outputAlertsText outputs alerts in compact text format.
func outputAlertsText(alerts []ops.Alert) {
	for _, a := range alerts {
		icon := "!"
		if a.Level == "critical" {
			icon = "X"
		}
		fmt.Printf("[%s] %s: %s (%s)\n", icon, strings.ToUpper(a.Type), a.Message, a.Value)
	}
}
