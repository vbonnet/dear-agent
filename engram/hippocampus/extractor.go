package hippocampus

import (
	"regexp"
	"strings"
)

// extractDecisions scans session history for key decisions.
//
// Phase 5 V1: Simple pattern matching for markdown decision markers.
// Phase 5 V2+: Enhanced with LLM-based semantic extraction.
func (h *Hippocampus) extractDecisions(history string) ([]Decision, error) {
	decisions := []Decision{}

	// Pattern: "## Decision: <title>" or "**Decision**: <title>"
	// More flexible: match with or without header markers
	decisionPattern := regexp.MustCompile(`(?mi)^[#\s]*\*{0,2}Decision\*{0,2}:?\s*(.+)$`)
	matches := decisionPattern.FindAllStringSubmatch(history, -1)

	for _, match := range matches {
		if len(match) > 1 {
			title := strings.TrimSpace(match[1])
			decisions = append(decisions, Decision{
				Title:     title,
				Rationale: "", // Extracted in V2
				Impact:    "", // Extracted in V2
			})
		}
	}

	return decisions, nil
}

// extractOutcomes scans for concrete results achieved.
func (h *Hippocampus) extractOutcomes(history string) ([]Outcome, error) {
	outcomes := []Outcome{}

	// Pattern: "Completed:", "Implemented:", "Created:", etc.
	// "- **Outcome**: <description>"

	outcomePattern := regexp.MustCompile(`(?m)^[-*]\s*\*?\*?(Completed|Implemented|Created|Achieved)\*?\*?:?\s*(.+)$`)
	matches := outcomePattern.FindAllStringSubmatch(history, -1)

	for _, match := range matches {
		if len(match) > 2 {
			description := strings.TrimSpace(match[2])
			outcomes = append(outcomes, Outcome{
				Description: description,
				Evidence:    "", // Extracted in V2
			})
		}
	}

	return outcomes, nil
}

// extractTechnicalLearnings scans for technical insights.
func (h *Hippocampus) extractTechnicalLearnings(history string) ([]Learning, error) {
	learnings := []Learning{}

	// Pattern: "Learned:", "Discovered:", "Found that", etc.
	// Also match without list markers
	techPattern := regexp.MustCompile(`(?mi)(Learned|Discovered|Found|Technical):\s*(.+)$`)
	matches := techPattern.FindAllStringSubmatch(history, -1)

	for _, match := range matches {
		if len(match) > 2 {
			learning := strings.TrimSpace(match[2])
			learnings = append(learnings, Learning{
				Learning:    learning,
				Context:     "", // Extracted in V2
				Application: "", // Extracted in V2
			})
		}
	}

	return learnings, nil
}

// extractProcessLearnings scans for process insights.
func (h *Hippocampus) extractProcessLearnings(history string) ([]Learning, error) {
	learnings := []Learning{}

	// Pattern: "Faster than estimated", "Should have", "Next time", etc.
	processPattern := regexp.MustCompile(`(?m)(Faster than estimated|Should have|Next time|Process|Efficiency):\s*(.+)$`)
	matches := processPattern.FindAllStringSubmatch(history, -1)

	for _, match := range matches {
		if len(match) > 2 {
			learning := strings.TrimSpace(match[2])
			learnings = append(learnings, Learning{
				Learning:    learning,
				Context:     "", // Extracted in V2
				Application: "", // Extracted in V2
			})
		}
	}

	return learnings, nil
}

// extractActivePlan scans for Wayfinder Plan state.
func (h *Hippocampus) extractActivePlan(history string) (*Plan, error) {
	// Pattern: "Current Phase:", "Next Steps:", etc.
	// Match without header marker requirement
	phasePattern := regexp.MustCompile(`(?mi)Current Phase:?\s*(.+)$`)
	matches := phasePattern.FindStringSubmatch(history)

	if len(matches) > 1 {
		return &Plan{
			Status:       "active",
			CurrentPhase: strings.TrimSpace(matches[1]),
			NextSteps:    []string{}, // Extracted in V2
		}, nil
	}

	// No active plan found
	return nil, nil
}

// extractEngrams scans for loaded engrams mentioned in history.
func (h *Hippocampus) extractEngrams(history string) ([]string, error) {
	engrams := []string{}

	// Pattern: "*.ai.md" references
	engramPattern := regexp.MustCompile(`([a-z-]+\.ai\.md)`)
	matches := engramPattern.FindAllStringSubmatch(history, -1)

	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			engram := match[1]
			if !seen[engram] {
				engrams = append(engrams, engram)
				seen[engram] = true
			}
		}
	}

	return engrams, nil
}
