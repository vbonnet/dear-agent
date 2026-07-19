package instructionlint

import (
	"bytes"
	"fmt"
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

func parseScriptSegments(source []byte) []Segment {
	var segments []Segment
	var quote byte
	heredoc := ""
	for index, raw := range strings.Split(string(source), "\n") {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if heredoc != "" {
			if value == heredoc {
				heredoc = ""
				continue
			}
			segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
			continue
		}
		if quote != 0 {
			segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
			if unescapedByteCount(raw, quote)%2 == 1 {
				quote = 0
			}
			continue
		}
		if strings.HasPrefix(value, "#") {
			segments = append(segments, Segment{Kind: SegmentProse, Line: index + 1, Text: value})
			continue
		}
		if marker := scriptHeredocMarker(value); marker != "" {
			heredoc = marker
			continue
		}
		assignment := stripShellDeclaration(value)
		if shellAssignment.MatchString(assignment) && strings.Contains(assignment, "$(") {
			segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
			continue
		}
		if assignmentQuote := scriptAssignmentQuote(assignment); assignmentQuote != 0 {
			segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
			if unescapedByteCount(raw, assignmentQuote)%2 == 1 {
				quote = assignmentQuote
			}
			continue
		}
		for _, prefix := range []string{"echo ", "printf ", "emit "} {
			if strings.HasPrefix(value, prefix) {
				segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
				break
			}
		}
	}
	return segments
}

var shellAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var shellDeclaration = regexp.MustCompile(`^(?:local|export|readonly|typeset|declare)(?:\s+-[A-Za-z]+)*\s+`)
var heredocMarker = regexp.MustCompile(`<<-?['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

func stripShellDeclaration(value string) string {
	return shellDeclaration.ReplaceAllString(value, "")
}

func scriptAssignmentQuote(value string) byte {
	if !shellAssignment.MatchString(value) {
		return 0
	}
	for index := strings.IndexByte(value, '=') + 1; index < len(value); index++ {
		if value[index] == '\'' || value[index] == '"' {
			return value[index]
		}
	}
	return 0
}

func scriptHeredocMarker(value string) string {
	match := heredocMarker.FindStringSubmatch(value)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func unescapedByteCount(value string, target byte) int {
	count := 0
	escaped := false
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && !escaped {
			escaped = true
			continue
		}
		if value[index] == target && !escaped {
			count++
		}
		escaped = false
	}
	return count
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
		var visit func(*yaml.Node)
		visit = func(node *yaml.Node) {
			if node.Kind == yaml.ScalarNode {
				for _, match := range allowedBashTool.FindAllStringSubmatch(node.Value, -1) {
					command := strings.ReplaceAll(match[1], ":*", " *")
					segments = append(segments, Segment{Kind: SegmentShell, Line: node.Line + 1, Text: command})
				}
			}
			for _, child := range node.Content {
				visit(child)
			}
		}
		visit(value)
	}
	return segments
}

func parseYAMLSegments(source []byte) ([]Segment, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(source, &root); err != nil {
		return nil, err
	}
	var segments []Segment
	seenComments := map[string]bool{}
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
			kind := SegmentProse
			if !allowedBashTool.MatchString(node.Value) && commandShaped(node.Value) {
				kind = SegmentShell
			}
			segments = append(segments, Segment{Kind: kind, Line: node.Line, Text: node.Value})
			segments = append(segments, bashToolSegments(node.Value, node.Line)...)
		}
		segments = appendYAMLCommentSegments(segments, seenComments, node.HeadComment, node.Line-commentLineCount(node.HeadComment))
		segments = appendYAMLCommentSegments(segments, seenComments, node.LineComment, node.Line)
		segments = appendYAMLCommentSegments(segments, seenComments, node.FootComment, node.Line+1)
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(&root)
	return segments, nil
}

func bashToolSegments(value string, line int) []Segment {
	var segments []Segment
	for _, match := range allowedBashTool.FindAllStringSubmatch(value, -1) {
		command := strings.ReplaceAll(match[1], ":*", " *")
		segments = append(segments, Segment{Kind: SegmentShell, Line: line, Text: command})
	}
	return segments
}

func appendYAMLCommentSegments(segments []Segment, seen map[string]bool, comment string, startLine int) []Segment {
	if strings.TrimSpace(comment) == "" {
		return segments
	}
	if startLine < 1 {
		startLine = 1
	}
	for offset, raw := range strings.Split(comment, "\n") {
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
		if value == "" {
			continue
		}
		line := startLine + offset
		key := fmt.Sprintf("%d\x00%s", line, value)
		if seen[key] {
			continue
		}
		seen[key] = true
		segments = append(segments, Segment{Kind: SegmentProse, Line: line, Text: value})
		if commandShaped(value) {
			segments = append(segments, Segment{Kind: SegmentShell, Line: line, Text: value})
		}
		for _, match := range embeddedShellCommand.FindAllStringSubmatch(value, -1) {
			command := match[1]
			if command == "" {
				command = match[2]
			}
			if strings.TrimSpace(command) != "" {
				segments = append(segments, Segment{Kind: SegmentInline, Line: line, Text: command})
			}
		}
	}
	return segments
}

func commentLineCount(comment string) int {
	if comment == "" {
		return 0
	}
	return strings.Count(comment, "\n") + 1
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
	kind := SegmentProse
	language := strings.TrimSpace(string(block.Language(source)))
	if shellLanguage(language) {
		kind = SegmentShell
	}
	continuing := false
	for i := 0; i < block.Lines().Len(); i++ {
		segment := block.Lines().At(i)
		value := segment.Value(source)
		lineKind := kind
		if kind != SegmentShell && (continuing || commandShaped(string(value))) {
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
	if strings.Contains(value, "$(") {
		return true
	}
	if embeddedCommandStart.MatchString(strings.TrimSpace(value)) {
		return true
	}
	commands := splitShellCommands(value)
	for _, candidate := range commands {
		fields := parseShellWords(candidate)
		if len(fields) > 1 && fields[0] == "$" {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		command := executableBase(strings.TrimLeft(fields[0], "("))
		switch command {
		case "agm", "bash", "bd", "command", "dash", "env", "gh", "git", "gtimeout", "ksh", "nohup", "safe-merge", "safe-pr", "safe-push", "sh", "sudo", "timeout", "zsh":
			return true
		case "if", "while", "until", "then", "do", "!":
			return true
		}
	}
	return strings.Contains(value, "&&") || strings.Contains(value, "||")
}

var embeddedCommandStart = regexp.MustCompile(`(?:^|["':=\[(][[:space:]]*)((?:agm|bd|gh|git|safe-merge|safe-pr|safe-push)\b.*)$`)

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
