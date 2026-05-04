package hippocampus

import (
	"strings"
)

// MemoryDocument represents a parsed MEMORY.md file as a tree of sections.
type MemoryDocument struct {
	Preamble string          // content before first heading (may be empty)
	Sections []MemorySection // parsed top-level sections
}

// MemorySection represents a heading and its content within a MEMORY.md file.
type MemorySection struct {
	Level    int             // heading level (1=#, 2=##, 3=###)
	Heading  string          // heading text without # prefix
	Content  []string        // lines under this heading (bullets, text, blank lines)
	Children []MemorySection // nested subsections
}

// ParseMemoryMD parses markdown content into a MemoryDocument tree.
// It handles #, ##, and ### headings, nesting deeper headings as children.
// The parser is forgiving and does not error on unusual formatting.
func ParseMemoryMD(content string) (*MemoryDocument, error) {
	doc := &MemoryDocument{}

	if content == "" {
		return doc, nil
	}

	lines := strings.Split(content, "\n")

	var levels []int
	var headings []string
	var contents [][]string
	var preambleLines []string
	inPreamble := true

	for _, line := range lines {
		level, heading := parseHeading(line)
		switch {
		case level > 0:
			inPreamble = false
			levels = append(levels, level)
			headings = append(headings, heading)
			contents = append(contents, nil)
		case inPreamble:
			preambleLines = append(preambleLines, line)
		case len(levels) > 0:
			idx := len(contents) - 1
			contents[idx] = append(contents[idx], line)
		}
	}

	if len(preambleLines) > 0 {
		doc.Preamble = strings.Join(preambleLines, "\n")
	}

	doc.Sections = buildTree(levels, headings, contents)

	return doc, nil
}

// buildTree converts parallel slices of section data into a nested tree.
// Sections with a higher level number (deeper nesting) that immediately follow
// a section become its children.
func buildTree(levels []int, headings []string, contents [][]string) []MemorySection {
	if len(levels) == 0 {
		return nil
	}

	var result []MemorySection
	i := 0

	for i < len(levels) {
		sec := MemorySection{
			Level:   levels[i],
			Heading: headings[i],
			Content: contents[i],
		}

		// Collect children: consecutive sections with a deeper level.
		j := i + 1
		for j < len(levels) && levels[j] > levels[i] {
			j++
		}

		if j > i+1 {
			sec.Children = buildTree(levels[i+1:j], headings[i+1:j], contents[i+1:j])
		}

		result = append(result, sec)
		i = j
	}

	return result
}

// parseHeading checks if a line is a markdown heading and returns level and text.
// Returns (0, "") if the line is not a heading.
func parseHeading(line string) (int, string) {
	if len(line) == 0 || line[0] != '#' {
		return 0, ""
	}

	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}

	// Must be followed by a space or end of string.
	if level < len(line) && line[level] != ' ' {
		return 0, ""
	}

	heading := ""
	if level < len(line) {
		heading = strings.TrimSpace(line[level:])
	}

	return level, heading
}

// Render reconstructs the markdown content from the document tree.
// It is round-trip safe: ParseMemoryMD(doc.Render()) produces an identical document.
func (d *MemoryDocument) Render() string {
	var parts []string

	if d.Preamble != "" {
		parts = append(parts, d.Preamble)
	}

	for _, sec := range d.Sections {
		parts = append(parts, renderSection(sec)...)
	}

	return strings.Join(parts, "\n")
}

// renderSection renders a single section and its children as lines.
func renderSection(sec MemorySection) []string {
	var lines []string

	// Heading line.
	prefix := strings.Repeat("#", sec.Level)
	if sec.Heading != "" {
		lines = append(lines, prefix+" "+sec.Heading)
	} else {
		lines = append(lines, prefix)
	}

	// Content lines.
	lines = append(lines, sec.Content...)

	// Children.
	for _, child := range sec.Children {
		lines = append(lines, renderSection(child)...)
	}

	return lines
}

// LineCount returns the total number of lines in the rendered document.
func (d *MemoryDocument) LineCount() int {
	rendered := d.Render()
	if rendered == "" {
		return 0
	}
	return strings.Count(rendered, "\n") + 1
}

// AddEntry appends an entry (typically a bullet line) to the section with the
// given heading. If the section does not exist, a new level-2 section is created.
func (d *MemoryDocument) AddEntry(sectionHeading string, entry string) {
	sec := d.FindSection(sectionHeading)
	if sec != nil {
		sec.Content = append(sec.Content, entry)
		return
	}

	// Create new section at level 2.
	newSec := MemorySection{
		Level:   2,
		Heading: sectionHeading,
		Content: []string{entry},
	}
	d.Sections = append(d.Sections, newSec)
}

// RemoveEntry removes the first occurrence of entry from the section with the
// given heading. Returns false if the section or entry was not found.
func (d *MemoryDocument) RemoveEntry(sectionHeading string, entry string) bool {
	sec := d.FindSection(sectionHeading)
	if sec == nil {
		return false
	}

	for i, line := range sec.Content {
		if line == entry {
			sec.Content = append(sec.Content[:i], sec.Content[i+1:]...)
			return true
		}
	}

	return false
}

// FindSection searches the document tree for a section with the given heading text.
// It searches all levels (top-level and children) using depth-first traversal.
// Returns a pointer to the section, or nil if not found.
func (d *MemoryDocument) FindSection(heading string) *MemorySection {
	for i := range d.Sections {
		if found := findInSection(&d.Sections[i], heading); found != nil {
			return found
		}
	}
	return nil
}

// findInSection recursively searches a section and its children for a heading match.
func findInSection(sec *MemorySection, heading string) *MemorySection {
	if sec.Heading == heading {
		return sec
	}
	for i := range sec.Children {
		if found := findInSection(&sec.Children[i], heading); found != nil {
			return found
		}
	}
	return nil
}
