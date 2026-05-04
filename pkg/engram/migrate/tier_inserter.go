package migrate

import (
	"strings"
)

// InsertTierMarkers adds tier markers to content based on heuristics
func InsertTierMarkers(content string) (string, error) {
	lines := strings.Split(content, "\n")

	// Find frontmatter end
	frontmatterEnd := findFrontmatterEnd(lines)
	if frontmatterEnd == -1 {
		frontmatterEnd = 0
	}

	// Analyze content structure
	structure := analyzeStructure(lines[frontmatterEnd:])

	// Generate tiered content
	var result strings.Builder

	// Write frontmatter unchanged
	for i := 0; i <= frontmatterEnd; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}

	// Insert T0: First paragraph or frontmatter description
	result.WriteString("\n> [!T0]\n")
	t0Content := extractT0(lines[frontmatterEnd:], structure)
	for _, line := range t0Content {
		result.WriteString("> ")
		result.WriteString(line)
		result.WriteString("\n")
	}

	// Insert T1: First 2-3 sections (overview level)
	result.WriteString("\n> [!T1]\n")
	t1Content := extractT1(lines[frontmatterEnd:], structure)
	for _, line := range t1Content {
		result.WriteString("> ")
		result.WriteString(line)
		result.WriteString("\n")
	}

	// Insert T2: Full remaining content
	result.WriteString("\n> [!T2]\n")
	t2Content := extractT2(lines[frontmatterEnd:], structure)
	for _, line := range t2Content {
		result.WriteString("> ")
		result.WriteString(line)
		result.WriteString("\n")
	}

	return result.String(), nil
}

// ContentStructure represents analyzed content
type ContentStructure struct {
	Paragraphs []Paragraph
	Headers    []Header
	CodeBlocks []CodeBlock
}

// Paragraph represents a paragraph block
type Paragraph struct {
	StartLine int
	EndLine   int
	Text      string
}

// Header represents a markdown header
type Header struct {
	Line  int
	Level int
	Text  string
}

// CodeBlock represents a code block
type CodeBlock struct {
	StartLine int
	EndLine   int
	Language  string
}

// analyzeStructure analyzes content structure
func analyzeStructure(lines []string) *ContentStructure {
	structure := &ContentStructure{}

	inCodeBlock := false
	currentParagraph := []string{}
	paragraphStart := 0

	for i, line := range lines {
		// Check for code block
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				// Save current paragraph if any
				if len(currentParagraph) > 0 {
					structure.Paragraphs = append(structure.Paragraphs, Paragraph{
						StartLine: paragraphStart,
						EndLine:   i - 1,
						Text:      strings.Join(currentParagraph, "\n"),
					})
					currentParagraph = []string{}
				}
			}
			continue
		}

		// Check for header
		if strings.HasPrefix(line, "#") {
			level := strings.Count(strings.Split(line, " ")[0], "#")
			text := strings.TrimSpace(strings.TrimLeft(line, "#"))
			structure.Headers = append(structure.Headers, Header{
				Line:  i,
				Level: level,
				Text:  text,
			})

			// Save current paragraph
			if len(currentParagraph) > 0 {
				structure.Paragraphs = append(structure.Paragraphs, Paragraph{
					StartLine: paragraphStart,
					EndLine:   i - 1,
					Text:      strings.Join(currentParagraph, "\n"),
				})
				currentParagraph = []string{}
			}
			continue
		}

		// Build paragraph
		if !inCodeBlock {
			if strings.TrimSpace(line) == "" {
				// End of paragraph
				if len(currentParagraph) > 0 {
					structure.Paragraphs = append(structure.Paragraphs, Paragraph{
						StartLine: paragraphStart,
						EndLine:   i - 1,
						Text:      strings.Join(currentParagraph, "\n"),
					})
					currentParagraph = []string{}
				}
			} else {
				if len(currentParagraph) == 0 {
					paragraphStart = i
				}
				currentParagraph = append(currentParagraph, line)
			}
		}
	}

	// Save final paragraph
	if len(currentParagraph) > 0 {
		structure.Paragraphs = append(structure.Paragraphs, Paragraph{
			StartLine: paragraphStart,
			EndLine:   len(lines) - 1,
			Text:      strings.Join(currentParagraph, "\n"),
		})
	}

	return structure
}

// extractT0 extracts T0 content (first paragraph, 50-150 tokens)
func extractT0(_ []string, structure *ContentStructure) []string {
	if len(structure.Paragraphs) == 0 {
		return []string{"TODO: Add summary"}
	}

	// Use first paragraph as T0
	firstPara := structure.Paragraphs[0]
	paraLines := strings.Split(firstPara.Text, "\n")

	// Trim to ~150 tokens (roughly first 2-3 sentences)
	if len(paraLines) > 3 {
		paraLines = paraLines[:3]
	}

	return paraLines
}

// extractT1 extracts T1 content (first 2-3 sections, 150-500 tokens)
func extractT1(lines []string, structure *ContentStructure) []string {
	result := []string{}

	// Find first major section (## level)
	firstSectionIdx := -1
	for i, header := range structure.Headers {
		if header.Level <= 2 {
			firstSectionIdx = i
			break
		}
	}

	if firstSectionIdx == -1 {
		// No sections, use first few paragraphs
		for i := 0; i < min(3, len(structure.Paragraphs)); i++ {
			result = append(result, strings.Split(structure.Paragraphs[i].Text, "\n")...)
		}
		return result
	}

	// Extract first 2-3 sections
	endLine := len(lines)
	if firstSectionIdx+3 < len(structure.Headers) {
		endLine = structure.Headers[firstSectionIdx+3].Line
	}

	startLine := structure.Headers[firstSectionIdx].Line
	for i := startLine; i < endLine && i < len(lines); i++ {
		result = append(result, lines[i])
	}

	return result
}

// extractT2 extracts T2 content (everything)
func extractT2(lines []string, structure *ContentStructure) []string {
	return lines
}

func findFrontmatterEnd(lines []string) int {
	if len(lines) < 2 || lines[0] != "---" {
		return -1
	}

	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return i
		}
	}

	return -1
}
