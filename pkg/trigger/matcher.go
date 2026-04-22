package trigger

import (
	"sort"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// TriggerEvent is a normalized event envelope for trigger evaluation.
type TriggerEvent struct {
	Type      string                 // "phase.started", "task.assigned", etc.
	Data      map[string]interface{} // Event-specific data
	ProjectID string                 // For scope filtering
	SessionID string
}

// MatchResult represents a matched trigger with its engram path.
type MatchResult struct {
	EngramPath string
	Priority   int
	Trigger    engram.TriggerSpec
}

// TriggerMatcher evaluates triggers against events.
type TriggerMatcher struct {
	registry *TriggerRegistry
}

// NewTriggerMatcher creates a new TriggerMatcher backed by the given registry.
func NewTriggerMatcher(registry *TriggerRegistry) *TriggerMatcher {
	return &TriggerMatcher{
		registry: registry,
	}
}

// Match evaluates all triggers for the given event type.
// Returns matched engram paths sorted by priority (highest first).
func (m *TriggerMatcher) Match(event TriggerEvent) []MatchResult {
	entries := m.registry.Lookup(event.Type)
	if len(entries) == 0 {
		return nil
	}

	var results []MatchResult

	for _, entry := range entries {
		if matchAllPredicates(entry.Trigger.Match, event.Data) {
			results = append(results, MatchResult{
				EngramPath: entry.EngramPath,
				Priority:   entry.Trigger.Priority,
				Trigger:    entry.Trigger,
			})
		}
	}

	// Sort by priority descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Priority > results[j].Priority
	})

	return results
}

// matchAllPredicates checks if all match conditions are satisfied by the event data.
func matchAllPredicates(match map[string]interface{}, data map[string]interface{}) bool {
	// Empty or nil match means always match.
	if len(match) == 0 {
		return true
	}

	for key, matchValue := range match {
		eventValue, ok := data[key]
		if !ok {
			return false
		}
		if !matchPredicate(matchValue, eventValue) {
			return false
		}
	}

	return true
}

// matchPredicate checks if a single match condition is satisfied.
// Supports: exact string match, array "any-of" match.
func matchPredicate(matchValue interface{}, eventValue interface{}) bool {
	switch mv := matchValue.(type) {
	case string:
		// Exact string match.
		ev, ok := eventValue.(string)
		if !ok {
			return false
		}
		return mv == ev

	case []interface{}:
		// Array "any-of" match: event value must equal any element in the array.
		for _, candidate := range mv {
			if candidateStr, ok := candidate.(string); ok {
				if evStr, ok := eventValue.(string); ok && candidateStr == evStr {
					return true
				}
			}
			// Also support non-string equality.
			if candidate == eventValue {
				return true
			}
		}
		return false

	default:
		// Fallback: direct equality.
		return matchValue == eventValue
	}
}
