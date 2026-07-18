package adrlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	adrFilePattern   = regexp.MustCompile(`^(?:ADR-)?([0-9]{3,4})-[a-z0-9-]+\.md$`)
	adrTitlePattern  = regexp.MustCompile(`(?m)^# ADR-([0-9]{3,4}): (.+)$`)
	adrStatusPattern = regexp.MustCompile(`(?m)^Status: (Accepted|Proposed|Deprecated|Superseded)(?: .*)?$`)
	adrIndexPattern  = regexp.MustCompile(`(?m)^\| \[([0-9]{3,4})\]\(([^)]+\.md)\) \| ([^|]+) \| (Accepted|Proposed|Deprecated|Superseded) \|$`)
	markdownLink     = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
)

type record struct {
	id       string
	filename string
	title    string
	status   string
}

func parseRecord(root, relative string, data []byte, maxLines int) (record, []Violation) {
	name := filepath.Base(relative)
	fileMatch := adrFilePattern.FindStringSubmatch(name)
	record := record{filename: name}
	if len(fileMatch) == 2 {
		record.id = fileMatch[1]
	}
	var violations []Violation
	titles := adrTitlePattern.FindAllStringSubmatch(string(data), -1)
	if len(titles) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one ADR heading, got %d", len(titles))})
	} else {
		record.title = strings.TrimSpace(titles[0][2])
		if titles[0][1] != record.id {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("heading ID %s does not match filename ID %s", titles[0][1], record.id)})
		}
	}
	statuses := adrStatusPattern.FindAllStringSubmatch(string(data), -1)
	if len(statuses) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one normalized Status line, got %d", len(statuses))})
	} else {
		record.status = statuses[0][1]
	}
	violations = append(violations, commonDocumentViolations(root, relative, data, maxLines)...)
	if record.status == "Superseded" && !hasADRSuccessorLink(data) {
		violations = append(violations, Violation{Path: relative, Reason: "Superseded record must link to another governed ADR"})
	}
	return record, violations
}

func parseAggregate(root, relative string, data []byte, maxLines int) []Violation {
	statuses := adrStatusPattern.FindAllStringSubmatch(string(data), -1)
	var violations []Violation
	if len(statuses) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one normalized Status line, got %d", len(statuses))})
	}
	violations = append(violations, commonDocumentViolations(root, relative, data, maxLines)...)
	if len(statuses) == 1 && statuses[0][1] == "Superseded" && !hasADRSuccessorLink(data) {
		violations = append(violations, Violation{Path: relative, Reason: "Superseded record must link to another governed ADR"})
	}
	return violations
}

func commonDocumentViolations(root, relative string, data []byte, maxLines int) []Violation {
	var violations []Violation
	if lines := strings.Count(string(data), "\n") + 1; lines > maxLines {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("%d lines exceeds the ADR line budget of %d", lines, maxLines)})
	}
	for _, target := range markdownTargets(data) {
		if !relativeLink(target) {
			continue
		}
		pathPart, _, _ := strings.Cut(target, "#")
		if pathPart == "" {
			continue
		}
		var targetPath string
		if trimmed, ok := strings.CutPrefix(pathPart, "/"); ok {
			targetPath = filepath.Join(root, filepath.FromSlash(trimmed))
		} else {
			targetPath = filepath.Join(root, filepath.Dir(filepath.FromSlash(relative)), filepath.FromSlash(pathPart))
		}
		if _, err := os.Stat(filepath.Clean(targetPath)); err != nil {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("relative link %q does not resolve", target)})
		}
	}
	return violations
}

func markdownTargets(data []byte) []string {
	matches := markdownLink.FindAllStringSubmatch(string(data), -1)
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		targets = append(targets, match[1])
	}
	return targets
}

func relativeLink(target string) bool {
	lower := strings.ToLower(target)
	return !strings.Contains(lower, "://") && !strings.HasPrefix(lower, "mailto:") && !strings.HasPrefix(lower, "#")
}

func hasADRSuccessorLink(data []byte) bool {
	for _, target := range markdownTargets(data) {
		pathPart, _, _ := strings.Cut(target, "#")
		base := filepath.Base(pathPart)
		if base == "ADR.md" || adrFilePattern.MatchString(base) {
			return true
		}
	}
	return false
}
