package validator

import (
	"fmt"
	"regexp"
	"strings"
)

// Approach represents a design approach with required content
type Approach struct {
	Name         string
	Description  string
	HasPros      bool
	HasCons      bool
	HasTradeoffs bool
	Content      string
}

// validateLateralThinkingEnhanced validates approach quality and distinctness
func validateLateralThinkingEnhanced(content string, phaseName string, deliverablePath string) error {
	// Extract approaches from content
	approaches := extractApproaches(content)

	// Check minimum count
	if len(approaches) < 3 {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("Lateral Thinking requirement not met: found %d approach(es), need ≥3", len(approaches)),
			fmt.Sprintf("Add ≥%d more distinct approach(es) to %s before proceeding.\n\n"+
				"Each approach must include:\n"+
				"- Name and description\n"+
				"- Pros and cons (tradeoffs)\n"+
				"- Complexity, timeline, or risks\n\n"+
				"Example approaches: Microservices, Modular Monolith, Serverless, Event-Driven", 3-len(approaches), deliverablePath),
		)
	}

	// Validate each approach has required content
	for i, approach := range approaches {
		if !approach.HasTradeoffs {
			return NewValidationError(
				"complete "+phaseName,
				fmt.Sprintf("Approach %d (%s) missing tradeoff analysis", i+1, approach.Name),
				fmt.Sprintf("Add pros/cons or tradeoffs section to Approach %d.\n\n"+
					"Example:\n"+
					"**Pros:**\n"+
					"- Fast implementation\n"+
					"- Low complexity\n\n"+
					"**Cons:**\n"+
					"- Limited scalability\n"+
					"- Higher maintenance", i+1),
			)
		}
	}

	// Check distinctness (similarity between approaches)
	for i := 0; i < len(approaches); i++ {
		for j := i + 1; j < len(approaches); j++ {
			similarity := calculateSimilarity(approaches[i].Content, approaches[j].Content)
			if similarity > 0.8 {
				return NewValidationError(
					"complete "+phaseName,
					fmt.Sprintf("Approaches %d and %d are too similar (%.0f%% overlap)", i+1, j+1, similarity*100),
					fmt.Sprintf("Approaches must be DISTINCT alternatives, not variations.\n\n"+
						"Current approaches:\n"+
						"- %s\n"+
						"- %s\n\n"+
						"Ensure each approach represents a fundamentally different architecture or strategy.", approaches[i].Name, approaches[j].Name),
				)
			}
		}
	}

	return nil
}

// extractApproaches parses markdown to extract approaches
func extractApproaches(content string) []Approach {
	matches := findApproachMatches(content)
	if len(matches) == 0 {
		return []Approach{}
	}

	approaches := make([]Approach, 0, len(matches))
	for i, match := range matches {
		approach := buildApproach(content, match, i, matches)
		approaches = append(approaches, approach)
	}

	return approaches
}

// findApproachMatches finds all approach headers in the content.
func findApproachMatches(content string) [][]int {
	approachPattern := regexp.MustCompile(`(?mi)^#{2,3}\s+Approach\s+([A-Z]|[0-9]+):?\s+(.+)$`)
	return approachPattern.FindAllStringSubmatchIndex(content, -1)
}

// buildApproach constructs an Approach from a regex match.
func buildApproach(content string, match []int, index int, allMatches [][]int) Approach {
	approachName := extractApproachName(content, match)
	approachContent := extractApproachContent(content, index, allMatches)

	approach := Approach{
		Name:         approachName,
		Content:      approachContent,
		HasPros:      false,
		HasCons:      false,
		HasTradeoffs: false,
	}

	analyzeApproachContent(&approach)
	return approach
}

// extractApproachName extracts the approach name from a regex match.
func extractApproachName(content string, match []int) string {
	// match[4] = group 2 start (name), match[5] = group 2 end
	return strings.TrimSpace(content[match[4]:match[5]])
}

// extractApproachContent extracts the content between approach headers.
func extractApproachContent(content string, index int, allMatches [][]int) string {
	match := allMatches[index]
	startIdx := match[1]
	endIdx := len(content)

	if index+1 < len(allMatches) {
		endIdx = allMatches[index+1][0]
	}

	return content[startIdx:endIdx]
}

// analyzeApproachContent checks for pros/cons/tradeoffs markers.
func analyzeApproachContent(approach *Approach) {
	lowerContent := strings.ToLower(approach.Content)

	approach.HasPros = hasProsMarkers(lowerContent)
	approach.HasCons = hasConsMarkers(lowerContent)
	approach.HasTradeoffs = hasTradeoffMarkers(lowerContent, approach.HasPros, approach.HasCons)
}

// hasProsMarkers checks if content contains pros indicators.
func hasProsMarkers(lowerContent string) bool {
	return strings.Contains(lowerContent, "pros:") ||
		strings.Contains(lowerContent, "**pros") ||
		strings.Contains(lowerContent, "advantages:") ||
		strings.Contains(lowerContent, "benefits:")
}

// hasConsMarkers checks if content contains cons indicators.
func hasConsMarkers(lowerContent string) bool {
	return strings.Contains(lowerContent, "cons:") ||
		strings.Contains(lowerContent, "**cons") ||
		strings.Contains(lowerContent, "disadvantages:") ||
		strings.Contains(lowerContent, "drawbacks:") ||
		strings.Contains(lowerContent, "limitations:")
}

// hasTradeoffMarkers checks if content contains tradeoff indicators.
func hasTradeoffMarkers(lowerContent string, hasPros, hasCons bool) bool {
	return strings.Contains(lowerContent, "tradeoffs:") ||
		strings.Contains(lowerContent, "trade-offs:") ||
		(hasPros && hasCons)
}

// calculateSimilarity computes similarity ratio between two approach contents
// Returns value between 0.0 (completely different) and 1.0 (identical)
func calculateSimilarity(content1, content2 string) float64 {
	// Normalize content (lowercase, remove punctuation, split into words)
	words1 := normalizeAndSplit(content1)
	words2 := normalizeAndSplit(content2)

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0 // Both empty = identical
	}
	if len(words1) == 0 || len(words2) == 0 {
		return 0.0 // One empty = completely different
	}

	// Count overlapping words (simple Jaccard similarity)
	overlap := 0
	wordSet1 := make(map[string]bool)
	for _, word := range words1 {
		wordSet1[word] = true
	}

	for _, word := range words2 {
		if wordSet1[word] {
			overlap++
		}
	}

	// Jaccard similarity = |intersection| / |union|
	union := len(words1) + len(words2) - overlap
	if union == 0 {
		return 1.0
	}

	return float64(overlap) / float64(union)
}

// normalizeAndSplit normalizes text and splits into words
func normalizeAndSplit(text string) []string {
	// Lowercase
	text = strings.ToLower(text)

	// Remove common markdown and punctuation
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "#", "")
	text = strings.ReplaceAll(text, "-", " ")
	text = strings.ReplaceAll(text, ",", " ")
	text = strings.ReplaceAll(text, ".", " ")
	text = strings.ReplaceAll(text, ":", " ")

	// Split into words
	words := strings.Fields(text)

	// Filter out very short words (likely articles/prepositions)
	var filtered []string
	for _, word := range words {
		if len(word) >= 3 {
			filtered = append(filtered, word)
		}
	}

	return filtered
}
