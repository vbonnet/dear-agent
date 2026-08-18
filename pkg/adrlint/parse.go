package adrlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var (
	adrFilePattern              = regexp.MustCompile(`^(?:ADR-)?([0-9]{3,4})-[a-z0-9-]+\.md$`)
	adrTitlePattern             = regexp.MustCompile(`(?m)^# ADR-([0-9]{3,4}): (.+)$`)
	adrStatusPattern            = regexp.MustCompile(`(?m)^Status: (Accepted|Proposed|Deprecated|Superseded)(?: .*)?$`)
	adrStatusLine               = regexp.MustCompile(`(?m)^Status:.*$`)
	adrIndexPattern             = regexp.MustCompile(`(?m)^\| \[([0-9]{3,4})\]\(([^)]+\.md)\) \| ([^|]+) \| (Accepted|Proposed|Deprecated|Superseded) \|$`)
	adrLikePrefix               = regexp.MustCompile(`(?i)^(?:adr[-_ ]?[0-9]+(?:[^0-9]|$)|(?:[0-9]{1,3}|0[0-9]{3})(?:[^0-9]|$))`)
	markdownLink                = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	markdownReferenceLink       = regexp.MustCompile(`\[([^]]+)\]\[([^]]*)\]`)
	markdownReferenceDefinition = regexp.MustCompile(`(?m)^[ \t]{0,3}\[([^]]+)\]:[ \t]+<?([^> \t]+)>?(?:[ \t]+.*)?$`)
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
	rendered := markdownOutsideCode(data)
	titles := adrTitlePattern.FindAllStringSubmatch(string(rendered), -1)
	if len(titles) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one ADR heading, got %d", len(titles))})
	} else {
		record.title = strings.TrimSpace(titles[0][2])
		if titles[0][1] != record.id {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("heading ID %s does not match filename ID %s", titles[0][1], record.id)})
		}
	}
	statuses := adrStatusPattern.FindAllStringSubmatch(string(rendered), -1)
	statusLines := adrStatusLine.FindAll(rendered, -1)
	if len(statusLines) != 1 || len(statuses) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one normalized Status line, got %d status-like line(s)", len(statusLines))})
	} else {
		record.status = statuses[0][1]
	}
	violations = append(violations, commonDocumentViolations(root, relative, data)...)
	if record.status == "Superseded" && !hasADRSuccessorLink(root, relative, []byte(statuses[0][0]), data, governed) {
		violations = append(violations, Violation{Path: relative, Reason: "Superseded record must link to another governed ADR"})
	}
	return record, violations
}

func parseAggregate(root, relative string, data []byte, governed map[string]bool) []Violation {
	rendered := markdownOutsideCode(data)
	statuses := adrStatusPattern.FindAllStringSubmatch(string(rendered), -1)
	statusLines := adrStatusLine.FindAll(rendered, -1)
	var violations []Violation
	if len(statusLines) != 1 || len(statuses) != 1 {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("want one normalized Status line, got %d status-like line(s)", len(statusLines))})
	}
	violations = append(violations, commonDocumentViolations(root, relative, data)...)
	if len(statuses) == 1 && statuses[0][1] == "Superseded" && !hasADRSuccessorLink(root, relative, []byte(statuses[0][0]), data, governed) {
		violations = append(violations, Violation{Path: relative, Reason: "Superseded record must link to another governed ADR"})
	}
	return violations
}

func commonDocumentViolations(root, relative string, data []byte) []Violation {
	var violations []Violation
	for _, label := range undefinedMarkdownReferences(data) {
		violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("undefined reference-style link label %q", label)})
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
		targetPath = filepath.Clean(targetPath)
		inside, err := filepath.Rel(root, targetPath)
		if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("relative link %q escapes repository", target)})
			continue
		}
		if _, err := os.Stat(targetPath); err != nil {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("relative link %q does not resolve", target)})
		}
	}
	return violations
}

