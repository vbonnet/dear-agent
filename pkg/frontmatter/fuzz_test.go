package frontmatter

import (
	"testing"
)

// FuzzParse feeds random bytes to the markdown frontmatter parser.
// The parser must never panic on any input.
func FuzzParse(f *testing.F) {
	// Seed corpus with representative inputs
	f.Add([]byte("# Heading 1\n## Heading 2\n### Heading 3\n"))
	f.Add([]byte("---\ntitle: Test\n---\n# Real Heading\n"))
	f.Add([]byte(""))
	f.Add([]byte("# Heading with **bold** and *italic*\n"))
	f.Add([]byte("```bash\n# Not a heading\n```\n"))
	f.Add([]byte("---\r\ntitle: CRLF\r\n---\r\n# Heading\r\n"))
	f.Add([]byte("# [Link](https://example.com)\n"))
	f.Add([]byte("###### Deep heading\n"))
	f.Add([]byte("---\nno closing frontmatter\n"))
	f.Add([]byte("# " + string(make([]byte, 4096)) + "\n"))

	parser := NewParser()

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse must not panic on any input
		sections, err := parser.Parse(string(data))
		_ = err

		// If parsing succeeded, exercise FindSections and FuzzyMatch too
		if len(sections) > 0 {
			_ = parser.FindSections(sections, sections[0].Heading, true)
			_ = parser.FindSections(sections, sections[0].Heading, false)
			parser.FuzzyMatch(sections[0].Heading, "test pattern", 0.75)
		}
	})
}

// FuzzFuzzyMatch feeds random strings to the fuzzy matcher.
func FuzzFuzzyMatch(f *testing.F) {
	f.Add("Acceptance Criteria", "Accept Criteria", 0.75)
	f.Add("", "", 0.80)
	f.Add("short", "very long string that is different", 0.50)
	f.Add("unicode cafe\u0301", "unicode cafe", 0.70)

	parser := NewParser()

	f.Fuzz(func(t *testing.T, heading, pattern string, threshold float64) {
		// Must never panic regardless of input
		_ = parser.FuzzyMatch(heading, pattern, threshold)
	})
}
