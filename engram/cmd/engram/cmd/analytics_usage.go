package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/internal/telemetry/usage"
	"github.com/vbonnet/dear-agent/pkg/table"
)

var (
	usageSince  string
	usageFormat string
	usageTop    int
)

var analyticsUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show CLI usage statistics",
	Long: `Display usage statistics from command execution logs.

Shows command frequency, average duration, and usage patterns to help identify
which commands are used most frequently and may benefit from optimization.

EXAMPLES
  # Show all usage stats
  $ engram analytics usage

  # Show top 10 commands
  $ engram analytics usage --top 10

  # Show usage since specific date
  $ engram analytics usage --since 2026-02-01

  # Export to JSON
  $ engram analytics usage --format json

DATA SOURCE
  Reads from ~/.engram/usage.jsonl (populated by CLI usage tracking)`,
	RunE: runAnalyticsUsage,
}

func init() {
	analyticsCmd.AddCommand(analyticsUsageCmd)

	analyticsUsageCmd.Flags().StringVar(&usageSince, "since", "", "Show usage since date (YYYY-MM-DD)")
	analyticsUsageCmd.Flags().StringVar(&usageFormat, "format", "table", "Output format (table/json/csv)")
	analyticsUsageCmd.Flags().IntVar(&usageTop, "top", 20, "Show top N commands")
}

type commandStats struct {
	Command      string
	Count        int
	TotalMs      int64
	AvgMs        int64
	FirstUsed    time.Time
	LastUsed     time.Time
	SuccessCount int
	FailureCount int
}

//nolint:gocyclo // reason: linear CLI flag handling and report rendering
func runAnalyticsUsage(cmd *cobra.Command, args []string) error {
	// Find usage file
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	usageFile := fmt.Sprintf("%s/.engram/usage.jsonl", home)

	// Check if file exists
	if _, err := os.Stat(usageFile); os.IsNotExist(err) {
		fmt.Println("No usage data found.")
		fmt.Printf("Usage tracking writes to: %s\n", usageFile)
		fmt.Println("\nUsage data is collected automatically when you run engram commands.")
		return nil
	}

	// Read and parse events
	events, err := readUsageEvents(usageFile)
	if err != nil {
		return fmt.Errorf("failed to read usage data: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No usage events found.")
		return nil
	}

	// Filter by date if --since specified
	if usageSince != "" {
		sinceTime, err := time.Parse("2006-01-02", usageSince)
		if err != nil {
			return fmt.Errorf("invalid --since date format (use YYYY-MM-DD): %w", err)
		}

		filtered := []usage.Event{}
		for _, e := range events {
			if e.Timestamp.After(sinceTime) || e.Timestamp.Equal(sinceTime) {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	if len(events) == 0 {
		fmt.Printf("No usage events found since %s\n", usageSince)
		return nil
	}

	// Aggregate stats by command
	stats := aggregateStats(events)

	// Sort by count (most used first)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	// Limit to top N
	if usageTop > 0 && len(stats) > usageTop {
		stats = stats[:usageTop]
	}

	// Output in requested format
	switch usageFormat {
	case "table":
		return outputUsageTable(stats)
	case "json":
		return outputUsageJSON(stats)
	case "csv":
		return outputUsageCSV(stats)
	default:
		return fmt.Errorf("unknown format: %s (use table/json/csv)", usageFormat)
	}
}

func readUsageEvents(path string) ([]usage.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	events := []usage.Event{}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event usage.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Skip invalid lines (with warning)
			fmt.Fprintf(os.Stderr, "Warning: Skipping invalid line %d: %v\n", i+1, err)
			continue
		}

		events = append(events, event)
	}

	return events, nil
}

func aggregateStats(events []usage.Event) []commandStats {
	statsMap := make(map[string]*commandStats)

	for _, e := range events {
		cmd := e.Command
		if cmd == "" {
			cmd = "unknown"
		}

		if _, exists := statsMap[cmd]; !exists {
			statsMap[cmd] = &commandStats{
				Command:   cmd,
				FirstUsed: e.Timestamp,
				LastUsed:  e.Timestamp,
			}
		}

		s := statsMap[cmd]
		s.Count++
		s.TotalMs += e.Duration

		if e.Timestamp.Before(s.FirstUsed) {
			s.FirstUsed = e.Timestamp
		}
		if e.Timestamp.After(s.LastUsed) {
			s.LastUsed = e.Timestamp
		}

		if e.Success {
			s.SuccessCount++
		} else {
			s.FailureCount++
		}
	}

	// Calculate averages
	stats := []commandStats{}
	for _, s := range statsMap {
		if s.Count > 0 {
			s.AvgMs = s.TotalMs / int64(s.Count)
		}
		stats = append(stats, *s)
	}

	return stats
}

func outputUsageTable(stats []commandStats) error {
	tbl := table.New([]string{"COMMAND", "COUNT", "AVG TIME", "SUCCESS RATE", "LAST USED"})

	for _, s := range stats {
		avgTime := formatDuration(s.AvgMs)
		successRate := float64(s.SuccessCount) / float64(s.Count) * 100
		lastUsed := s.LastUsed.Format("2006-01-02")

		tbl.AddRow(
			s.Command,
			fmt.Sprintf("%d", s.Count),
			avgTime,
			fmt.Sprintf("%.1f%%", successRate),
			lastUsed,
		)
	}

	if err := tbl.Print(); err != nil {
		return err
	}

	totalEvents := 0
	for _, s := range stats {
		totalEvents += s.Count
	}

	fmt.Printf("\nTotal commands: %d\n", len(stats))
	fmt.Printf("Total executions: %d\n", totalEvents)

	return nil
}

func outputUsageJSON(stats []commandStats) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(stats)
}

func outputUsageCSV(stats []commandStats) error {
	fmt.Println("command,count,avg_ms,success_rate,first_used,last_used")

	for _, s := range stats {
		successRate := float64(s.SuccessCount) / float64(s.Count) * 100

		fmt.Printf("%s,%d,%d,%.1f,%s,%s\n",
			s.Command,
			s.Count,
			s.AvgMs,
			successRate,
			s.FirstUsed.Format(time.RFC3339),
			s.LastUsed.Format(time.RFC3339),
		)
	}

	return nil
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}

	seconds := float64(ms) / 1000.0
	if seconds < 60 {
		return fmt.Sprintf("%.1fs", seconds)
	}

	minutes := int(seconds / 60)
	remainingSeconds := int(seconds) % 60
	return fmt.Sprintf("%dm%ds", minutes, remainingSeconds)
}
