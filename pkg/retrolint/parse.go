package retrolint

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	dateHeaderRegex   = regexp.MustCompile(`(?mi)^(?:[*_#\s]*Date[*_#\s]*:)\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	filenameDateRegex = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	guardsHeaderRegex = regexp.MustCompile(`(?mi)^#{1,4}\s*Guards:?|^Guards:`)
	fencedCodeRegex   = regexp.MustCompile("(?s)```(?:ya?ml)?\\s*\\n(.*?)\\n```")
)

// ParseRetrospective parses a retrospective document and extracts metadata and guards.
func ParseRetrospective(ctx context.Context, r io.Reader, filename string) (*Retrospective, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading retrospective: %w", err)
	}
	content := string(data)
	retro := &Retrospective{
		Path: filename,
	}

	parseDateMetadata(retro, content, filename)

	// Try frontmatter YAML first
	fm, body, hasFM := extractFrontmatter(content)
	if hasFM {
		parseFrontmatterGuards(retro, fm)
	}

	// If no guards found in frontmatter, parse body
	if len(retro.Guards) == 0 {
		parseBodyGuards(retro, body)
	}

	return retro, nil
}

func parseDateMetadata(retro *Retrospective, content, filename string) {
	if m := dateHeaderRegex.FindStringSubmatch(content); len(m) > 1 {
		retro.Date = m[1]
	} else if base := filepath.Base(filename); base != "" {
		if m := filenameDateRegex.FindStringSubmatch(base); len(m) > 1 {
			retro.Date = m[1]
		}
	}
	if retro.Date != "" {
		if t, err := time.Parse("2006-01-02", retro.Date); err == nil {
			retro.ParsedDate = &t
		}
	}
}

func extractFrontmatter(content string) (string, string, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return "", content, false
	}
	rest := trimmed[3:]
	fm, body, found := strings.Cut(rest, "\n---")
	if !found {
		return "", content, false
	}
	body = strings.TrimPrefix(body, "\n")
	return fm, body, true
}

type frontmatterDoc struct {
	Date   string      `yaml:"date"`
	Guards []yaml.Node `yaml:"guards"`
}

func parseFrontmatterGuards(retro *Retrospective, fm string) {
	var doc frontmatterDoc
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return
	}
	if retro.Date == "" && doc.Date != "" {
		retro.Date = doc.Date
		if t, err := time.Parse("2006-01-02", doc.Date); err == nil {
			retro.ParsedDate = &t
		}
	}
	for _, node := range doc.Guards {
		g, ok := parseGuardNode(&node)
		if ok {
			retro.Guards = append(retro.Guards, g)
		}
	}
}

func parseBodyGuards(retro *Retrospective, body string) {
	section := extractGuardsSection(body)
	if section == "" {
		return
	}

	// Try fenced yaml in section
	if m := fencedCodeRegex.FindStringSubmatch(section); len(m) > 1 {
		if parseYAMLGuardsBlock(retro, m[1]) {
			return
		}
	}

	// Try parsing section directly as YAML list
	if parseYAMLGuardsBlock(retro, section) {
		return
	}

	// Fallback to line-by-line parsing
	parseLineGuards(retro, section)
}

func extractGuardsSection(body string) string {
	loc := guardsHeaderRegex.FindStringIndex(body)
	if loc == nil {
		return ""
	}
	after := body[loc[1]:]
	// Next section header terminates Guards section
	lines := strings.Split(after, "\n")
	var sectionLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(sectionLines) > 0 && strings.HasPrefix(trimmed, "## ") {
			break
		}
		sectionLines = append(sectionLines, line)
	}
	return strings.Join(sectionLines, "\n")
}

func parseYAMLGuardsBlock(retro *Retrospective, block string) bool {
	var nodes []yaml.Node
	if err := yaml.Unmarshal([]byte(block), &nodes); err == nil && len(nodes) > 0 {
		count := 0
		for _, n := range nodes {
			if g, ok := parseGuardNode(&n); ok {
				retro.Guards = append(retro.Guards, g)
				count++
			}
		}
		if count > 0 {
			return true
		}
	}
	// Also test if it's wrapped in `guards:`
	var wrapper struct {
		Guards []yaml.Node `yaml:"guards"`
	}
	if err := yaml.Unmarshal([]byte(block), &wrapper); err == nil && len(wrapper.Guards) > 0 {
		for _, n := range wrapper.Guards {
			if g, ok := parseGuardNode(&n); ok {
				retro.Guards = append(retro.Guards, g)
			}
		}
		return len(retro.Guards) > 0
	}
	return false
}

