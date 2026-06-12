// Package earsbdd converts EARS requirements (from SPEC.md files) into
// Gherkin BDD feature stubs. It is the second stage in the
// EARS → BDD → code pipeline that ears-lint starts.
//
// Extraction reads lines line-by-line, identifies requirements by the
// presence of a "shall" keyword (same detector as earslint), captures the
// requirement ID from bold-markdown notation (**ID**), and splits the text
// into a condition clause and an action clause so that Generate can emit
// idiomatic Given/When/Then steps.
package earsbdd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Requirement is a single EARS requirement extracted from a SPEC.md line.
type Requirement struct {
	// ID is the structured identifier, e.g. "FSG-01".
	ID string
	// FullText is the cleaned requirement sentence (markdown stripped, no ID prefix).
	FullText string
	// Condition is the trigger/context clause before "shall". For event-driven
	// requirements like "When X, the system shall Y", Condition is "When X".
	Condition string
	// Action is the expected behavior after "shall", e.g. "allow the write".
	Action string
	// File is the source SPEC.md path.
	File string
	// Line is the 1-based line number in File.
	Line int
}

var (
	// idPattern captures **ID** at the start of a requirement line.
	idPattern = regexp.MustCompile(`\*\*([A-Z][A-Z0-9-]*\d+)\*\*`)
	// shallDetect matches lines that contain "shall" as a whole word.
	shallDetect = regexp.MustCompile(`(?i)\bshall\b`)
	// condTail strips the trailing ", the <actor>" from a condition clause.
	// EARS format: "When <trigger>, the <system> shall <response>" — the actor
	// ("the system", "the write-guard") belongs to the Then step, not the When.
	condTail = regexp.MustCompile(`(?i),?\s+the\s+[\w][\w-]*\s*$`)
	// listMarker strips leading markdown list/heading markers.
	listMarker = regexp.MustCompile(`^(\s*[-*+]\s+|\s*\d+[.)]\s+)`)
)

// Extract reads from r and returns every EARS requirement found.
// name is used as the File field in each returned Requirement.
func Extract(name string, r io.Reader) ([]Requirement, error) {
	var reqs []Requirement
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	inFence := false
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		if !shallDetect.MatchString(trimmed) {
			continue
		}

		id := ""
		if m := idPattern.FindStringSubmatch(trimmed); m != nil {
			id = m[1]
		}

		clean := stripMarkdown(trimmed)
		if clean == "" {
			continue
		}
		// After markdown stripping, **FSG-01** becomes "FSG-01" and sits at the
		// front of the text. Remove it so splitShall sees a clean EARS sentence.
		if id != "" {
			clean = strings.TrimSpace(strings.TrimPrefix(clean, id))
		}

		condition, action := splitShall(clean)
		reqs = append(reqs, Requirement{
			ID:        id,
			FullText:  clean,
			Condition: condition,
			Action:    action,
			File:      name,
			Line:      lineNo,
		})
	}
	if err := scanner.Err(); err != nil {
		return reqs, fmt.Errorf("reading %s: %w", name, err)
	}
	return reqs, nil
}

// ExtractFile extracts requirements from the file at path.
func ExtractFile(path string) ([]Requirement, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	return Extract(path, f)
}

// splitShall splits a cleaned requirement line on the first occurrence of
// "shall", returning the condition clause (before) and the action clause
// (after). If no "shall" is found, condition is the full text and action
// is empty.
func splitShall(text string) (condition, action string) {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, " shall ")
	if idx < 0 {
		return text, ""
	}
	condition = strings.TrimSpace(text[:idx])
	action = strings.TrimSpace(text[idx+7:]) // len(" shall ") == 7
	// Strip the trailing ", the <actor>" from the condition: in EARS format
	// "When <trigger>, the <system> shall <response>", the actor is implicit
	// in the Then step and clutters the When step.
	condition = strings.TrimSpace(condTail.ReplaceAllString(condition, ""))
	// Remove trailing period from action if present.
	action = strings.TrimRight(action, ".")
	action = strings.TrimSpace(action)
	return condition, action
}

// stripMarkdown removes markdown list markers, headings, bold/italic emphasis,
// requirement ID prefixes (**ID**), and backtick code spans.
func stripMarkdown(s string) string {
	s = listMarker.ReplaceAllString(s, "")
	s = strings.TrimLeft(s, "#")
	s = strings.TrimSpace(s)
	// Remove **bold** and *italic* emphasis (keep inner text).
	// Removing the ID prefix (**FSG-01**) is a side effect of this.
	s = regexp.MustCompile(`\*{1,3}([^*]+)\*{1,3}`).ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}