func undefinedMarkdownReferences(data []byte) []string {
	data = markdownOutsideCode(data)
	definitions := make(map[string]bool)
	for _, match := range markdownReferenceDefinition.FindAllStringSubmatch(string(data), -1) {
		definitions[normalizeReferenceLabel(match[1])] = true
	}
	missing := make(map[string]string)
	for _, match := range markdownReferenceLink.FindAllStringSubmatch(string(data), -1) {
		rawLabel := referenceLinkLabel(match)
		label := normalizeReferenceLabel(rawLabel)
		if !definitions[label] {
			missing[label] = strings.TrimSpace(rawLabel)
		}
	}
	labels := make([]string, 0, len(missing))
	for _, label := range missing {
		labels = append(labels, label)
	}
	slices.Sort(labels)
	return labels
}

func referenceLinkLabel(match []string) string {
	if match[2] != "" {
		return match[2]
	}
	return match[1]
}

func normalizeReferenceLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

func markdownTargets(data []byte) []string {
	data = markdownOutsideCode(data)
	inlineMatches := markdownLink.FindAllStringSubmatch(string(data), -1)
	referenceDefinitions := markdownReferenceDefinition.FindAllStringSubmatch(string(data), -1)
	targets := make([]string, 0, len(inlineMatches)+len(referenceDefinitions))
	for _, match := range inlineMatches {
		targets = append(targets, markdownLinkDestination(match[1]))
	}
	for _, match := range referenceDefinitions {
		targets = append(targets, match[2])
	}
	return targets
}

func markdownOutsideCode(data []byte) []byte {
	masked := append([]byte(nil), data...)
	document := goldmark.DefaultParser().Parse(text.NewReader(data))
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch current := node.(type) {
		case *ast.CodeBlock:
			maskMarkdownSegments(masked, current.Lines())
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			maskMarkdownSegments(masked, current.Lines())
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			for child := current.FirstChild(); child != nil; child = child.NextSibling() {
				if textNode, ok := child.(*ast.Text); ok {
					maskMarkdownSegment(masked, textNode.Segment)
				}
			}
			return ast.WalkSkipChildren, nil
		default:
			return ast.WalkContinue, nil
		}
	})
	return masked
}

func maskMarkdownSegments(data []byte, segments *text.Segments) {
	for index := 0; index < segments.Len(); index++ {
		maskMarkdownSegment(data, segments.At(index))
	}
}

func maskMarkdownSegment(data []byte, segment text.Segment) {
	for index := segment.Start; index < segment.Stop && index < len(data); index++ {
		if data[index] != '\n' && data[index] != '\r' {
			data[index] = ' '
		}
	}
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

func hasADRSuccessorLink(root, relative string, statusLine, document []byte, governed map[string]bool) bool {
	statuses := adrStatusPattern.FindAllStringSubmatch(string(statusLine), -1)
	if len(statuses) != 1 || statuses[0][1] != "Superseded" {
		return false
	}
	for _, target := range successorTargets(statusLine, document) {
		if isLiveADRSuccessor(root, relative, target, governed) {
			return true
		}
	}
	return false
}

func successorTargets(statusLine, document []byte) []string {
	targets := markdownTargets(statusLine)
	document = markdownOutsideCode(document)
	definitions := make(map[string]string)
	for _, match := range markdownReferenceDefinition.FindAllStringSubmatch(string(document), -1) {
		definitions[normalizeReferenceLabel(match[1])] = match[2]
	}
	for _, match := range markdownReferenceLink.FindAllStringSubmatch(string(statusLine), -1) {
		if target, ok := definitions[normalizeReferenceLabel(referenceLinkLabel(match))]; ok {
			targets = append(targets, target)
		}
	}
	return targets
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
	data = markdownOutsideCode(data)
	statuses := adrStatusPattern.FindAllStringSubmatch(string(data), -1)
	return len(statuses) == 1 && (statuses[0][1] == "Accepted" || statuses[0][1] == "Proposed")
}
