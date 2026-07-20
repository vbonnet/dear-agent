package instructionlint

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
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
	segments = append(segments, proseCommandSegments(source, classified)...)
	frontmatter := markdownFrontmatterCommands(source)
	for _, command := range frontmatter {
		segments = slices.DeleteFunc(segments, func(segment Segment) bool {
			return segment.Line == command.Line && segment.Kind == SegmentShell && allowedBashTool.MatchString(segment.Text)
		})
	}
	segments = append(segments, frontmatter...)
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].Line != segments[j].Line {
			return segments[i].Line < segments[j].Line
		}
		if segments[i].Kind != segments[j].Kind {
			return segments[i].Kind < segments[j].Kind
		}
		return segments[i].Text < segments[j].Text
	})
	return slices.CompactFunc(segments, func(left, right Segment) bool {
		return left.Kind == right.Kind && left.Line == right.Line && left.Text == right.Text
	})
}

func parseScriptSegments(source []byte) []Segment {
	var segments []Segment
	var quote byte
	heredoc := ""
	visibleContinuation := false
	visibleHelpers := agentVisibleScriptHelpers(source)
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
		if visibleContinuation {
			segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
			if commandQuote := unclosedScriptQuote(value); commandQuote != 0 {
				quote = commandQuote
				visibleContinuation = false
			} else {
				visibleContinuation = hasShellLineContinuation(raw)
			}
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
		if agentVisibleScriptCommand(value, visibleHelpers) {
			segments = append(segments, Segment{Kind: SegmentShell, Line: index + 1, Text: value})
			if commandQuote := unclosedScriptQuote(value); commandQuote != 0 {
				quote = commandQuote
			} else {
				visibleContinuation = hasShellLineContinuation(raw)
			}
		}
	}
	return segments
}

