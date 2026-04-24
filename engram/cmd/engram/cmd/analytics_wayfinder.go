package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var wayfinderAnalyticsCmd = &cobra.Command{
	Use:   "wayfinder",
	Short: "Show Wayfinder ROI metrics",
	Long: `Analyze Wayfinder phase transitions and ROI from telemetry logs.

Shows:
  - Phase outcomes (success, failure, partial completion)
  - Quality scores (1.0 - rework*0.2 - errors*0.1)
  - Rework and error tracking
  - Phase duration and cost estimates
  - ROI analysis across sessions

EXAMPLES
  # Show all Wayfinder transitions
  $ engram analytics wayfinder

  # Show last week's transitions
  $ engram analytics wayfinder --last-week

  # Show specific session
  $ engram analytics wayfinder --session-id abc123

  # Export to CSV
  $ engram analytics wayfinder --format csv > wayfinder.csv

TARGET
  Quality score ≥0.8 (low rework, minimal errors)`,
	RunE: runWayfinderAnalytics,
}

var (
	wayfinderSessionID string
	wayfinderLastWeek  bool
	wayfinderLastMonth bool
	wayfinderFormat    string
)

func init() {
	analyticsCmd.AddCommand(wayfinderAnalyticsCmd)

	wayfinderAnalyticsCmd.Flags().StringVar(&wayfinderSessionID, "session-id", "", "Filter by session ID")
	wayfinderAnalyticsCmd.Flags().BoolVar(&wayfinderLastWeek, "last-week", false, "Show last 7 days")
	wayfinderAnalyticsCmd.Flags().BoolVar(&wayfinderLastMonth, "last-month", false, "Show last 30 days")
	wayfinderAnalyticsCmd.Flags().StringVar(&wayfinderFormat, "format", "text", "Output format: text, json, csv")
}

func runWayfinderAnalytics(cmd *cobra.Command, args []string) error {
	// Find Wayfinder ROI logs
	logDir := filepath.Join(os.Getenv("HOME"), ".engram", "logs")
	wayfinderFile := filepath.Join(logDir, "wayfinder-roi.jsonl")

	// Check if wayfinder file exists
	if _, err := os.Stat(wayfinderFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No Wayfinder ROI data found at %s\n", wayfinderFile)
		fmt.Fprintf(os.Stderr, "Run a Wayfinder session with telemetry enabled to generate ROI data.\n")
		return nil
	}

	// Parse wayfinder events
	transitions, err := parseWayfinderTransitions(wayfinderFile)
	if err != nil {
		return fmt.Errorf("failed to parse wayfinder logs: %w", err)
	}

	// Apply filters
	transitions = filterWayfinderTransitions(transitions, wayfinderSessionID, wayfinderLastWeek, wayfinderLastMonth)

	if len(transitions) == 0 {
		fmt.Println("No Wayfinder transition data matches the specified filters.")
		return nil
	}

	// Output in requested format
	switch wayfinderFormat {
	case "json":
		return outputWayfinderJSON(transitions)
	case "csv":
		return outputWayfinderCSV(transitions)
	default:
		return outputWayfinderText(transitions)
	}
}

type wayfinderTransition struct {
	Timestamp    time.Time
	SessionID    string
	PhaseName    string
	Outcome      string
	DurationMs   int
	ErrorCount   int
	ReworkCount  int
	QualityScore float64
	Metrics      map[string]interface{}
}

func parseWayfinderTransitions(filename string) ([]wayfinderTransition, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var transitions []wayfinderTransition
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip malformed lines
			continue
		}

		// Parse timestamp
		timestamp, _ := time.Parse(time.RFC3339, entry["timestamp"].(string))

		// Extract data
		data, ok := entry["data"].(map[string]interface{})
		if !ok {
			continue
		}

		transition := wayfinderTransition{
			Timestamp:    timestamp,
			SessionID:    getStringField(data, "session_id"),
			PhaseName:    getStringField(data, "phase_name"),
			Outcome:      getStringField(data, "outcome"),
			DurationMs:   getIntField(data, "duration_ms"),
			ErrorCount:   getIntField(data, "error_count"),
			ReworkCount:  getIntField(data, "rework_count"),
			QualityScore: getFloatField(data, "quality_score"),
			Metrics:      getMapField(data, "metrics"),
		}

		transitions = append(transitions, transition)
	}

	return transitions, scanner.Err()
}

