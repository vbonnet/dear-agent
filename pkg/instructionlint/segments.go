package instructionlint

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

func parseGoPromptSegments(source []byte) ([]segment, error) {
	files := token.NewFileSet()
	root, err := parser.ParseFile(files, "instruction-surface.go", source, 0)
	if err != nil {
		return nil, err
	}
	var segments []segment
	inspected := map[*goast.BasicLit]bool{}
	goast.Inspect(root, func(node goast.Node) bool {
		expression, ok := node.(*goast.BinaryExpr)
		if !ok || expression.Op != token.ADD {
			return true
		}
		var literals []*goast.BasicLit
		collectGoStringLiterals(expression, &literals)
		if !slices.ContainsFunc(literals, goLiteralIsMultiline) {
			return true
		}
		for _, literal := range literals {
			segments = append(segments, goLiteralSegments(files, literal)...)
			inspected[literal] = true
		}
		return false
	})
	goast.Inspect(root, func(node goast.Node) bool {
		literal, ok := node.(*goast.BasicLit)
		if !ok || inspected[literal] || !goLiteralIsMultiline(literal) {
			return true
		}
		segments = append(segments, goLiteralSegments(files, literal)...)
		return true
	})
	return segments, nil
}

func collectGoStringLiterals(expression goast.Expr, literals *[]*goast.BasicLit) {
	switch typed := expression.(type) {
	case *goast.BasicLit:
		if typed.Kind == token.STRING {
			*literals = append(*literals, typed)
		}
	case *goast.BinaryExpr:
		if typed.Op == token.ADD {
			collectGoStringLiterals(typed.X, literals)
			collectGoStringLiterals(typed.Y, literals)
		}
	case *goast.ParenExpr:
		collectGoStringLiterals(typed.X, literals)
	}
}

func goLiteralIsMultiline(literal *goast.BasicLit) bool {
	value, err := strconv.Unquote(literal.Value)
	return err == nil && strings.Contains(value, "\n")
}

func goLiteralSegments(files *token.FileSet, literal *goast.BasicLit) []segment {
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return nil
	}
	startLine := files.Position(literal.Pos()).Line
	segments := parseSegments([]byte(value))
	for index := range segments {
		segments[index].Line += startLine - 1
	}
	return segments
}

// segmentKind distinguishes prose from command-shaped Markdown content.
type segmentKind string

const (
	// segmentProse is ordinary Markdown outside executable fences.
	segmentProse segmentKind = "prose"
	// segmentInline is inline code that may contain a command form.
	segmentInline segmentKind = "inline"
	// segmentShell is content in a shell-language fenced block.
	segmentShell segmentKind = "shell"
)

// segment is one normalized, line-addressable policy input.
type segment struct {
	Kind segmentKind
	Line int
	Text string
}