func hasShellLineContinuation(value string) bool {
	backslashes := 0
	for index := len(value) - 1; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func agentVisibleScriptCommand(value string, helpers map[string]bool) bool {
	for _, command := range splitShellCommands(value) {
		fields := stripCommandPrefixes(parseShellWords(command))
		if len(fields) > 0 && (slices.Contains([]string{"echo", "emit", "jq", "printf"}, fields[0]) || helpers[fields[0]]) {
			return true
		}
	}
	return false
}

var shellAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var shellDeclaration = regexp.MustCompile(`^(?:local|export|readonly|typeset|declare)(?:\s+-[A-Za-z]+)*\s+`)
var heredocMarker = regexp.MustCompile(`<<-?['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)
var shellFunction = regexp.MustCompile(`^(?:function\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*\(\))?|([A-Za-z_][A-Za-z0-9_]*)\s*\(\))\s*\{`)

func agentVisibleScriptHelpers(source []byte) map[string]bool {
	functions := scriptFunctions(source)
	visible := map[string]bool{}
	changed := true
	for changed {
		changed = false
		for name, body := range functions {
			if visible[name] {
				continue
			}
			for _, line := range body {
				if agentVisibleScriptCommand(line, visible) {
					visible[name] = true
					changed = true
					break
				}
			}
		}
	}
	return visible
}

func scriptFunctions(source []byte) map[string][]string {
	lines := strings.Split(string(source), "\n")
	functions := map[string][]string{}
	for index := 0; index < len(lines); index++ {
		value := strings.TrimSpace(lines[index])
		match := shellFunction.FindStringSubmatchIndex(value)
		if len(match) == 0 {
			continue
		}
		nameStart, nameEnd := match[2], match[3]
		if nameStart < 0 {
			nameStart, nameEnd = match[4], match[5]
		}
		name := value[nameStart:nameEnd]
		body := []string{strings.TrimSpace(value[match[1]:])}
		depth := shellBraceDelta(value)
		for depth > 0 && index+1 < len(lines) {
			index++
			line := strings.TrimSpace(lines[index])
			body = append(body, line)
			depth += shellBraceDelta(line)
		}
		functions[name] = body
	}
	return functions
}

func shellBraceDelta(value string) int {
	delta := 0
	var quote byte
	escaped := false
	for index := 0; index < len(value); index++ {
		current := value[index]
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == '#' && (index == 0 || value[index-1] == ' ' || value[index-1] == '\t') {
			break
		}
		switch current {
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

func unclosedScriptQuote(value string) byte {
	state := shellScanState{}
	for index := 0; index < len(value); index++ {
		if width := state.boundaryWidth(value, index); width > 1 {
			index += width - 1
		}
	}
	return state.quote
}

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
	var visit func(*yaml.Node, bool)
	visit = func(node *yaml.Node, mappingKey bool) {
		if !mappingKey && node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
			segments = append(segments, yamlScalarSegments(source, node)...)
		}
		segments = appendYAMLCommentSegments(segments, seenComments, node.HeadComment, node.Line-commentLineCount(node.HeadComment))
		segments = appendYAMLCommentSegments(segments, seenComments, node.LineComment, node.Line)
		segments = appendYAMLCommentSegments(segments, seenComments, node.FootComment, node.Line+1)
		for index, child := range node.Content {
			visit(child, node.Kind == yaml.MappingNode && index%2 == 0)
		}
	}
	visit(&root, false)
	return segments, nil
}

func yamlScalarSegments(source []byte, node *yaml.Node) []Segment {
	if node.Style == yaml.FoldedStyle || node.Style == yaml.LiteralStyle {
		return yamlBlockScalarSegments(source, node)
	}
	if physical := yamlFlowScalarLines(source, node); len(physical) > 1 {
		var segments []Segment
		for _, line := range physical {
			segments = append(segments, yamlValueSegments(line.text, line.number)...)
		}
		return segments
	}
	var segments []Segment
	for offset, raw := range strings.Split(node.Value, "\n") {
		segments = append(segments, yamlValueSegments(raw, node.Line+offset)...)
	}
	return segments
}

type yamlPhysicalLine struct {
	number int
	text   string
}

func yamlFlowScalarLines(source []byte, node *yaml.Node) []yamlPhysicalLine {
	lines := strings.Split(string(source), "\n")
	start := node.Line - 1
	column := node.Column - 1
	if start < 0 || start >= len(lines) || column < 0 || column >= len(lines[start]) {
		return nil
	}
	headerIndent := leadingSpaces(lines[start])
	physical := []yamlPhysicalLine{{number: node.Line, text: strings.Trim(lines[start][column:], " '\"")}}
	for index := start + 1; index < len(lines); index++ {
		raw := lines[index]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if leadingSpaces(raw) <= headerIndent {
			break
		}
		physical = append(physical, yamlPhysicalLine{number: index + 1, text: strings.Trim(strings.TrimSpace(raw), "'\"")})
	}
	return physical
}

func yamlValueSegments(raw string, line int) []Segment {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	kind := SegmentProse
	if !allowedBashTool.MatchString(value) && commandShaped(value) {
		kind = SegmentShell
	}
	segments := []Segment{{Kind: kind, Line: line, Text: value}}
	segments = append(segments, bashToolSegments(value, line)...)
	return appendEmbeddedProseSegments(segments, value, line)
}

func yamlBlockScalarSegments(source []byte, node *yaml.Node) []Segment {
	lines := strings.Split(string(source), "\n")
	header := node.Line - 1
	if header < 0 || header >= len(lines) {
		return nil
	}
	headerIndent := leadingSpaces(lines[header])
	var segments []Segment
	for index := header + 1; index < len(lines); index++ {
		raw := lines[index]
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if leadingSpaces(raw) <= headerIndent {
			break
		}
		line := index + 1
		segments = append(segments, yamlValueSegments(value, line)...)
	}
	return segments
}

func leadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
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
		segments = appendEmbeddedProseSegments(segments, value, line)
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

func appendEmbeddedProseSegments(segments []Segment, value string, line int) []Segment {
	for _, match := range embeddedProseCommand.FindAllStringSubmatch(value, -1) {
		if len(match) == 2 {
			segments = append(segments, Segment{Kind: SegmentInline, Line: line, Text: strings.TrimSpace(match[1])})
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
	value = stripMarkdownContainerPrefixes(value)
	if strings.Contains(value, "$(") {
		return true
	}
	if embeddedCommandStart.MatchString(strings.TrimSpace(value)) {
		return true
	}
	commands := splitShellCommands(value)
	for _, candidate := range commands {
		fields := trimShellGroupPrefixes(parseShellWords(candidate))
		if len(fields) > 1 && fields[0] == "$" {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		command := executableBase(strings.TrimLeft(fields[0], "("))
		switch command {
		case "agm", "bash", "bd", "command", "dash", "env", "exec", "gh", "git", "gtimeout", "ksh", "nohup", "safe-merge", "safe-pr", "safe-push", "sh", "sudo", "timeout", "zsh":
			return true
		case "if", "while", "until", "then", "do", "!":
			return true
		}
	}
	return strings.Contains(value, "&&") || strings.Contains(value, "||")
}

var embeddedCommandStart = regexp.MustCompile(`(?:^|["':=\[(][[:space:]]*)((?:agm|bd|gh|git|safe-merge|safe-pr|safe-push)\b.*)$`)
var embeddedProseCommand = regexp.MustCompile(`(?i)\b(?:run|use|execute|invoke|call|try)\s+((?:agm|bd|gh|git|safe-merge|safe-pr|safe-push)\b.*)$`)

func proseCommandSegments(source []byte, classified map[int]SegmentKind) []Segment {
	var segments []Segment
	for index, raw := range strings.Split(string(source), "\n") {
		line := index + 1
		if _, ok := classified[line]; ok {
			continue
		}
		for _, match := range embeddedProseCommand.FindAllStringSubmatch(raw, -1) {
			if len(match) == 2 {
				segments = append(segments, Segment{Kind: SegmentInline, Line: line, Text: strings.TrimSpace(match[1])})
			}
		}
	}
	return segments
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
			if commandShaped(normalized) {
				kind = SegmentShell
			}
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
