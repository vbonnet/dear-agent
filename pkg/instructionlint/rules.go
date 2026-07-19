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
var shellBoundary = regexp.MustCompile(`(?:&&|\|\||[;|])`)
var environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

var instructionRules = []rule{
	{id: "wayfinder-v1", replacement: "CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, and RETRO", applies: proseOrShell, detect: retiredWayfinder.MatchString},
	{id: "bare-beads", replacement: "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>", applies: commandSegment, detect: bareBeads},
	{id: "raw-git-push", replacement: "safe-push", applies: commandSegment, detect: func(text string) bool { return strings.Contains(shellText(text), "git push") }},
	{id: "raw-gh-merge", replacement: "safe-merge", applies: commandSegment, detect: rawGHMerge},
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
	for _, fields := range commandFields(text) {
		if len(fields) > 0 && fields[0] == "bd" && beadsCommandIsBare(fields) {
			return true
		}
	}
	return false
}

func beadsCommandIsBare(fields []string) bool {
	const canonical = "~/beads/context-engine/.beads"
	if len(fields) >= 3 && fields[1] == "--db" {
		return strings.Trim(fields[2], `"'`) != canonical
	}
	if len(fields) >= 2 {
		if value, ok := strings.CutPrefix(fields[1], "--db="); ok {
			return strings.Trim(value, `"'`) != canonical
		}
	}
	return true
}

func rawGHMerge(text string) bool {
	for _, fields := range commandFields(text) {
		if len(fields) >= 3 && fields[0] == "gh" && fields[1] == "pr" && fields[2] == "merge" {
			return true
		}
	}
	return false
}

func commandFields(text string) [][]string {
	var commands [][]string
	for _, command := range shellBoundary.Split(shellText(text), -1) {
		fields := stripCommandPrefixes(strings.Fields(strings.TrimSpace(command)))
		if len(fields) > 0 {
			commands = append(commands, fields)
		}
	}
	return commands
}

func stripCommandPrefixes(fields []string) []string {
	for len(fields) > 0 {
		switch {
		case environmentAssignment.MatchString(fields[0]):
			fields = fields[1:]
		case fields[0] == "env":
			fields = fields[1:]
		case (fields[0] == "timeout" || fields[0] == "gtimeout") && len(fields) >= 2:
			fields = fields[2:]
		default:
			return fields
		}
	}
	return fields
}

func positionalAGMSearch(text string) bool {
	fields := strings.Fields(shellText(text))
	return len(fields) > 2 && fields[0] == "agm" && fields[1] == "search" && !strings.HasPrefix(fields[2], "-")
}
