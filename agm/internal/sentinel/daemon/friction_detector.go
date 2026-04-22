package daemon

import (
	"regexp"
	"strings"
)

// FrictionSignal represents a detected friction phrase in pane content.
type FrictionSignal struct {
	PatternID   string // normalized pattern key, e.g. "friction:recurring_issue"
	Description string // the matched phrase from the content
	Raw         string // full line containing the match
}

// frictionPattern pairs a compiled regex with its canonical pattern ID and
// a human-readable description template.
type frictionPattern struct {
	re          *regexp.Regexp
	patternID   string
	description string
}

// frictionPatterns are the phrases that signal recurring friction.
// Each regex is case-insensitive and matches within a single line.
var frictionPatterns = []frictionPattern{
	{
		re:          regexp.MustCompile(`(?i)\bsame\s+\S+\s+as\s+always\b`),
		patternID:   "friction:same_as_always",
		description: "Recurring issue: same problem as always",
	},
	{
		re:          regexp.MustCompile(`(?i)\bthis\s+keeps\s+happening\b`),
		patternID:   "friction:keeps_happening",
		description: "Recurring issue: this keeps happening",
	},
	{
		re:          regexp.MustCompile(`(?i)\brecurring\s+issue\b`),
		patternID:   "friction:recurring_issue",
		description: "Explicitly flagged as recurring issue",
	},
	{
		re:          regexp.MustCompile(`(?i)\bevery\s+session\s+hits\s+this\b`),
		patternID:   "friction:every_session",
		description: "Friction observed in every session",
	},
	{
		re:          regexp.MustCompile(`(?i)\bhappens\s+every\s+time\b`),
		patternID:   "friction:every_time",
		description: "Friction happens every time",
	},
	{
		re:          regexp.MustCompile(`(?i)\bsame\s+error\s+again\b`),
		patternID:   "friction:same_error_again",
		description: "Same error encountered again",
	},
	{
		re:          regexp.MustCompile(`(?i)\bworkaround\s+(?:for|again)\b`),
		patternID:   "friction:workaround",
		description: "Workaround applied for known issue",
	},
	{
		re:          regexp.MustCompile(`(?i)\bknown\s+issue\b`),
		patternID:   "friction:known_issue",
		description: "Reference to a known issue",
	},
	{
		re:          regexp.MustCompile(`(?i)\bkeep(?:s)?\s+(?:failing|breaking|erroring)\b`),
		patternID:   "friction:keeps_failing",
		description: "Something keeps failing or breaking",
	},
	{
		re:          regexp.MustCompile(`(?i)\bhit(?:ting)?\s+this\s+(?:bug|issue|problem|error)\s+again\b`),
		patternID:   "friction:hit_again",
		description: "Hit the same bug/issue again",
	},
	{
		re:          regexp.MustCompile(`(?i)\balways\s+(?:fails|breaks|errors)\s+(?:here|at|on)\b`),
		patternID:   "friction:always_fails",
		description: "Consistently fails at a specific point",
	},
	{
		re:          regexp.MustCompile(`(?i)\bnot\s+again\b`),
		patternID:   "friction:not_again",
		description: "Frustration at recurring problem",
	},
}

// DetectFriction scans multi-line content for friction signal phrases.
// Returns all detected signals. Lines that don't match are skipped.
func DetectFriction(content string) []*FrictionSignal {
	var signals []*FrictionSignal
	seen := make(map[string]bool) // deduplicate by patternID within one scan

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for _, fp := range frictionPatterns {
			if seen[fp.patternID] {
				continue // one signal per pattern per scan
			}
			if match := fp.re.FindString(line); match != "" {
				signals = append(signals, &FrictionSignal{
					PatternID:   fp.patternID,
					Description: fp.description,
					Raw:         line,
				})
				seen[fp.patternID] = true
				break // one pattern per line
			}
		}
	}

	return signals
}
