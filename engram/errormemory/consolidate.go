package errormemory

import (
	"fmt"
	"strings"
	"time"
)

// ConsolidatedEngram represents an error pattern converted to engram format
// for integration with the ecphory retrieval system.
type ConsolidatedEngram struct {
	Title         string
	Description   string
	Tags          []string
	ErrorCategory string
	LessonLearned string
	Content       string // Full markdown content for .ai.md file
	Count         int
	LastSeen      time.Time
}

// ConsolidateToEngrams converts high-frequency error patterns from the JSONL
// database into structured engrams suitable for ecphory retrieval.
// Only patterns with count >= minCount are included.
func ConsolidateToEngrams(records []ErrorRecord, minCount int) []ConsolidatedEngram {
	var engrams []ConsolidatedEngram

	for _, rec := range records {
		if rec.Count < minCount {
			continue
		}

		engram := ConsolidatedEngram{
			Title:         fmt.Sprintf("Error Pattern: %s", rec.Pattern),
			Description:   rec.Remediation,
			Tags:          []string{"error-memory", "auto-generated", rec.ErrorCategory},
			ErrorCategory: rec.ErrorCategory,
			LessonLearned: rec.Remediation,
			Count:         rec.Count,
			LastSeen:      rec.LastSeen,
		}

		// Generate .ai.md content
		engram.Content = formatEngramContent(rec)
		engrams = append(engrams, engram)
	}

	return engrams
}

// formatEngramContent generates the markdown content for an error memory engram.
func formatEngramContent(rec ErrorRecord) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "title: \"Error Pattern: %s\"\n", rec.Pattern)
	sb.WriteString("type: reflection\n")
	fmt.Fprintf(&sb, "tags: [error-memory, auto-generated, %s]\n", rec.ErrorCategory)
	fmt.Fprintf(&sb, "error_category: %s\n", rec.ErrorCategory)
	fmt.Fprintf(&sb, "encoding_strength: %.1f\n", min(2.0, 1.0+float64(rec.Count)/1000.0))
	fmt.Fprintf(&sb, "created_at: %s\n", rec.FirstSeen.Format(time.RFC3339))
	fmt.Fprintf(&sb, "last_accessed: %s\n", rec.LastSeen.Format(time.RFC3339))
	sb.WriteString("---\n\n")

	fmt.Fprintf(&sb, "# Error Pattern: %s\n\n", rec.Pattern)
	sb.WriteString("## Problem\n\n")
	fmt.Fprintf(&sb, "Agents frequently attempt commands matching the \"%s\" pattern, which are blocked by the pretool-bash-blocker hook.\n\n", rec.Pattern)
	fmt.Fprintf(&sb, "**Occurrences**: %d times\n", rec.Count)
	fmt.Fprintf(&sb, "**Example command**: `%s`\n\n", rec.CommandSample)
	sb.WriteString("## Solution\n\n")
	fmt.Fprintf(&sb, "%s\n\n", rec.Remediation)
	sb.WriteString("## Lesson Learned\n\n")
	fmt.Fprintf(&sb, "%s\n", rec.Remediation)

	return sb.String()
}
