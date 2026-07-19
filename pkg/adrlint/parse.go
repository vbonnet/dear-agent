package adrlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	adrFilePattern              = regexp.MustCompile(`^(?:ADR-)?([0-9]{3,4})-[a-z0-9-]+\.md$`)
	adrTitlePattern             = regexp.MustCompile(`(?m)^# ADR-([0-9]{3,4}): (.+)$`)
	adrStatusPattern            = regexp.MustCompile(`(?m)^Status: (Accepted|Proposed|Deprecated|Superseded)(?: .*)?$`)
	adrStatusLine               = regexp.MustCompile(`(?m)^Status:.*$`)
	adrIndexPattern             = regexp.MustCompile(`(?m)^\| \[([0-9]{3,4})\]\(([^)]+\.md)\) \| ([^|]+) \| (Accepted|Proposed|Deprecated|Superseded) \|$`)
	adrLikePrefix               = regexp.MustCompile(`(?i)^(?:adr[-_ ]?[0-9]{1,4}(?:[^0-9]|$)|(?:[0-9]{1,3}|0[0-9]{3})(?:[^0-9]|$))`)
	markdownLink                = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	markdownReferenceDefinition = regexp.MustCompile(`(?m)^[ \t]{0,3}\[[^]]+\]:[ \t]+<?([^> \t]+)>?(?:[ \t]+.*)?$`)
)

type record struct {
	id       string
	filename string
	title    string
	status   string
}

func parseRecord(root, relative string, data []byte, governed map[string]bool) (record, []Violation) {
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
	statusLines := adrStatusLine.FindAll(data, -1)
	if len(statusLines) != 1 || len(statuses) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one normalized Status line, got %d status-like line(s)", len(statusLines))})
	} else {
		record.status = statuses[0][1]
	}
	violations = append(violations, commonDocumentViolations(root, relative, data)...)
	if record.status == "Superseded" && !hasADRSuccessorLink(root, relative, []byte(statuses[0][0]), governed) {
		violations = append(violations, Violation{Path: relative, Reason: "Superseded record must link to another governed ADR"})
	}
	return record, violations
}

func parseAggregate(root, relative string, data []byte, governed map[string]bool) []Violation {
	statuses := adrStatusPattern.FindAllStringSubmatch(string(data), -1)
	statusLines := adrStatusLine.FindAll(data, -1)
	var violations []Violation
	if len(statusLines) != 1 || len(statuses) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one normalized Status line, got %d status-like line(s)", len(statusLines))})
	}
	violations = append(violations, commonDocumentViolations(root, relative, data)...)
	if len(statuses) == 1 && statuses[0][1] == "Superseded" && !hasADRSuccessorLink(root, relative, []byte(statuses[0][0]), governed) {
		violations = append(violations, Violation{Path: relative, Reason: "Superseded record must link to another governed ADR"})
	}
	return violations
}

func commonDocumentViolations(root, relative string, data []byte) []Violation {
	var violations []Violation
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
	matches = append(matches, markdownReferenceDefinition.FindAllStringSubmatch(string(data), -1)...)
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		targets = append(targets, markdownLinkDestination(match[1]))
	}
	return targets
}

func markdownLinkDestination(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<") {
		if end := strings.Index(raw, ">"); end > 0 {
			return raw[1:end]
		}
	}
	if end := strings.IndexAny(raw, " \t\r\n"); end >= 0 {
		return raw[:end]
	}
	return raw
}

func relativeLink(target string) bool {
	lower := strings.ToLower(target)
	return !strings.Contains(lower, "://") && !strings.HasPrefix(lower, "mailto:") && !strings.HasPrefix(lower, "#")
}

func hasADRSuccessorLink(root, relative string, statusLine []byte, governed map[string]bool) bool {
	statuses := adrStatusPattern.FindAllStringSubmatch(string(statusLine), -1)
	if len(statuses) != 1 || statuses[0][1] != "Superseded" {
		return false
	}
	for _, target := range markdownTargets(statusLine) {
		if isLiveADRSuccessor(root, relative, target, governed) {
			return true
		}
	}
	return false
}

func isLiveADRSuccessor(root, relative, target string, governed map[string]bool) bool {
	if !relativeLink(target) {
		return false
	}
	pathPart, _, _ := strings.Cut(target, "#")
	base := filepath.Base(pathPart)
	if pathPart == "" || (base != "ADR.md" && !adrFilePattern.MatchString(base)) {
		return false
	}
	successor := filepath.Join(root, filepath.Dir(filepath.FromSlash(relative)), filepath.FromSlash(pathPart))
	if trimmed, ok := strings.CutPrefix(pathPart, "/"); ok {
		successor = filepath.Join(root, filepath.FromSlash(trimmed))
	}
	successor = filepath.Clean(successor)
	current := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	inside, err := filepath.Rel(root, successor)
	if err != nil || successor == current || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return false
	}
	governedPath := filepath.ToSlash(inside)
	if !governed[governedPath] {
		return false
	}
	data, err := os.ReadFile(successor)
	if err != nil {
		return false
	}
	statuses := adrStatusPattern.FindAllStringSubmatch(string(data), -1)
	return len(statuses) == 1 && (statuses[0][1] == "Accepted" || statuses[0][1] == "Proposed")
}