func filterWayfinderTransitions(transitions []wayfinderTransition, sessionID string, lastWeek, lastMonth bool) []wayfinderTransition {
	var filtered []wayfinderTransition
	now := time.Now()

	for _, transition := range transitions {
		// Filter by session ID
		if sessionID != "" && transition.SessionID != sessionID {
			continue
		}

		// Filter by time range
		if lastWeek && now.Sub(transition.Timestamp) > 7*24*time.Hour {
			continue
		}
		if lastMonth && now.Sub(transition.Timestamp) > 30*24*time.Hour {
			continue
		}

		filtered = append(filtered, transition)
	}

	return filtered
}

func outputWayfinderText(transitions []wayfinderTransition) error {
	fmt.Printf("Wayfinder ROI Report (%d transitions)\n", len(transitions))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Group by session
	sessionStats := make(map[string][]wayfinderTransition)
	for _, transition := range transitions {
		sessionStats[transition.SessionID] = append(sessionStats[transition.SessionID], transition)
	}

	for sessionID, sessionTransitions := range sessionStats {
		fmt.Printf("Session: %s\n", sessionID[:12])
		fmt.Printf("  Transitions: %d\n", len(sessionTransitions))

		totalDuration := 0
		totalErrors := 0
		totalRework := 0
		avgQuality := 0.0
		outcomes := make(map[string]int)

		for _, transition := range sessionTransitions {
			totalDuration += transition.DurationMs
			totalErrors += transition.ErrorCount
			totalRework += transition.ReworkCount
			avgQuality += transition.QualityScore
			outcomes[transition.Outcome]++
		}

		if len(sessionTransitions) > 0 {
			avgQuality /= float64(len(sessionTransitions))
		}

		fmt.Printf("  Total duration: %.1fm\n", float64(totalDuration)/60000.0)
		fmt.Printf("  Errors: %d | Rework phases: %d\n", totalErrors, totalRework)
		fmt.Printf("  Avg quality score: %.2f\n", avgQuality)
		fmt.Printf("  Outcomes: ")
		for outcome, count := range outcomes {
			fmt.Printf("%s=%d ", outcome, count)
		}
		fmt.Println()

		if avgQuality >= 0.8 {
			fmt.Println("  ✅ Meets ≥0.8 quality target")
		} else {
			fmt.Printf("  ⚠️  Below 0.8 quality target (%.2f vs 0.8)\n", avgQuality)
		}
		fmt.Println()
	}

	// Overall summary
	if len(transitions) > 0 {
		totalDuration := 0
		totalErrors := 0
		totalRework := 0
		avgQuality := 0.0

		for _, transition := range transitions {
			totalDuration += transition.DurationMs
			totalErrors += transition.ErrorCount
			totalRework += transition.ReworkCount
			avgQuality += transition.QualityScore
		}

		avgQuality /= float64(len(transitions))

		fmt.Println("Overall Summary:")
		fmt.Printf("  Total transitions: %d\n", len(transitions))
		fmt.Printf("  Total duration: %.1fh\n", float64(totalDuration)/3600000.0)
		fmt.Printf("  Total errors: %d\n", totalErrors)
		fmt.Printf("  Total rework: %d phases\n", totalRework)
		fmt.Printf("  Overall quality: %.2f\n", avgQuality)
	}

	return nil
}

func outputWayfinderJSON(transitions []wayfinderTransition) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(transitions)
}

func outputWayfinderCSV(transitions []wayfinderTransition) error {
	// CSV header
	fmt.Println("timestamp,session_id,phase_name,outcome,duration_ms,error_count,rework_count,quality_score")

	for _, transition := range transitions {
		fmt.Printf("%s,%s,%s,%s,%d,%d,%d,%.4f\n",
			transition.Timestamp.Format(time.RFC3339),
			transition.SessionID,
			transition.PhaseName,
			transition.Outcome,
			transition.DurationMs,
			transition.ErrorCount,
			transition.ReworkCount,
			transition.QualityScore,
		)
	}

	return nil
}
