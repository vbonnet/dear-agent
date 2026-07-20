package adrlint

import (
	"fmt"
	"regexp"
	"strings"
)

var adrIndexLikeRow = regexp.MustCompile(`(?i)^\|\s*\[(?:ADR-)?[0-9]+\]\([^)]*\.md(?:#[^)]*)?\)`)

func validateIndex(root, relative string, data []byte, records map[string]record) []Violation {
	violations := commonDocumentViolations(root, relative, data)
	document := markdownOutsideCode(data)
	indexed := map[string]record{}
	for line := range strings.SplitSeq(string(document), "\n") {
		trimmed := strings.TrimSpace(line)
		match := adrIndexPattern.FindStringSubmatch(trimmed)
		if len(match) == 0 {
			if adrIndexLikeRow.MatchString(trimmed) {
				violations = append(violations, Violation{Path: relative, Reason: "malformed ADR index row: " + line})
			}
			continue
		}
		name := match[2]
		if _, exists := indexed[name]; exists {
			violations = append(violations, Violation{Path: relative, Reason: "index contains duplicate row for " + name})
			continue
		}
		indexed[name] = record{
			id:       match[1],
			filename: name,
			title:    strings.TrimSpace(match[3]),
			status:   match[4],
		}
	}
	for name, candidate := range records {
		entry, exists := indexed[name]
		if !exists {
			violations = append(violations, Violation{Path: relative, Reason: "missing index entry for " + name})
			continue
		}
		if entry != candidate {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf(
				"%s: index identity/title/status differs from record (got %s / %q / %s, want %s / %q / %s)",
				name, entry.id, entry.title, entry.status, candidate.id, candidate.title, candidate.status,
			)})
		}
	}
	for name := range indexed {
		if _, exists := records[name]; !exists {
			violations = append(violations, Violation{Path: relative, Reason: "index points to non-record " + name})
		}
	}
	return violations
}