func parseSegments(source []byte) []segment {
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	classified, inline := classifyMarkdown(root, source)
	segments := joinShellContinuations(sourceSegments(source, classified))
	segments = append(segments, inline...)
	segments = append(segments, proseCommandSegments(source, classified)...)
	frontmatter := markdownFrontmatterCommands(source)
	for _, command := range frontmatter {
		segments = slices.DeleteFunc(segments, func(segment segment) bool {
			return segment.Line == command.Line && segment.Kind == segmentShell && allowedBashTool.MatchString(segment.Text)
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
	return slices.CompactFunc(segments, func(left, right segment) bool {
		return left.Kind == right.Kind && left.Line == right.Line && left.Text == right.Text
	})
}

var allowedBashTool = regexp.MustCompile(`Bash\(([^)]*)\)`)

func markdownFrontmatterCommands(source []byte) []segment {
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
	var segments []segment
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
					segments = append(segments, segment{Kind: segmentShell, Line: node.Line + 1, Text: command})
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

func parseYAMLSegments(source []byte) ([]segment, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(source, &root); err != nil {
		return nil, err
	}
	var segments []segment
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

func yamlScalarSegments(source []byte, node *yaml.Node) []segment {
	if node.Style == yaml.FoldedStyle || node.Style == yaml.LiteralStyle {
		return yamlBlockScalarSegments(source, node)
	}
	if physical := yamlFlowScalarLines(source, node); len(physical) > 1 {
		var segments []segment
		for _, line := range physical {
			segments = append(segments, yamlValueSegments(line.text, line.number)...)
		}
		return segments
	}
	var segments []segment
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

func yamlValueSegments(raw string, line int) []segment {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	kind := segmentProse
	if !allowedBashTool.MatchString(value) && commandShaped(value) {
		kind = segmentShell
	}
	segments := []segment{{Kind: kind, Line: line, Text: value}}
	segments = append(segments, bashToolSegments(value, line)...)
	return appendEmbeddedProseSegments(segments, value, line)
}

func yamlBlockScalarSegments(source []byte, node *yaml.Node) []segment {
	lines := strings.Split(string(source), "\n")
	header := node.Line - 1
	if header < 0 || header >= len(lines) {
		return nil
	}
	headerIndent := leadingSpaces(lines[header])
	var segments []segment
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

func bashToolSegments(value string, line int) []segment {
	var segments []segment
	for _, match := range allowedBashTool.FindAllStringSubmatch(value, -1) {
		command := strings.ReplaceAll(match[1], ":*", " *")
		segments = append(segments, segment{Kind: segmentShell, Line: line, Text: command})
	}
	return segments
}

func appendYAMLCommentSegments(segments []segment, seen map[string]bool, comment string, startLine int) []segment {
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
		segments = append(segments, segment{Kind: segmentProse, Line: line, Text: value})
		if commandShaped(value) {
			segments = append(segments, segment{Kind: segmentShell, Line: line, Text: value})
		}
		segments = appendEmbeddedProseSegments(segments, value, line)
		for _, match := range embeddedShellCommand.FindAllStringSubmatch(value, -1) {
			command := match[1]
			if command == "" {
				command = match[2]
			}
			if strings.TrimSpace(command) != "" {
				segments = append(segments, segment{Kind: segmentInline, Line: line, Text: command})
			}
		}
	}
	return segments
}

func appendEmbeddedProseSegments(segments []segment, value string, line int) []segment {
	for _, match := range embeddedProseCommand.FindAllStringSubmatch(value, -1) {
		if len(match) == 2 {
			segments = append(segments, segment{Kind: segmentInline, Line: line, Text: strings.TrimSpace(match[1])})
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

func classifyMarkdown(root ast.Node, source []byte) (map[int]segmentKind, []segment) {
	classified := map[int]segmentKind{}
	var inline []segment
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

func classifyIndentedCodeBlock(classified map[int]segmentKind, block *ast.CodeBlock, source []byte) {
	continuing := false
	for i := 0; i < block.Lines().Len(); i++ {
		segment := block.Lines().At(i)
		value := string(segment.Value(source))
		kind := segmentProse
		if continuing || commandShaped(value) {
			kind = segmentShell
		}
		classifySourceLines(classified, segment, source, kind)
		continuing = kind == segmentShell && shellContinues(value)
	}
}

func classifyFence(classified map[int]segmentKind, block *ast.FencedCodeBlock, source []byte) {
	kind := segmentProse
	language := strings.TrimSpace(string(block.Language(source)))
	if shellLanguage(language) {
		kind = segmentShell
	}
	continuing := false
	for i := 0; i < block.Lines().Len(); i++ {
		segment := block.Lines().At(i)
		value := segment.Value(source)
		lineKind := kind
		if kind != segmentShell && (continuing || commandShaped(string(value))) {
			lineKind = segmentShell
		}
		classifySourceLines(classified, segment, source, lineKind)
		continuing = lineKind == segmentShell && shellContinues(string(value))
	}
}

func classifySourceLines(classified map[int]segmentKind, segment text.Segment, source []byte, kind segmentKind) {
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
		case "agm", "bash", "bd", "command", "dash", "env", "eval", "exec", "gh", "git", "gtimeout", "ksh", "nohup", "safe-merge", "safe-pr", "safe-push", "sh", "sudo", "timeout", "zsh":
			return true
		case "if", "while", "until", "then", "do", "!":
			return true
		}
	}
	return strings.Contains(value, "&&") || strings.Contains(value, "||")
}

var embeddedCommandStart = regexp.MustCompile(`(?:^|["':=\[(][[:space:]]*)((?:agm|bd|gh|git|safe-merge|safe-pr|safe-push)\b.*)$`)
var embeddedProseCommand = regexp.MustCompile(`(?i)\b(?:run|use|execute|invoke|call|try)\s+((?:agm|bd|gh|git|safe-merge|safe-pr|safe-push)\b.*)$`)

func proseCommandSegments(source []byte, classified map[int]segmentKind) []segment {
	var segments []segment
	for index, raw := range strings.Split(string(source), "\n") {
		line := index + 1
		if _, ok := classified[line]; ok {
			continue
		}
		for _, match := range embeddedProseCommand.FindAllStringSubmatch(raw, -1) {
			if len(match) == 2 {
				segments = append(segments, segment{Kind: segmentInline, Line: line, Text: strings.TrimSpace(match[1])})
			}
		}
	}
	return segments
}

func codeSpanSegment(span *ast.CodeSpan, source []byte) (segment, bool) {
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
	return segment{Kind: segmentInline, Line: line, Text: normalized}, normalized != ""
}

func sourceSegments(source []byte, classified map[int]segmentKind) []segment {
	lines := strings.Split(string(source), "\n")
	segments := make([]segment, 0, len(lines))
	continuing := false
	for i, raw := range lines {
		line := i + 1
		normalized := strings.TrimSpace(raw)
		if strings.HasPrefix(normalized, "```") || strings.HasPrefix(normalized, "~~~") {
			continuing = false
			continue
		}
		kind, classifiedLine := classified[line]
		if classifiedLine && kind == "skip" {
			continuing = false
			continue
		}
		if !classifiedLine {
			kind = segmentProse
			if continuing || commandShaped(normalized) {
				kind = segmentShell
			}
		}
		if normalized != "" {
			segments = append(segments, segment{Kind: kind, Line: line, Text: normalized})
		}
		continuing = !classifiedLine && normalized != "" && kind == segmentShell && shellContinues(normalized)
	}
	return segments
}

func joinShellContinuations(segments []segment) []segment {
	joined := make([]segment, 0, len(segments))
	for i := 0; i < len(segments); i++ {
		current := segments[i]
		if current.Kind != segmentShell || !shellContinues(current.Text) {
			joined = append(joined, current)
			continue
		}

		var command strings.Builder
		command.WriteString(trimShellContinuation(current.Text))
		lastLine := current.Line
		for i+1 < len(segments) && segments[i+1].Kind == segmentShell && segments[i+1].Line == lastLine+1 {
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
