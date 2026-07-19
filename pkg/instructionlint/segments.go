package instructionlint

import (
	"bytes"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// SegmentKind distinguishes prose from command-shaped Markdown content.
type SegmentKind string

const (
	// SegmentProse is ordinary Markdown outside executable fences.
	SegmentProse SegmentKind = "prose"
	// SegmentInline is inline code that may contain a command form.
	SegmentInline SegmentKind = "inline"
	// SegmentShell is content in a shell-language fenced block.
	SegmentShell SegmentKind = "shell"
)

// Segment is one normalized, line-addressable policy input.
type Segment struct {
	Kind SegmentKind
	Line int
	Text string
}

func parseSegments(source []byte) []Segment {
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	classified, inline := classifyMarkdown(root, source)
	segments := joinShellContinuations(sourceSegments(source, classified))
	segments = append(segments, inline...)
	segments = append(segments, markdownFrontmatterCommands(source)...)
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].Line != segments[j].Line {
			return segments[i].Line < segments[j].Line
		}
		if segments[i].Kind != segments[j].Kind {
			return segments[i].Kind < segments[j].Kind
		}
		return segments[i].Text < segments[j].Text
	})
	return segments
}

var allowedBashTool = regexp.MustCompile(`Bash\(([^)]*)\)`)

func markdownFrontmatterCommands(source []byte) []Segment {
	if !bytes.HasPrefix(source, []byte("---\n")) {
		return nil
	}
	end := bytes.Index(source[4:], []byte("\n---\n"))
	if end < 0 {
		return nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal(source[4:4+end], &root); err != nil || len(root.Content) == 0 {
		return nil
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	var segments []Segment
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != "allowed-tools" {
			continue
		}
		value := mapping.Content[i+1]
		for _, match := range allowedBashTool.FindAllStringSubmatch(value.Value, -1) {
			command := strings.ReplaceAll(match[1], ":*", " *")
			segments = append(segments, Segment{Kind: SegmentShell, Line: value.Line + 1, Text: command})
		}
	}
	return segments
}

func parseYAMLSegments(source []byte) ([]Segment, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(source, &root); err != nil {
		return nil, err
	}
	var segments []Segment
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
			kind := SegmentProse
			if commandShaped(node.Value) {
				kind = SegmentShell
			}
			segments = append(segments, Segment{Kind: kind, Line: node.Line, Text: node.Value})
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(&root)
	return segments, nil
}

func classifyMarkdown(root ast.Node, source []byte) (map[int]SegmentKind, []Segment) {
	classified := map[int]SegmentKind{}
	var inline []Segment
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.FencedCodeBlock:
			classifyFence(classified, typed, source)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			classifyIndentedCodeBlock(classified, typed, source)
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			if segment, ok := codeSpanSegment(typed, source); ok {
				inline = append(inline, segment)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return classified, inline
}

func classifyIndentedCodeBlock(classified map[int]SegmentKind, block *ast.CodeBlock, source []byte) {
	continuing := false
	for i := 0; i < block.Lines().Len(); i++ {
		segment := block.Lines().At(i)
		value := string(segment.Value(source))
		kind := SegmentKind("skip")
		if continuing || commandShaped(value) {
			kind = SegmentShell
		}
		classifySourceLines(classified, segment, source, kind)
		continuing = kind == SegmentShell && shellContinues(value)
	}
}

func classifyFence(classified map[int]SegmentKind, block *ast.FencedCodeBlock, source []byte) {
	kind := SegmentKind("skip")
	language := strings.TrimSpace(string(block.Language(source)))
	if shellLanguage(language) {
		kind = SegmentShell
	}
	continuing := false
	for i := 0; i < block.Lines().Len(); i++ {
		segment := block.Lines().At(i)
		value := segment.Value(source)
		lineKind := kind
		if language == "" && (continuing || commandShaped(string(value))) {
			lineKind = SegmentShell
		}
		classifySourceLines(classified, segment, source, lineKind)
		continuing = lineKind == SegmentShell && shellContinues(string(value))
	}
}

func classifySourceLines(classified map[int]SegmentKind, segment text.Segment, source []byte, kind SegmentKind) {
	startLine := sourceLine(source, segment.Start)
	value := segment.Value(source)
	lineCount := bytes.Count(value, []byte{'\n'})
	if !bytes.HasSuffix(value, []byte{'\n'}) {
		lineCount++
	}
	for offset := 0; offset < lineCount; offset++ {
		classified[startLine+offset] = kind
	}
}

func commandShaped(value string) bool {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return false
	}
	if fields[0] == "$" && len(fields) > 1 {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return false
	}
	command := strings.TrimLeft(fields[0], "(")
	switch command {
	case "agm", "bd", "command", "env", "gh", "git", "gtimeout", "nohup", "safe-merge", "safe-pr", "safe-push", "sudo", "timeout":
		return true
	case "if", "while", "until", "then", "do", "!":
		return true
	default:
		return strings.Contains(value, "&&") || strings.Contains(value, "||")
	}
}

func codeSpanSegment(span *ast.CodeSpan, source []byte) (Segment, bool) {
	var value strings.Builder
	line := 1
	for child := span.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		if value.Len() == 0 {
			line = sourceLine(source, textNode.Segment.Start)
		}
		value.Write(textNode.Segment.Value(source))
	}
	normalized := strings.TrimSpace(value.String())
	return Segment{Kind: SegmentInline, Line: line, Text: normalized}, normalized != ""
}

func sourceSegments(source []byte, classified map[int]SegmentKind) []Segment {
	lines := strings.Split(string(source), "\n")
	segments := make([]Segment, 0, len(lines))
	for i, raw := range lines {
		line := i + 1
		normalized := strings.TrimSpace(raw)
		if strings.HasPrefix(normalized, "```") || strings.HasPrefix(normalized, "~~~") {
			continue
		}
		kind, classifiedLine := classified[line]
		if classifiedLine && kind == "skip" {
			continue
		}
		if !classifiedLine {
			kind = SegmentProse
		}
		if normalized != "" {
			segments = append(segments, Segment{Kind: kind, Line: line, Text: normalized})
		}
	}
	return segments
}

func joinShellContinuations(segments []Segment) []Segment {
	joined := make([]Segment, 0, len(segments))
	for i := 0; i < len(segments); i++ {
		current := segments[i]
		if current.Kind != SegmentShell || !shellContinues(current.Text) {
			joined = append(joined, current)
			continue
		}

		var command strings.Builder
		command.WriteString(trimShellContinuation(current.Text))
		lastLine := current.Line
		for i+1 < len(segments) && segments[i+1].Kind == SegmentShell && segments[i+1].Line == lastLine+1 {
			i++
			next := segments[i]
			command.WriteByte(' ')
			command.WriteString(trimShellContinuation(next.Text))
			lastLine = next.Line
			if !shellContinues(next.Text) {
				break
			}
		}
		current.Text = strings.Join(strings.Fields(command.String()), " ")
		joined = append(joined, current)
	}
	return joined
}

func shellContinues(value string) bool {
	return strings.HasSuffix(strings.TrimSpace(value), `\`)
}

func trimShellContinuation(value string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), `\`))
}

func shellLanguage(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "bash", "console", "sh", "shell", "zsh":
		return true
	default:
		return false
	}
}

func sourceLine(source []byte, offset int) int {
	if offset < 0 {
		return 1
	}
	if offset > len(source) {
		offset = len(source)
	}
	return bytes.Count(source[:offset], []byte{'\n'}) + 1
}
