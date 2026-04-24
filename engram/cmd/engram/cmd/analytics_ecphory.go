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

var ecphoryCmd = &cobra.Command{
	Use:   "ecphory",
	Short: "Show ecphory correctness metrics",
	Long: `Analyze ecphory retrieval correctness from audit logs.

Shows:
  - Correctness scores (appropriate retrievals / total retrievals)
  - Session context (language, framework, task type)
  - Inappropriate retrieval detection
  - Trend analysis

EXAMPLES
  # Show all ecphory audits
  $ engram analytics ecphory

  # Show last week's audits
  $ engram analytics ecphory --last-week

  # Show specific session
  $ engram analytics ecphory --session-id abc123

  # Export to CSV
  $ engram analytics ecphory --format csv > ecphory.csv

TARGET
  Correctness score ≥99% (D4 acceptance criteria)`,
	RunE: runEcphoryAnalytics,
}

var (
	ecphorySessionID string
	ecphoryLastWeek  bool
	ecphoryLastMonth bool
	ecphoryFormat    string
)

func init() {
	analyticsCmd.AddCommand(ecphoryCmd)

	ecphoryCmd.Flags().StringVar(&ecphorySessionID, "session-id", "", "Filter by session ID")
	ecphoryCmd.Flags().BoolVar(&ecphoryLastWeek, "last-week", false, "Show last 7 days")
	ecphoryCmd.Flags().BoolVar(&ecphoryLastMonth, "last-month", false, "Show last 30 days")
	ecphoryCmd.Flags().StringVar(&ecphoryFormat, "format", "text", "Output format: text, json, csv")
}

func runEcphoryAnalytics(cmd *cobra.Command, args []string) error {
	// Find ecphory audit logs
	logDir := filepath.Join(os.Getenv("HOME"), ".engram", "logs")
	auditFile := filepath.Join(logDir, "ecphory-audit.jsonl")

	// Check if audit file exists
	if _, err := os.Stat(auditFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No ecphory audit data found at %s\n", auditFile)
		fmt.Fprintf(os.Stderr, "Run a session with telemetry enabled to generate audit data.\n")
		return nil
	}

	// Parse audit events
	audits, err := parseEcphoryAudits(auditFile)
	if err != nil {
		return fmt.Errorf("failed to parse audit logs: %w", err)
	}

	// Apply filters
	audits = filterEcphoryAudits(audits, ecphorySessionID, ecphoryLastWeek, ecphoryLastMonth)

	if len(audits) == 0 {
		fmt.Println("No audit data matches the specified filters.")
		return nil
	}

	// Output in requested format
	switch ecphoryFormat {
	case "json":
		return outputEcphoryJSON(audits)
	case "csv":
		return outputEcphoryCSV(audits)
	default:
		return outputEcphoryText(audits)
	}
}

type ecphoryAudit struct {
	Timestamp          time.Time
	SessionID          string
	TotalRetrievals    int
	AppropriateCount   int
	InappropriateCount int
	Inconclusive       int
	CorrectnessScore   float64
	Context            map[string]interface{}
}

func parseEcphoryAudits(filename string) ([]ecphoryAudit, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var audits []ecphoryAudit
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

		audit := ecphoryAudit{
			Timestamp:          timestamp,
			SessionID:          getStringField(data, "session_id"),
			TotalRetrievals:    getIntField(data, "total_retrievals"),
			AppropriateCount:   getIntField(data, "appropriate_count"),
			InappropriateCount: getIntField(data, "inappropriate_count"),
			Inconclusive:       getIntField(data, "inconclusive"),
			CorrectnessScore:   getFloatField(data, "correctness_score"),
			Context:            getMapField(data, "context"),
		}

		audits = append(audits, audit)
	}

	return audits, scanner.Err()
}

func filterEcphoryAudits(audits []ecphoryAudit, sessionID string, lastWeek, lastMonth bool) []ecphoryAudit {
	var filtered []ecphoryAudit
	now := time.Now()

	for _, audit := range audits {
		// Filter by session ID
		if sessionID != "" && audit.SessionID != sessionID {
			continue
		}

		// Filter by time range
		if lastWeek && now.Sub(audit.Timestamp) > 7*24*time.Hour {
			continue
		}
		if lastMonth && now.Sub(audit.Timestamp) > 30*24*time.Hour {
			continue
		}

		filtered = append(filtered, audit)
	}

	return filtered
}

func outputEcphoryText(audits []ecphoryAudit) error {
	fmt.Printf("Ecphory Correctness Report (%d audits)\n", len(audits))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	for i, audit := range audits {
		fmt.Printf("[%d] Session: %s\n", i+1, audit.SessionID[:12])
		fmt.Printf("    Timestamp: %s\n", audit.Timestamp.Format("2006-01-02 15:04"))
		fmt.Printf("    Correctness: %.1f%% (%d/%d appropriate)\n",
			audit.CorrectnessScore*100,
			audit.AppropriateCount,
			audit.TotalRetrievals-audit.Inconclusive)
		if audit.InappropriateCount > 0 {
			fmt.Printf("    ⚠️  Inappropriate: %d retrievals\n", audit.InappropriateCount)
		}
		if audit.Inconclusive > 0 {
			fmt.Printf("    Inconclusive: %d retrievals\n", audit.Inconclusive)
		}
		if ctx, ok := audit.Context["language"]; ok {
			fmt.Printf("    Context: %v\n", ctx)
		}
		fmt.Println()
	}

	// Summary statistics
	if len(audits) > 0 {
		avgScore := 0.0
		for _, audit := range audits {
			avgScore += audit.CorrectnessScore
		}
		avgScore /= float64(len(audits))

		fmt.Println("Summary:")
		fmt.Printf("  Average correctness: %.1f%%\n", avgScore*100)
		if avgScore >= 0.99 {
			fmt.Println("  ✅ Meets ≥99% target")
		} else {
			fmt.Printf("  ⚠️  Below 99%% target (%.1f%% vs 99%%)\n", avgScore*100)
		}
	}

	return nil
}

func outputEcphoryJSON(audits []ecphoryAudit) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(audits)
}

func outputEcphoryCSV(audits []ecphoryAudit) error {
	// CSV header
	fmt.Println("timestamp,session_id,total_retrievals,appropriate,inappropriate,inconclusive,correctness_score,language,framework")

	for _, audit := range audits {
		fmt.Printf("%s,%s,%d,%d,%d,%d,%.4f,%s,%s\n",
			audit.Timestamp.Format(time.RFC3339),
			audit.SessionID,
			audit.TotalRetrievals,
			audit.AppropriateCount,
			audit.InappropriateCount,
			audit.Inconclusive,
			audit.CorrectnessScore,
			getStringField(audit.Context, "language"),
			getStringField(audit.Context, "framework"),
		)
	}

	return nil
}

// Helper functions for extracting fields from map[string]interface{}
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getIntField(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
	}
	return 0
}

func getFloatField(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		if num, ok := val.(float64); ok {
			return num
		}
	}
	return 0.0
}

func getMapField(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if submap, ok := val.(map[string]interface{}); ok {
			return submap
		}
	}
	return make(map[string]interface{})
}