func parseGuardNode(n *yaml.Node) (Guard, bool) {
	var m map[string]any
	if err := n.Decode(&m); err != nil {
		return parseGuardString(n.Value)
	}
	return parseGuardMap(m)
}

func parseGuardMap(m map[string]any) (Guard, bool) {
	g := Guard{}
	if t, ok := m["type"].(string); ok {
		g.Type = GuardType(strings.ToLower(t))
	}
	if p, ok := m["path"].(string); ok {
		g.Path = p
	}
	if l, ok := m["label"].(string); ok {
		g.Label = l
	}
	if b, ok := m["bead"].(string); ok {
		g.Bead = b
	}
	if r, ok := m["reason"].(string); ok {
		g.Reason = r
	} else if r, ok := m["rationale"].(string); ok {
		g.Reason = r
	}

	if g.Type == "" {
		parseGuardShorthandMap(&g, m)
	}
	return g, g.Type != "" || g.Path != "" || g.Label != "" || g.Bead != ""
}

func parseGuardShorthandMap(g *Guard, m map[string]any) {
	for k, v := range m {
		valStr, _ := v.(string)
		switch strings.ToLower(k) {
		case "test":
			g.Type = GuardTypeTest
			g.Path = valStr
		case "file":
			g.Type = GuardTypeFile
			g.Path = valStr
		case "launchd":
			g.Type = GuardTypeLaunchd
			g.Label = valStr
		case "hook":
			g.Type = GuardTypeHook
			g.Path = valStr
		case "workflow":
			g.Type = GuardTypeWorkflow
			g.Path = valStr
		case "lint":
			g.Type = GuardTypeLint
			g.Path = valStr
		case "deferred":
			g.Type = GuardTypeDeferred
			parseDeferredValue(g, valStr, v)
		}
	}
}

func parseDeferredValue(g *Guard, strVal string, rawVal any) {
	if defMap, ok := rawVal.(map[string]any); ok {
		if b, ok := defMap["bead"].(string); ok {
			g.Bead = b
		}
		if r, ok := defMap["reason"].(string); ok {
			g.Reason = r
		}
		return
	}
	if strVal != "" {
		parseDeferredString(g, strVal)
	}
}

func parseDeferredString(g *Guard, s string) {
	s = strings.TrimSpace(s)
	// Match bead followed by reason in parens: "ce-12345 (Need release)"
	if idx := strings.Index(s, "("); idx > 0 && strings.HasSuffix(s, ")") {
		g.Bead = strings.TrimSpace(s[:idx])
		g.Reason = strings.TrimSpace(s[idx+1 : len(s)-1])
		return
	}
	// Match bead - reason: "ce-12345 - Need release"
	if parts := strings.SplitN(s, " - ", 2); len(parts) == 2 {
		g.Bead = strings.TrimSpace(parts[0])
		g.Reason = strings.TrimSpace(parts[1])
		return
	}
	// Match bead: reason
	if parts := strings.SplitN(s, ": ", 2); len(parts) == 2 {
		g.Bead = strings.TrimSpace(parts[0])
		g.Reason = strings.TrimSpace(parts[1])
		return
	}
	g.Bead = s
}

func parseLineGuards(retro *Retrospective, section string) {
	sc := bufio.NewScanner(strings.NewReader(section))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") {
			continue
		}
		item := strings.TrimSpace(line[2:])
		if g, ok := parseGuardString(item); ok {
			retro.Guards = append(retro.Guards, g)
		}
	}
}

func parseGuardString(item string) (Guard, bool) {
	item = strings.Trim(item, "`\"'")
	parts := strings.SplitN(item, ":", 2)
	if len(parts) != 2 {
		return Guard{}, false
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	val := strings.TrimSpace(parts[1])
	val = strings.Trim(val, "`\"'")

	g := Guard{}
	switch key {
	case "test":
		g.Type = GuardTypeTest
		g.Path = val
	case "file":
		g.Type = GuardTypeFile
		g.Path = val
	case "launchd":
		g.Type = GuardTypeLaunchd
		g.Label = val
	case "hook":
		g.Type = GuardTypeHook
		g.Path = val
	case "workflow":
		g.Type = GuardTypeWorkflow
		g.Path = val
	case "lint":
		g.Type = GuardTypeLint
		g.Path = val
	case "deferred":
		g.Type = GuardTypeDeferred
		parseDeferredString(&g, val)
	default:
		return Guard{}, false
	}
	return g, true
}
