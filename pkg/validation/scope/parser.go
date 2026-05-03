// Package scope provides scope-related functionality.
package scope

import (
	"regexp"
	"strings"
)

// Parser handles markdown section extraction and fuzzy matching
type Parser struct{}

// NewParser creates a new section parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse extracts all headings from a markdown document
func (p *Parser) Parse(markdown string) []Section {
	sections := []Section{}
	lines := strings.Split(markdown, "\n")

	// Regex for ATX-style headings: ^#{1,6}\s+(.+?)(?:\s+#+)?$
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+?)(?:\s+#+)?$`)

	for i, line := range lines {
		if matches := headingRegex.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			heading := p.stripFormatting(matches[2])

			sections = append(sections, Section{
				Heading:   heading,
				Level:     level,
				StartLine: i + 1, // 1-indexed
				Raw:       line,
			})
		}
	}

	return sections
}

// stripFormatting removes markdown formatting from heading text
func (p *Parser) stripFormatting(text string) string {
	// Remove inline code: `code`
	text = regexp.MustCompile("`[^`]+`").ReplaceAllString(text, "")

	// Remove bold: **text** or __text__
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`__([^_]+)__`).ReplaceAllString(text, "$1")

	// Remove italic: *text* or _text_
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(text, "$1")

	// Remove links: [text](url) -> text
	text = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(text, "$1")

	// Remove images: ![alt](url) -> ""
	text = regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`).ReplaceAllString(text, "")

	// Collapse multiple spaces to single space
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// FuzzyMatch checks if heading matches pattern using Levenshtein distance
func (p *Parser) FuzzyMatch(heading, pattern string, threshold float64) bool {
	// Normalize: lowercase, trim whitespace
	h := strings.ToLower(strings.TrimSpace(heading))
	ptn := strings.ToLower(strings.TrimSpace(pattern))

	// Exact match
	if h == ptn {
		return true
	}

	// Calculate Levenshtein similarity
	distance := levenshteinDistance(h, ptn)
	maxLength := max(len(h), len(ptn))

	if maxLength == 0 {
		return h == ptn
	}

	similarity := 1.0 - float64(distance)/float64(maxLength)

	return similarity >= threshold
}

// FindSections searches sections for matches to pattern
func (p *Parser) FindSections(sections []Section, pattern string, fuzzy bool) []Section {
	matches := []Section{}

	if fuzzy {
		// Use default threshold of 0.75
		for _, section := range sections {
			if p.FuzzyMatch(section.Heading, pattern, 0.75) {
				matches = append(matches, section)
			}
		}
	} else {
		// Exact match (case-insensitive)
		normalized := strings.ToLower(strings.TrimSpace(pattern))
		for _, section := range sections {
			if strings.ToLower(strings.TrimSpace(section.Heading)) == normalized {
				matches = append(matches, section)
			}
		}
	}

	return matches
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	// Convert strings to rune slices for proper Unicode handling
	r1 := []rune(s1)
	r2 := []rune(s2)

	len1 := len(r1)
	len2 := len(r2)

	// Early exit for empty strings
	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	// Create distance matrix with size (len1+1) x (len2+1)
	// Use two rows instead of full matrix for memory efficiency
	prevRow := make([]int, len2+1)
	currRow := make([]int, len2+1)

	// Initialize first row
	for j := 0; j <= len2; j++ {
		prevRow[j] = j
	}

	// Calculate distances
	for i := 1; i <= len1; i++ {
		currRow[0] = i

		for j := 1; j <= len2; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}

			deletion := prevRow[j] + 1
			insertion := currRow[j-1] + 1
			substitution := prevRow[j-1] + cost

			currRow[j] = min(deletion, min(insertion, substitution))
		}

		// Swap rows
		prevRow, currRow = currRow, prevRow
	}

	return prevRow[len2]
}

