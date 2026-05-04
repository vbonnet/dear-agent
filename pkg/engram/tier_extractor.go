package engram

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TierLevel represents tier hierarchy
type TierLevel int

// Tier levels, ordered from most condensed to most detailed.
const (
	Tier0 TierLevel = iota // Summary (50-150 tokens)
	Tier1                  // Overview (150-500 tokens)
	Tier2                  // Full content (500-2000 tokens)
)

var (
	// tierMarkerRegex matches tier markers: > [!T0], > [!T1], > [!T2]
	// Uses multiline mode so ^ matches start of line, not just start of string
	tierMarkerRegex = regexp.MustCompile(`(?m)^>\s*\[!T(\d)\]`)
)

// ExtractTier extracts content for specified tier level
// Extraction is cumulative: T1 includes T0, T2 includes T1+T0
func ExtractTier(content string, level TierLevel) (string, error) {
	if !HasTierMarkers(content) {
		// No tier markers - treat as backward compatibility
		// Return full content for any tier level
		return content, nil
	}

	var result strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))

	inTier := false
	currentTier := -1

	for scanner.Scan() {
		line := scanner.Text()

		// Check for tier marker
		if matches := tierMarkerRegex.FindStringSubmatch(line); matches != nil {
			tierNum, err := strconv.Atoi(matches[1])
			if err != nil {
				return "", fmt.Errorf("invalid tier number: %s", matches[1])
			}

			currentTier = tierNum

			// Include this tier if it's <= target level (cumulative)
			if tierNum <= int(level) {
				inTier = true
			} else {
				inTier = false
			}

			// Skip marker line itself
			continue
		}

		// Collect content if in target tier
		if inTier && currentTier >= 0 {
			// Strip blockquote prefix ("> " or just ">")
			cleaned := line
			if after, found := strings.CutPrefix(cleaned, "> "); found {
				cleaned = after
			} else if cleaned == ">" {
				cleaned = "" // Empty blockquote line
			}
			result.WriteString(cleaned)
			result.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan error: %w", err)
	}

	return strings.TrimSpace(result.String()), nil
}

// HasTierMarkers checks if content has tier markers
func HasTierMarkers(content string) bool {
	return tierMarkerRegex.MatchString(content)
}

// ValidateTierMarkers checks for malformed tier markers
func ValidateTierMarkers(content string) error {
	scanner := bufio.NewScanner(strings.NewReader(content))

	foundTiers := make(map[int]bool)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if matches := tierMarkerRegex.FindStringSubmatch(line); matches != nil {
			tierNum, err := strconv.Atoi(matches[1])
			if err != nil {
				return fmt.Errorf("line %d: invalid tier number: %s", lineNum, matches[1])
			}

			// Check tier number in valid range (0-2)
			if tierNum < 0 || tierNum > 2 {
				return fmt.Errorf("line %d: invalid tier number: %d (must be 0-2)", lineNum, tierNum)
			}

			// Track found tiers
			foundTiers[tierNum] = true
		}
	}

	// Verify all tiers present (T0, T1, T2)
	for i := 0; i <= 2; i++ {
		if !foundTiers[i] {
			return fmt.Errorf("missing tier marker: [!T%d]", i)
		}
	}

	return scanner.Err()
}

// CountTokens estimates token count (rough approximation)
// Uses GPT-4 tokenization heuristic: ~0.75 tokens per word
func CountTokens(text string) int {
	words := strings.Fields(text)
	return int(float64(len(words)) * 0.75)
}

// TierMetrics contains tier extraction metrics
type TierMetrics struct {
	Content    string
	Tokens     int
	Characters int
	Lines      int
}

// ExtractTierWithMetrics returns tier content and metrics
func ExtractTierWithMetrics(content string, level TierLevel) (*TierMetrics, error) {
	extracted, err := ExtractTier(content, level)
	if err != nil {
		return nil, err
	}

	metrics := &TierMetrics{
		Content:    extracted,
		Tokens:     CountTokens(extracted),
		Characters: len(extracted),
		Lines:      strings.Count(extracted, "\n") + 1,
	}

	return metrics, nil
}
