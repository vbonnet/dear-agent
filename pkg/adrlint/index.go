package adrlint

import (
	"strings"
)

func validateIndex(root, relative string, data []byte, records map[string]record, maxLines int) []Violation {
	violations := commonDocumentViolations(root, relative, data, maxLines)
	indexed := map[string]record{}
	for _, match := range adrIndexPattern.FindAllStringSubmatch(string(data), -1) {
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
			violations = append(violations, Violation{Path: relative, Reason: name + ": index identity/title/status differs from record"})
		}
	}
	for name := range indexed {
		if _, exists := records[name]; !exists {
			violations = append(violations, Violation{Path: relative, Reason: "index points to non-record " + name})
		}
	}
	return violations
}
