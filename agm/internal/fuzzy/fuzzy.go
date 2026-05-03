// Package fuzzy provides fuzzy functionality.
package fuzzy

import (
	"sort"
	"strings"

	"github.com/agnivade/levenshtein"
)

// Match represents a fuzzy match result
type Match struct {
	Name       string
	Similarity float64
	Distance   int
}

// FindSimilar finds similar strings to the input using Levenshtein distance
// Returns matches with similarity >= threshold, sorted by similarity (best first)
// Limited to top 5 results
func FindSimilar(input string, candidates []string, threshold float64) []Match {
	if len(candidates) == 0 {
		return []Match{}
	}

	// Normalize input for case-insensitive matching
	inputLower := strings.ToLower(input)
	var matches []Match

	for _, candidate := range candidates {
		// Case-insensitive comparison
		candidateLower := strings.ToLower(candidate)

		// Compute Levenshtein distance
		distance := levenshtein.ComputeDistance(inputLower, candidateLower)

		// Calculate similarity score (1.0 = perfect match, 0.0 = completely different)
		maxLen := max(len(inputLower), len(candidateLower))
		if maxLen == 0 {
			continue
		}

		similarity := 1.0 - float64(distance)/float64(maxLen)

		// Only include matches above threshold
		if similarity >= threshold {
			matches = append(matches, Match{
				Name:       candidate, // Return original case
				Similarity: similarity,
				Distance:   distance,
			})
		}
	}

	// Sort by similarity (best first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})

	// Return top 5 results
	if len(matches) > 5 {
		return matches[:5]
	}

	return matches
}

