package phaseisolation

import (
	"regexp"
	"strings"
)

// Section represents a heading extracted from a markdown document.
type Section struct {
	Heading   string // Heading text (without formatting)
	Level     int    // Heading level (1-6)
	StartLine int    // Line number in source (1-indexed)
}

// SectionParser extracts headings from markdown documents.
// This is a simplified Go implementation that uses regex-based parsing
// instead of a full AST parser. For most use cases this is sufficient.
type SectionParser struct{}

// NewSectionParser creates a new SectionParser.
func NewSectionParser() *SectionParser {
	return &SectionParser{}
}

var headingRegex = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// Parse extracts all headings from a markdown document.
func (sp *SectionParser) Parse(markdown string) []Section {
	lines := strings.Split(markdown, "\n")
	var sections []Section
	inCodeBlock := false
	inFrontmatter := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle frontmatter
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
			}
			continue
		}

		// Handle code blocks
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		// Match headings
		matches := headingRegex.FindStringSubmatch(trimmed)
		if matches != nil {
			level := len(matches[1])
			heading := strings.TrimSpace(matches[2])
			// Strip inline formatting (bold, italic, code)
			heading = stripInlineFormatting(heading)
			sections = append(sections, Section{
				Heading:   heading,
				Level:     level,
				StartLine: i + 1,
			})
		}
	}

	return sections
}

// stripInlineFormatting removes bold, italic, and inline code from text.
func stripInlineFormatting(text string) string {
	// Remove bold
	text = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(text, "$1")
	// Remove italic
	text = regexp.MustCompile(`\*(.+?)\*`).ReplaceAllString(text, "$1")
	// Remove inline code
	text = regexp.MustCompile("`(.+?)`").ReplaceAllString(text, "$1")
	return text
}

// FuzzyMatch checks if heading matches pattern using Levenshtein distance.
func (sp *SectionParser) FuzzyMatch(heading, pattern string, threshold float64) bool {
	h := strings.ToLower(strings.TrimSpace(heading))
	p := strings.ToLower(strings.TrimSpace(pattern))

	if h == p {
		return true
	}

	distance := levenshteinDistance(h, p)
	maxLen := len(h)
	if len(p) > maxLen {
		maxLen = len(p)
	}
	if maxLen == 0 {
		return true
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)
	return similarity >= threshold
}

// FindSections finds sections matching a pattern.
func (sp *SectionParser) FindSections(sections []Section, pattern string, fuzzy bool) []Section {
	var result []Section
	for _, s := range sections {
		if fuzzy {
			if sp.FuzzyMatch(s.Heading, pattern, 0.75) {
				result = append(result, s)
			}
		} else {
			if strings.EqualFold(strings.TrimSpace(s.Heading), strings.TrimSpace(pattern)) {
				result = append(result, s)
			}
		}
	}
	return result
}

// levenshteinDistance computes the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two rows instead of full matrix
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				curr[j-1]+1,
				min(prev[j]+1, prev[j-1]+cost),
			)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}
