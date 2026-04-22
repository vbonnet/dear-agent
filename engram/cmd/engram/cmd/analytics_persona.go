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

var personaCmd = &cobra.Command{
	Use:   "persona",
	Short: "Show persona effectiveness metrics",
	Long: `Analyze persona review effectiveness from telemetry logs.

Shows:
  - Issues caught by persona (by severity)
  - False positive rate (from S11 retrospective validation)
  - Time overhead per review
  - Persona comparison (which personas find more issues)

EXAMPLES
  # Show all persona reviews
  $ engram analytics persona

  # Show specific persona
  $ engram analytics persona --id reuse-advocate

  # Show last week's reviews
  $ engram analytics persona --last-week

  # Export to CSV
  $ engram analytics persona --format csv > persona.csv

TARGET
  Classification accuracy ≥90% (D4 acceptance criteria)`,
	RunE: runPersonaAnalytics,
}

var (
	personaID        string
	personaLastWeek  bool
	personaLastMonth bool
	personaFormat    string
)

func init() {
	analyticsCmd.AddCommand(personaCmd)

	personaCmd.Flags().StringVar(&personaID, "id", "", "Filter by persona ID")
	personaCmd.Flags().BoolVar(&personaLastWeek, "last-week", false, "Show last 7 days")
	personaCmd.Flags().BoolVar(&personaLastMonth, "last-month", false, "Show last 30 days")
	personaCmd.Flags().StringVar(&personaFormat, "format", "text", "Output format: text, json, csv")
}

func runPersonaAnalytics(cmd *cobra.Command, args []string) error {
	// Find persona effectiveness logs
	logDir := filepath.Join(os.Getenv("HOME"), ".engram", "logs")
	personaFile := filepath.Join(logDir, "persona-effectiveness.jsonl")

	// Check if persona file exists
	if _, err := os.Stat(personaFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No persona effectiveness data found at %s\n", personaFile)
		fmt.Fprintf(os.Stderr, "Run a session with persona reviews to generate effectiveness data.\n")
		return nil
	}

	// Parse persona events
	reviews, err := parsePersonaReviews(personaFile)
	if err != nil {
		return fmt.Errorf("failed to parse persona logs: %w", err)
	}

	// Apply filters
	reviews = filterPersonaReviews(reviews, personaID, personaLastWeek, personaLastMonth)

	if len(reviews) == 0 {
		fmt.Println("No persona review data matches the specified filters.")
		return nil
	}

	// Output in requested format
	switch personaFormat {
	case "json":
		return outputPersonaJSON(reviews)
	case "csv":
		return outputPersonaCSV(reviews)
	default:
		return outputPersonaText(reviews)
	}
}

type personaReview struct {
	Timestamp      time.Time
	SessionID      string
	PersonaID      string
	IssuesFound    int
	Severity       string
	TimeOverheadMs int
	FalsePositives int
	Classification map[string]interface{}
}

func parsePersonaReviews(filename string) ([]personaReview, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reviews []personaReview
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

		review := personaReview{
			Timestamp:      timestamp,
			SessionID:      getStringField(data, "session_id"),
			PersonaID:      getStringField(data, "persona_id"),
			IssuesFound:    getIntField(data, "issues_found"),
			Severity:       getStringField(data, "severity"),
			TimeOverheadMs: getIntField(data, "time_overhead_ms"),
			FalsePositives: getIntField(data, "false_positives"),
			Classification: getMapField(data, "classification_metadata"),
		}

		reviews = append(reviews, review)
	}

	return reviews, scanner.Err()
}

func filterPersonaReviews(reviews []personaReview, personaID string, lastWeek, lastMonth bool) []personaReview {
	var filtered []personaReview
	now := time.Now()

	for _, review := range reviews {
		// Filter by persona ID
		if personaID != "" && review.PersonaID != personaID {
			continue
		}

		// Filter by time range
		if lastWeek && now.Sub(review.Timestamp) > 7*24*time.Hour {
			continue
		}
		if lastMonth && now.Sub(review.Timestamp) > 30*24*time.Hour {
			continue
		}

		filtered = append(filtered, review)
	}

	return filtered
}

func outputPersonaText(reviews []personaReview) error {
	fmt.Printf("Persona Effectiveness Report (%d reviews)\n", len(reviews))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Group by persona
	personaStats := make(map[string][]personaReview)
	for _, review := range reviews {
		personaStats[review.PersonaID] = append(personaStats[review.PersonaID], review)
	}

	for personaID, personaReviews := range personaStats {
		fmt.Printf("Persona: %s\n", personaID)
		fmt.Printf("  Reviews: %d\n", len(personaReviews))

		totalIssues := 0
		totalFalsePositives := 0
		totalOverhead := 0

		for _, review := range personaReviews {
			totalIssues += review.IssuesFound
			totalFalsePositives += review.FalsePositives
			totalOverhead += review.TimeOverheadMs
		}

		avgOverhead := 0
		if len(personaReviews) > 0 {
			avgOverhead = totalOverhead / len(personaReviews)
		}

		accuracy := 0.0
		if totalIssues > 0 {
			accuracy = float64(totalIssues-totalFalsePositives) / float64(totalIssues)
		}

		fmt.Printf("  Issues found: %d\n", totalIssues)
		fmt.Printf("  False positives: %d\n", totalFalsePositives)
		fmt.Printf("  Accuracy: %.1f%%\n", accuracy*100)
		fmt.Printf("  Avg overhead: %dms\n", avgOverhead)

		if accuracy >= 0.90 {
			fmt.Println("  ✅ Meets ≥90% accuracy target")
		} else {
			fmt.Printf("  ⚠️  Below 90%% accuracy target (%.1f%% vs 90%%)\n", accuracy*100)
		}
		fmt.Println()
	}

	// Overall summary
	if len(reviews) > 0 {
		totalIssues := 0
		totalFalsePositives := 0
		for _, review := range reviews {
			totalIssues += review.IssuesFound
			totalFalsePositives += review.FalsePositives
		}

		overallAccuracy := 0.0
		if totalIssues > 0 {
			overallAccuracy = float64(totalIssues-totalFalsePositives) / float64(totalIssues)
		}

		fmt.Println("Overall Summary:")
		fmt.Printf("  Total reviews: %d\n", len(reviews))
		fmt.Printf("  Total issues: %d\n", totalIssues)
		fmt.Printf("  Overall accuracy: %.1f%%\n", overallAccuracy*100)
	}

	return nil
}

func outputPersonaJSON(reviews []personaReview) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(reviews)
}

func outputPersonaCSV(reviews []personaReview) error {
	// CSV header
	fmt.Println("timestamp,session_id,persona_id,issues_found,severity,time_overhead_ms,false_positives")

	for _, review := range reviews {
		fmt.Printf("%s,%s,%s,%d,%s,%d,%d\n",
			review.Timestamp.Format(time.RFC3339),
			review.SessionID,
			review.PersonaID,
			review.IssuesFound,
			review.Severity,
			review.TimeOverheadMs,
			review.FalsePositives,
		)
	}

	return nil
}
