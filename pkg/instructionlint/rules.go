package instructionlint

import (
	"regexp"
	"strings"
)

type rule struct {
	id          string
	replacement string
	applies     func(SegmentKind) bool
	detect      func(string) bool
}

var retiredWayfinder = regexp.MustCompile(`(?:\bV1\b|\bW0(?:\b|-)|\bD[1-6](?:\b|-)|\bS(?:1[01]|[1-9])(?:\b|-))`)

var instructionRules = []rule{
	{id: "wayfinder-v1", replacement: "CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, and RETRO", applies: proseOrShell, detect: retiredWayfinder.MatchString},
	{id: "bare-beads", replacement: "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>", applies: commandSegment, detect: bareBeads},
	{id: "raw-git-push", replacement: "safe-push", applies: commandSegment, detect: func(text string) bool { return strings.Contains(shellText(text), "git push") }},
	{id: "raw-gh-merge", replacement: "safe-merge", applies: commandSegment, detect: func(text string) bool { return strings.HasPrefix(shellText(text), "gh pr merge") }},
	{id: "safe-pr-emergency", replacement: "safe-pr normally or escalate with agm escalate; there is no bypass flag", applies: commandSegment, detect: func(text string) bool {
		normalized := shellText(text)
		return strings.Contains(normalized, "safe-pr") && strings.Contains(normalized, "--emergency")
	}},
	{id: "agm-health-json", replacement: "agm -o json session health <name>", applies: commandSegment, detect: func(text string) bool {
		normalized := shellText(text)
		return strings.HasPrefix(normalized, "agm session health") && strings.Contains(normalized, "--json")
	}},
	{id: "agm-root-status", replacement: "agm session status --format json", applies: commandSegment, detect: func(text string) bool { return strings.HasPrefix(shellText(text), "agm status") }},
	{id: "agm-positional-search", replacement: "agm search --query <text>", applies: commandSegment, detect: positionalAGMSearch},
	{id: "agm-root-new", replacement: "agm session new --workspace <name>", applies: commandSegment, detect: func(text string) bool { return strings.HasPrefix(shellText(text), "agm new") }},
	{id: "agm-session-send", replacement: "agm send <session> <message>", applies: commandSegment, detect: func(text string) bool { return strings.HasPrefix(shellText(text), "agm session send") }},
}

func knownRule(id string) bool {
	for _, candidate := range instructionRules {
		if candidate.id == id {
			return true
		}
	}
	return false
}

func evaluateSegment(path string, segment Segment) []Violation {
	var violations []Violation
	for _, candidate := range instructionRules {
		if candidate.applies(segment.Kind) && candidate.detect(segment.Text) {
			violations = append(violations, Violation{
				Path:        path,
				Line:        segment.Line,
				Rule:        candidate.id,
				Excerpt:     strings.TrimSpace(segment.Text),
				Replacement: candidate.replacement,
			})
		}
	}
	return violations
}

func proseOrShell(kind SegmentKind) bool {
	return kind == SegmentProse || kind == SegmentShell
}

func commandSegment(kind SegmentKind) bool {
	return kind == SegmentInline || kind == SegmentShell
}

func shellText(text string) string {
	normalized := strings.TrimSpace(text)
	normalized = strings.TrimPrefix(normalized, "$ ")
	return strings.TrimSpace(normalized)
}

func bareBeads(text string) bool {
	fields := strings.Fields(shellText(text))
	return len(fields) > 0 && fields[0] == "bd" && (len(fields) == 1 || fields[1] != "--db")
}

func positionalAGMSearch(text string) bool {
	fields := strings.Fields(shellText(text))
	return len(fields) > 2 && fields[0] == "agm" && fields[1] == "search" && !strings.HasPrefix(fields[2], "-")
}
