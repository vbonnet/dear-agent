package skipdetect

import (
	"regexp"
	"strings"
)

// Finding represents a detected skip-validation pattern.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Pattern string `json:"pattern"`
	Match   string `json:"match"`
	Level   string `json:"level"` // "warning" or "error"
}

// skipPatterns defines the patterns to detect in Go source files.
var skipPatterns = []struct {
	name     string
	regex    *regexp.Regexp
	level    string
	testOnly bool // only apply to _test.go files
}{
	{
		name:     "t.Skip call",
		regex:    regexp.MustCompile(`\bt\.Skip[f]?\s*\(`),
		level:    "warning",
		testOnly: true,
	},
	{
		name:     "AGM_SKIP_TEST_GATE reference",
		regex:    regexp.MustCompile(`AGM_SKIP_TEST_GATE`),
		level:    "error",
		testOnly: false,
	},
	{
		name:     "--force bypass suggestion",
		regex:    regexp.MustCompile(`--force\s+(to\s+)?(override|bypass|skip)`),
		level:    "warning",
		testOnly: false,
	},
	{
		name:     "--no-verify reference",
		regex:    regexp.MustCompile(`--no-verify`),
		level:    "warning",
		testOnly: false,
	},
}

// ScanLines checks source content for skip-validation patterns.
// filename is used for reporting and testOnly filtering.
func ScanLines(filename string, lines []string) []Finding {
	isTest := strings.HasSuffix(filename, "_test.go")
	var findings []Finding

	for i, line := range lines {
		for _, sp := range skipPatterns {
			if sp.testOnly && !isTest {
				continue
			}
			if sp.regex.MatchString(line) {
				findings = append(findings, Finding{
					File:    filename,
					Line:    i + 1,
					Pattern: sp.name,
					Match:   strings.TrimSpace(line),
					Level:   sp.level,
				})
			}
		}
	}
	return findings
}
