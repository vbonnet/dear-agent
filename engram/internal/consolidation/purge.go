package consolidation

import (
	"fmt"
	"log/slog"
)

// PurgeStats reports what PurgeContext removed or redacted.
type PurgeStats struct {
	// ToolResultsStripped is the count of old tool results removed.
	ToolResultsStripped int

	// PIIRedactions counts redactions by pattern name (e.g., "api_key": 3).
	PIIRedactions map[string]int

	// EventsPreserved is the number of structural events kept.
	EventsPreserved int

	// EventsTotal is the total event count before purging.
	EventsTotal int
}

// String returns a human-readable summary of purge statistics.
func (s PurgeStats) String() string {
	total := 0
	for _, v := range s.PIIRedactions {
		total += v
	}
	return fmt.Sprintf("purge: stripped=%d pii_redactions=%d preserved=%d/%d",
		s.ToolResultsStripped, total, s.EventsPreserved, s.EventsTotal)
}

// toolResultEventType identifies tool result events eligible for stripping.
const toolResultEventType = "tool_result"

// maxPreservedEvents is the threshold beyond which old tool results are stripped.
const maxPreservedEvents = 50

// PurgeContext sanitizes a WorkingContext for safe task switching or sleep cycle resume.
//
// It performs two operations:
//  1. Strips old tool results: events beyond the most recent maxPreservedEvents
//     that are of type "tool_result" are removed. Structural events (phase changes,
//     task updates, memory stores) are always preserved.
//  2. Redacts PII/secrets: applies DefaultPurgePatterns to all string content
//     in remaining events, replacing matches with safe placeholders.
//
// The original WorkingContext is modified in place. Returns statistics about
// what was purged.
//
// Example:
//
//	stats := consolidation.PurgeContext(workingCtx, slog.Default())
//	slog.Info("context purged", "stats", stats)
func PurgeContext(wc *WorkingContext, logger *slog.Logger) PurgeStats {
	stats := PurgeStats{
		PIIRedactions: make(map[string]int),
		EventsTotal:   len(wc.RecentHistory),
	}

	// Phase 1: Strip old tool results beyond threshold.
	wc.RecentHistory = stripOldToolResults(wc.RecentHistory, &stats)

	// Phase 2: Redact PII/secrets from remaining content.
	patterns := DefaultPurgePatterns()
	redactEvents(wc.RecentHistory, patterns, &stats)
	redactMemories(wc.RelevantMemory, patterns, &stats)
	redactMemories(wc.PinnedItems, patterns, &stats)

	stats.EventsPreserved = len(wc.RecentHistory)

	logger.Info("context purged",
		"tool_results_stripped", stats.ToolResultsStripped,
		"pii_redactions", stats.PIIRedactions,
		"events_preserved", stats.EventsPreserved,
		"events_total", stats.EventsTotal,
	)

	return stats
}

// stripOldToolResults removes tool_result events from the oldest portion of history,
// keeping all events within the most recent maxPreservedEvents window and all
// non-tool-result (structural) events regardless of position.
func stripOldToolResults(events []SessionEvent, stats *PurgeStats) []SessionEvent {
	if len(events) <= maxPreservedEvents {
		return events
	}

	cutoff := len(events) - maxPreservedEvents
	result := make([]SessionEvent, 0, len(events))

	for i, event := range events {
		if i < cutoff && event.Type == toolResultEventType {
			stats.ToolResultsStripped++
			continue
		}
		result = append(result, event)
	}

	return result
}

// redactEvents applies PII patterns to event Data fields.
func redactEvents(events []SessionEvent, patterns []PurgePattern, stats *PurgeStats) {
	for i := range events {
		if s, ok := events[i].Data.(string); ok {
			events[i].Data = redactString(s, patterns, stats)
		}
	}
}

// redactMemories applies PII patterns to memory Content fields.
func redactMemories(memories []Memory, patterns []PurgePattern, stats *PurgeStats) {
	for i := range memories {
		if s, ok := memories[i].Content.(string); ok {
			memories[i].Content = redactString(s, patterns, stats)
		}
	}
}

// redactString applies all patterns to a string, returning the sanitized version.
func redactString(s string, patterns []PurgePattern, stats *PurgeStats) string {
	for _, p := range patterns {
		matches := p.Pattern.FindAllString(s, -1)
		if len(matches) > 0 {
			stats.PIIRedactions[p.Name] += len(matches)
			s = p.Pattern.ReplaceAllString(s, p.Replacement)
		}
	}
	return s
}
