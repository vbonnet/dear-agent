package errormemory

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// PruneExpired removes records whose TTLExpiry is before now.
func PruneExpired(records []ErrorRecord) []ErrorRecord {
	now := time.Now()
	var result []ErrorRecord
	for _, rec := range records {
		if rec.TTLExpiry.After(now) {
			result = append(result, rec)
		}
	}
	return result
}

// TopN returns the top N records sorted by count * recency_factor (descending).
// recency_factor = 1.0 / (days_since_last_seen + 1)
func TopN(records []ErrorRecord, n int) []ErrorRecord {
	if len(records) == 0 {
		return nil
	}

	now := time.Now()

	type scored struct {
		record ErrorRecord
		score  float64
	}

	scoredRecords := make([]scored, len(records))
	for i, rec := range records {
		daysSince := now.Sub(rec.LastSeen).Hours() / 24.0
		if daysSince < 0 {
			daysSince = 0
		}
		recencyFactor := 1.0 / (daysSince + 1.0)
		scoredRecords[i] = scored{
			record: rec,
			score:  float64(rec.Count) * recencyFactor,
		}
	}

	sort.Slice(scoredRecords, func(i, j int) bool {
		return scoredRecords[i].score > scoredRecords[j].score
	})

	if n > len(scoredRecords) {
		n = len(scoredRecords)
	}

	result := make([]ErrorRecord, n)
	for i := 0; i < n; i++ {
		result[i] = scoredRecords[i].record
	}
	return result
}

// DBStats returns a summary of the error database.
type DBStats struct {
	TotalRecords   int
	ExpiredRecords int
	UniquePatterns int
	TotalCount     int
	TopPattern     string
	TopCount       int
	OldestRecord   time.Time
	NewestRecord   time.Time
}

// Stats returns a summary of the error database.
func Stats(records []ErrorRecord) DBStats {
	stats := DBStats{}
	if len(records) == 0 {
		return stats
	}

	now := time.Now()
	patterns := make(map[string]bool)

	stats.TotalRecords = len(records)
	stats.OldestRecord = records[0].FirstSeen
	stats.NewestRecord = records[0].LastSeen

	for _, rec := range records {
		patterns[rec.Pattern] = true
		stats.TotalCount += rec.Count

		if rec.Count > stats.TopCount {
			stats.TopCount = rec.Count
			stats.TopPattern = rec.Pattern
		}

		if rec.FirstSeen.Before(stats.OldestRecord) {
			stats.OldestRecord = rec.FirstSeen
		}
		if rec.LastSeen.After(stats.NewestRecord) {
			stats.NewestRecord = rec.LastSeen
		}

		if rec.TTLExpiry.Before(now) {
			stats.ExpiredRecords++
		}
	}

	stats.UniquePatterns = len(patterns)
	return stats
}

// FormatSummary formats the top N records into a human-readable summary for session injection.
// Returns the formatted text and approximate token count (words / 0.75).
func FormatSummary(records []ErrorRecord, maxEntries int) (string, int) {
	if len(records) == 0 {
		return "", 0
	}

	top := TopN(records, maxEntries)
	if len(top) == 0 {
		return "", 0
	}

	now := time.Now()
	var b strings.Builder
	b.WriteString("[error-memory] Common mistakes to avoid:\n")

	for _, rec := range top {
		age := formatAge(now.Sub(rec.LastSeen))
		fmt.Fprintf(&b, "  - Do NOT use %s -- %s (%sx, last %s ago)\n",
			rec.Pattern, rec.Remediation, formatCount(rec.Count), age)
	}

	text := b.String()
	words := len(strings.Fields(text))
	tokenCount := int(math.Ceil(float64(words) / 0.75))

	return text, tokenCount
}

// formatAge formats a duration into a human-readable age string.
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

// formatCount formats a count with comma separators.
func formatCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	s := fmt.Sprintf("%d", n)
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		// s comes from fmt.Sprintf("%d", n) — pure ASCII digits.
		result = append(result, byte(c)) //nolint:gosec // ASCII-only source
	}
	return string(result)
}
