// Package frontmatter provides markdown section parsing with fuzzy matching.
//
// This package extracts headings from markdown documents with accurate line numbers
// and provides fuzzy matching capabilities for anti-pattern detection.
package frontmatter

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Section represents an extracted markdown heading.
type Section struct {
	// Heading text (without formatting)
	Heading string `json:"heading"`

	// Heading level (1-6 for h1-h6)
	Level int `json:"level"`

	// Line number in source document (1-indexed)
	StartLine int `json:"startLine"`

	// Original heading with formatting (for debugging)
	Raw string `json:"raw,omitempty"`
}

// Parser extracts sections from markdown documents.
type Parser struct {
	parser parser.Parser
}

// NewParser creates a new markdown section parser.
func NewParser() *Parser {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
		),
	)
	return &Parser{
		parser: md.Parser(),
	}
}

// Parse extracts all headings from a markdown document.
//
// Uses goldmark to parse markdown into an AST, then extracts all heading
// nodes with their text, level, and line numbers.
//
// Example:
//
//	parser := NewParser()
//	sections, err := parser.Parse("# H1\n## H2\n### H3")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// sections contains 3 entries with heading, level, and startLine
func (p *Parser) Parse(markdown string) ([]Section, error) {
	// Detect frontmatter boundaries
	frontmatterEnd := detectFrontmatterEnd(markdown)

	reader := text.NewReader([]byte(markdown))
	doc := p.parser.Parse(reader)

	var sections []Section

	// Walk the AST and extract headings
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		// Only process nodes when entering (not exiting)
		if !entering {
			return ast.WalkContinue, nil
		}

		// Check if node is a heading
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Extract text from heading
		headingText := extractText(heading, []byte(markdown))
		if headingText == "" {
			return ast.WalkContinue, nil
		}

		// Get line number (1-indexed)
		lines := heading.Lines()
		if lines.Len() == 0 {
			return ast.WalkContinue, nil
		}
		startLine := lines.At(0).Start

		// Count newlines before this position to get 1-indexed line number
		lineNumber := 1
		for i := 0; i < startLine; i++ {
			if markdown[i] == '\n' {
				lineNumber++
			}
		}

		// Skip headings inside frontmatter
		if startLine < frontmatterEnd {
			return ast.WalkContinue, nil
		}

		sections = append(sections, Section{
			Heading:   headingText,
			Level:     heading.Level,
			StartLine: lineNumber,
			Raw:       extractRaw(heading, headingText),
		})

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse markdown: %w", err)
	}

	return sections, nil
}

// FuzzyMatch checks if heading matches pattern with fuzzy matching.
//
// Uses Levenshtein distance to calculate similarity between the
// heading and pattern strings. Normalizes both strings (lowercase,
// trim whitespace) before comparison.
//
// Example:
//
//	parser := NewParser()
//	match := parser.FuzzyMatch("Acceptance Criteria", "Accept Criteria", 0.80)
//	// match is true (similarity: ~0.79)
func (p *Parser) FuzzyMatch(heading, pattern string, threshold float64) bool {
	// Normalize: lowercase, trim whitespace
	h := strings.ToLower(strings.TrimSpace(heading))
	pat := strings.ToLower(strings.TrimSpace(pattern))

	// Exact match
	if h == pat {
		return true
	}

	// Calculate Levenshtein similarity
	distance := levenshteinDistance(h, pat)
	maxLength := maxInt(len(h), len(pat))
	if maxLength == 0 {
		return true // both empty
	}

	similarity := 1.0 - float64(distance)/float64(maxLength)

	return similarity >= threshold
}

// FindSections finds sections matching a pattern (exact or fuzzy).
//
// Searches through sections array and returns all sections whose
// headings match the given pattern.
//
// Example:
//
//	sections := []Section{
//	    {Heading: "Acceptance Criteria", Level: 2, StartLine: 10},
//	    {Heading: "Task Breakdown", Level: 2, StartLine: 20},
//	}
//	// Fuzzy match
//	matches := parser.FindSections(sections, "Accept Criteria", true)
//	// Returns the first section (fuzzy match)
func (p *Parser) FindSections(sections []Section, pattern string, fuzzy bool) []Section {
	var matches []Section

	if fuzzy {
		for _, s := range sections {
			if p.FuzzyMatch(s.Heading, pattern, 0.75) {
				matches = append(matches, s)
			}
		}
	} else {
		normalized := strings.ToLower(strings.TrimSpace(pattern))
		for _, s := range sections {
			sectionNorm := strings.ToLower(strings.TrimSpace(s.Heading))
			if sectionNorm == normalized {
				matches = append(matches, s)
			}
		}
	}

	return matches
}

// extractText extracts plain text from a heading node (strips formatting).
//
// Recursively extracts text from heading's children, handling:
// - Plain text nodes
// - Inline code
// - Bold/italic formatting
// - Links
// - Other inline elements
func extractText(heading *ast.Heading, source []byte) string {
	var buf strings.Builder

	for child := heading.FirstChild(); child != nil; child = child.NextSibling() {
		extractNodeText(child, source, &buf)
	}

	return strings.TrimSpace(buf.String())
}

// extractNodeText recursively extracts text from AST nodes.
func extractNodeText(node ast.Node, source []byte, buf *strings.Builder) {
	switch n := node.(type) {
	case *ast.Text:
		buf.Write(n.Segment.Value(source))
	case *ast.String:
		buf.Write(n.Value)
	case *ast.CodeSpan:
		// CodeSpan has children that are Text nodes
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			extractNodeText(child, source, buf)
		}
	default:
		// Recursively process children for other node types (bold, italic, links, etc.)
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			extractNodeText(child, source, buf)
		}
	}
}

// extractRaw reconstructs the markdown heading syntax.
func extractRaw(heading *ast.Heading, text string) string {
	return strings.Repeat("#", heading.Level) + " " + text
}

// levenshteinDistance calculates the Levenshtein edit distance between two strings.
func levenshteinDistance(s1, s2 string) int {
	r1 := []rune(s1)
	r2 := []rune(s2)
	len1 := len(r1)
	len2 := len(r2)

	// Create distance matrix
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
	}

	// Initialize first column and row
	for i := 0; i <= len1; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}

			matrix[i][j] = minInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len1][len2]
}

func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// detectFrontmatterEnd returns the byte position where frontmatter ends.
// Returns 0 if there's no frontmatter.
// Frontmatter is delimited by --- at the start and end.
func detectFrontmatterEnd(markdown string) int {
	// Check if document starts with frontmatter delimiter
	if !strings.HasPrefix(markdown, "---\n") && !strings.HasPrefix(markdown, "---\r\n") {
		return 0
	}

	// Find the closing delimiter
	// Skip the opening ---
	start := strings.Index(markdown, "\n")
	if start == -1 {
		return 0
	}
	start++ // Move past the newline

	// Look for closing ---
	end := strings.Index(markdown[start:], "\n---\n")
	if end == -1 {
		end = strings.Index(markdown[start:], "\n---\r\n")
	}
	if end == -1 {
		// No closing delimiter found
		return 0
	}

	// Return byte position after frontmatter
	// end is relative to start, so adjust
	return start + end + 5 // 5 = len("\n---\n")
}
