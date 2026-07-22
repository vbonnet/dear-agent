package instructionlint

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
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

func parseGoPromptSegments(source []byte) ([]Segment, error) {
	files := token.NewFileSet()
	root, err := parser.ParseFile(files, "instruction-surface.go", source, 0)
	if err != nil {
		return nil, err
	}
	var segments []Segment
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

func goLiteralSegments(files *token.FileSet, literal *goast.BasicLit) []Segment {
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
	state := scriptParseState{
		visibleHelpers:   agentVisibleScriptHelpers(source),
		pendingHeredocs:  map[string][]Segment{},
		pendingVariables: map[string][]Segment{},
		variableAliases:  map[string]string{},
		descriptors:      defaultScriptDescriptors(),
	}
	line := 0
	for raw := range strings.SplitSeq(string(source), "\n") {
		line++
		value := strings.TrimSpace(raw)
		if len(state.heredocs) > 0 {
			heredoc := &state.heredocs[0]
			terminator := raw
			if heredoc.stripTabs {
				terminator = strings.TrimLeft(terminator, "\t")
			}
			if terminator == heredoc.marker {
				segments = state.commitHeredoc(segments, *heredoc)
				state.heredocs = state.heredocs[1:]
				continue
			}
			heredoc.body = append(heredoc.body, Segment{Kind: SegmentShell, Line: line, Text: value})
			continue
		}
		if value == "" {
			continue
		}
		state.updatePersistentDescriptors(value)
		state.updatePendingFileWrites(value)
		state.propagatePendingVariables(value)
		state.updateVariableAliases(value)
		for path, pending := range state.pendingHeredocs {
			if variable, captured := scriptAssignedSubstitutionReadsPath(value, path, state.descriptors); captured {
				state.pendingVariables[variable] = append(state.pendingVariables[variable], pending...)
				delete(state.pendingHeredocs, path)
				continue
			}
			if scriptLineExecutesPath(value, path) || scriptLinePrintsPath(value, path, state.descriptors) {
				segments = append(segments, pending...)
				delete(state.pendingHeredocs, path)
			}
		}
		for variable, pending := range state.pendingVariables {
			if scriptLinePrintsVariable(
				value, variable, state.visibleHelpers, state.variableAliases, state.descriptors,
			) {
				segments = append(segments, pending...)
				delete(state.pendingVariables, variable)
			}
		}
		if state.visibleContinuation {
			continued := state.continuedWith(value)
			if heredocs := scriptHeredocSpecs(continued, state.descriptors); len(heredocs) > 0 {
				state.heredocs = heredocs
				state.visibleContinuation = false
				state.continuedCommand = ""
				continue
			}
		}
		if include, handled := state.consumeOngoing(raw, value); handled {
			if include {
				segments = append(segments, Segment{Kind: SegmentShell, Line: line, Text: value})
			}
			continue
		}
		if strings.HasPrefix(value, "#") {
			segments = append(segments, Segment{Kind: SegmentProse, Line: line, Text: value})
			continue
		}
		if heredocs := scriptHeredocSpecs(value, state.descriptors); len(heredocs) > 0 {
			state.heredocs = heredocs
			continue
		}
		assignment := stripShellDeclaration(value)
		if shellAssignment.MatchString(assignment) && strings.Contains(assignment, "$(") {
			segments = append(segments, Segment{Kind: SegmentShell, Line: line, Text: value})
			continue
		}
		if assignmentQuote := scriptAssignmentQuote(assignment); assignmentQuote != 0 {
			segments = append(segments, Segment{Kind: SegmentShell, Line: line, Text: value})
			if unescapedByteCount(raw, assignmentQuote)%2 == 1 {
				state.quote = assignmentQuote
			}
			continue
		}
		if agentVisibleScriptCommand(value, state.visibleHelpers) {
			segments = append(segments, Segment{Kind: SegmentShell, Line: line, Text: value})
			if commandQuote := unclosedScriptQuote(value); commandQuote != 0 {
				state.quote = commandQuote
			} else {
				state.visibleContinuation = hasShellLineContinuation(raw)
				if state.visibleContinuation {
					state.continuedCommand = stripShellLineContinuation(value)
				}
			}
		}
	}
	sort.SliceStable(segments, func(left, right int) bool {
		return segments[left].Line < segments[right].Line
	})
	return segments
}

type scriptParseState struct {
	quote               byte
	heredocs            []scriptHeredoc
	pendingHeredocs     map[string][]Segment
	pendingVariables    map[string][]Segment
	variableAliases     map[string]string
	descriptors         map[int]scriptOutputDestination
	visibleContinuation bool
	continuedCommand    string
	visibleHelpers      map[string]bool
}

type scriptHeredoc struct {
	marker    string
	stripTabs bool
	body      []Segment
	outputs   []scriptOutputDestination
}

func (state *scriptParseState) commitHeredoc(segments []Segment, heredoc scriptHeredoc) []Segment {
	if slices.ContainsFunc(heredoc.outputs, func(output scriptOutputDestination) bool { return output.visible }) {
		return append(segments, heredoc.body...)
	}
	for _, output := range heredoc.outputs {
		if output.variable != "" {
			body := heredoc.body
			if output.captureLines > 0 && len(body) > output.captureLines {
				body = body[:output.captureLines]
			}
			state.pendingVariables[output.variable] = slices.Clone(body)
			continue
		}
		if output.path == "" {
			continue
		}
		if output.append {
			state.pendingHeredocs[output.path] = append(state.pendingHeredocs[output.path], heredoc.body...)
		} else {
			state.pendingHeredocs[output.path] = slices.Clone(heredoc.body)
		}
	}
	return segments
}

func (state *scriptParseState) updatePersistentDescriptors(value string) {
	commands := splitScriptCommandParts(value)
	for index, command := range commands {
		if index > 0 && commands[index-1].separator != ";" {
			// A pipeline, background job, or conditional command may not execute in
			// this shell. Do not let its hypothetical exec mutate later routing.
			continue
		}
		tokens := parseShellTokens(command.text)
		for len(tokens) > 0 && tokens[0].text == "{" {
			tokens = tokens[1:]
		}
		if len(tokens) == 0 || tokens[0].text != "exec" {
			continue
		}
		applyScriptRedirections(tokens[1:], state.descriptors)
	}
}

func (state *scriptParseState) updatePendingFileWrites(value string) {
	commands := splitScriptCommandParts(value)
	for index, command := range commands {
		destination := scriptCommandOutputDestination(command.text, state.descriptors)
		if destination.redirected && destination.path != "" && !destination.append {
			delete(state.pendingHeredocs, destination.path)
		}

		pipelineDestination := scriptHeredocCommandOutputDestination(commands, index, state.descriptors)
		if pipelineDestination.visible || !pipelineDestination.redirected || pipelineDestination.path == "" {
			continue
		}
		var copied []Segment
		for path, pending := range state.pendingHeredocs {
			if scriptCommandReadsPath(command.text, path, state.descriptors) {
				copied = append(copied, pending...)
			}
		}
		if len(copied) == 0 {
			continue
		}
		if pipelineDestination.append {
			state.pendingHeredocs[pipelineDestination.path] = append(
				state.pendingHeredocs[pipelineDestination.path], copied...)
		} else {
			state.pendingHeredocs[pipelineDestination.path] = slices.Clone(copied)
		}
	}
}

func (state *scriptParseState) propagatePendingVariables(value string) {
	assignment := stripShellDeclaration(strings.TrimSpace(value))
	if !shellAssignment.MatchString(assignment) {
		return
	}
	name, right, found := strings.Cut(assignment, "=")
	if !found {
		return
	}
	var propagated []Segment
	for variable, pending := range state.pendingVariables {
		if scriptReferencesVariable(right, variable) {
			propagated = append(propagated, pending...)
		}
	}
	if len(propagated) == 0 {
		delete(state.pendingVariables, name)
		return
	}
	state.pendingVariables[name] = slices.Clone(propagated)
}

func (state *scriptParseState) updateVariableAliases(value string) {
	assignment := stripShellDeclaration(strings.TrimSpace(value))
	if !shellAssignment.MatchString(assignment) {
		return
	}
	name, right, found := strings.Cut(assignment, "=")
	if !found || !shellVariableName.MatchString(right) {
		delete(state.variableAliases, name)
		return
	}
	state.variableAliases[name] = right
}

func (state *scriptParseState) consumeOngoing(raw, value string) (include, handled bool) {
	if state.visibleContinuation {
		state.continuedCommand = state.continuedWith(value)
		if commandQuote := unclosedScriptQuote(value); commandQuote != 0 {
			state.quote = commandQuote
			state.visibleContinuation = false
		} else {
			state.visibleContinuation = hasShellLineContinuation(raw)
		}
		if !state.visibleContinuation {
			state.continuedCommand = ""
		}
		return true, true
	}
	if state.quote != 0 {
		if unescapedByteCount(raw, state.quote)%2 == 1 {
			state.quote = 0
		}
		return true, true
	}
	return false, false
}

func (state *scriptParseState) continuedWith(value string) string {
	return strings.TrimSpace(state.continuedCommand + " " + stripShellLineContinuation(value))
}

func hasShellLineContinuation(value string) bool {
	backslashes := 0
	for index := len(value) - 1; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func stripShellLineContinuation(value string) string {
	if hasShellLineContinuation(value) {
		return strings.TrimSpace(value[:len(value)-1])
	}
	return strings.TrimSpace(value)
}

func agentVisibleScriptCommand(value string, helpers map[string]bool) bool {
	for _, command := range splitShellCommands(value) {
		fields := stripCommandPrefixes(parseShellWords(command))
		if len(fields) > 0 && (slices.Contains([]string{"cat", "echo", "emit", "jq", "printf"}, fields[0]) || helpers[fields[0]]) {
			return true
		}
	}
	return false
}

func scriptLinePrintsVariable(
	value, variable string,
	helpers map[string]bool,
	aliases map[string]string,
	descriptors map[int]scriptOutputDestination,
) bool {
	commands := splitScriptCommandParts(value)
	for index, command := range commands {
		if !scriptReferencesVariable(command.text, variable) &&
			!scriptIndirectExpansionSelectsVariable(command.text, variable, aliases) {
			continue
		}
		if !agentVisibleScriptCommand(command.text, helpers) &&
			!scriptHereStringReferencesVariable(command.text, variable) {
			continue
		}
		if scriptHeredocCommandOutputDestination(commands, index, descriptors).visible {
			return true
		}
	}
	return false
}

func scriptHereStringReferencesVariable(command, variable string) bool {
	fields := stripCommandPrefixes(parseShellWords(command))
	if len(fields) == 0 || slices.Contains([]string{"read", "readarray", "mapfile"}, executableBase(fields[0])) {
		return false
	}
	remaining := command
	for {
		index := strings.Index(remaining, "<<<")
		if index < 0 {
			return false
		}
		fields := parseShellWords(remaining[index+3:])
		if len(fields) > 0 && scriptReferencesVariable(fields[0], variable) {
			return true
		}
		remaining = remaining[index+3:]
	}
}

var shellIndirectExpansion = regexp.MustCompile(`\$\{!([A-Za-z_][A-Za-z0-9_]*)\}`)

func scriptIndirectExpansionSelectsVariable(command, variable string, aliases map[string]string) bool {
	for _, match := range shellIndirectExpansion.FindAllStringSubmatch(command, -1) {
		if len(match) == 2 && aliases[match[1]] == variable {
			return true
		}
	}
	return false
}

var shellAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var shellVariableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var shellDeclaration = regexp.MustCompile(`^(?:local|export|readonly|typeset|declare)(?:\s+-[A-Za-z]+)*\s+`)
var shellCommandSubstitution = regexp.MustCompile(`\$\(([^()]*)\)`)
var legacyShellCommandSubstitution = regexp.MustCompile("`([^`]*)`")
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

type scriptHeredocRedirect struct {
	marker     string
	descriptor int
	stripTabs  bool
	operator   int
}

func scriptHeredocRedirects(value string) []scriptHeredocRedirect {
	var redirects []scriptHeredocRedirect
	scanner := scriptHeredocScanner{}
	for index := 0; index+1 < len(value); index++ {
		current := value[index]
		if consumed, advance := scanner.consumeQuotedCommandSubstitution(value, index); consumed {
			index += advance
			continue
		}
		if scanner.consumeQuotedOrEscaped(current) {
			continue
		}
		if consumed, advance := scanner.consumeArithmetic(value, index); consumed {
			index += advance
			continue
		}
		if current == '#' && (index == 0 || shellHorizontalSpace(value[index-1])) {
			break
		}
		if marker, ok := scriptHeredocMarkerAt(value, index); ok {
			redirects = append(redirects, scriptHeredocRedirect{
				marker: marker, descriptor: scriptHeredocDescriptor(value, index),
				stripTabs: index+2 < len(value) && value[index+2] == '-',
				operator:  index,
			})
		}
	}
	return redirects
}

func scriptHeredocDescriptor(value string, operator int) int {
	start := operator
	for start > 0 && value[start-1] >= '0' && value[start-1] <= '9' {
		start--
	}
	if start == operator || (start > 0 && !shellHorizontalSpace(value[start-1]) &&
		!strings.ContainsRune(";|&({", rune(value[start-1]))) {
		return 0
	}
	descriptor, err := strconv.Atoi(value[start:operator])
	if err != nil {
		return 0
	}
	return descriptor
}

func scriptHasLaterStdinRedirect(value string, after int) bool {
	scanner := scriptHeredocScanner{}
	for index := 0; index < len(value); index++ {
		current := value[index]
		if consumed, advance := scanner.consumeQuotedCommandSubstitution(value, index); consumed {
			index += advance
			continue
		}
		if scanner.consumeQuotedOrEscaped(current) {
			continue
		}
		if consumed, advance := scanner.consumeArithmetic(value, index); consumed {
			index += advance
			continue
		}
		if current == '#' && (index == 0 || shellHorizontalSpace(value[index-1])) {
			break
		}
		if index <= after || current != '<' || index > 0 && value[index-1] == '<' ||
			index+1 < len(value) && value[index+1] == '(' {
			continue
		}
		if scriptHeredocDescriptor(value, index) == 0 {
			return true
		}
	}
	return false
}

func scriptHeredocMarkerAt(value string, index int) (string, bool) {
	if value[index] != '<' || value[index+1] != '<' ||
		(index > 0 && value[index-1] == '<') || (index+2 < len(value) && value[index+2] == '<') {
		return "", false
	}
	markerStart := index + 2
	if markerStart < len(value) && value[markerStart] == '-' {
		markerStart++
	}
	return scriptHeredocWord(value, markerStart)
}

type scriptHeredocScanner struct {
	quote                   byte
	escaped                 bool
	arithmeticDepth         int
	quotedSubstitutionDepth int
}

func (scanner *scriptHeredocScanner) consumeQuotedCommandSubstitution(
	value string,
	index int,
) (bool, int) {
	if scanner.quotedSubstitutionDepth == 0 {
		if scanner.quote != '"' || !strings.HasPrefix(value[index:], "$(") ||
			strings.HasPrefix(value[index:], "$((") {
			return false, 0
		}
		scanner.quote = 0
		scanner.quotedSubstitutionDepth = 1
		return true, 1
	}
	if scanner.quote != 0 {
		return false, 0
	}
	if strings.HasPrefix(value[index:], "$(") && !strings.HasPrefix(value[index:], "$((") {
		scanner.quotedSubstitutionDepth++
		return true, 1
	}
	switch value[index] {
	case '(':
		scanner.quotedSubstitutionDepth++
		return true, 0
	case ')':
		scanner.quotedSubstitutionDepth--
		if scanner.quotedSubstitutionDepth == 0 {
			scanner.quote = '"'
		}
		return true, 0
	default:
		return false, 0
	}
}

func (scanner *scriptHeredocScanner) consumeQuotedOrEscaped(current byte) bool {
	if scanner.escaped {
		scanner.escaped = false
		return true
	}
	if current == '\\' && scanner.quote != '\'' {
		scanner.escaped = true
		return true
	}
	if scanner.quote != 0 {
		if current == scanner.quote {
			scanner.quote = 0
		}
		return true
	}
	if current == '\'' || current == '"' {
		scanner.quote = current
		return true
	}
	return false
}

func (scanner *scriptHeredocScanner) consumeArithmetic(value string, index int) (bool, int) {
	if scanner.arithmeticDepth > 0 {
		switch value[index] {
		case '(':
			scanner.arithmeticDepth++
		case ')':
			scanner.arithmeticDepth--
		}
		return true, 0
	}
	if strings.HasPrefix(value[index:], "$((") {
		scanner.arithmeticDepth = 2
		return true, 2
	}
	if strings.HasPrefix(value[index:], "((") && arithmeticCommandBoundary(value, index) {
		scanner.arithmeticDepth = 2
		return true, 1
	}
	return false, 0
}

func arithmeticCommandBoundary(value string, index int) bool {
	if index == 0 {
		return true
	}
	return shellHorizontalSpace(value[index-1]) || strings.ContainsRune(";|&({", rune(value[index-1]))
}

func scriptHeredocWord(value string, start int) (string, bool) {
	for start < len(value) && shellHorizontalSpace(value[start]) {
		start++
	}
	var marker strings.Builder
	quote := byte(0)
	escaped := false
	started := false
	for index := start; index < len(value); index++ {
		current := value[index]
		if escaped {
			marker.WriteByte(current)
			escaped = false
			started = true
			continue
		}
		if current == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			} else {
				marker.WriteByte(current)
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			started = true
			continue
		}
		if heredocWordBoundary(current) {
			break
		}
		marker.WriteByte(current)
		started = true
	}
	if quote != 0 {
		return "", false
	}
	if escaped {
		marker.WriteByte('\\')
	}
	return marker.String(), started
}

func shellHorizontalSpace(value byte) bool {
	return value == ' ' || value == '\t'
}

func heredocWordBoundary(value byte) bool {
	return shellHorizontalSpace(value) || strings.ContainsRune(";|&<>", rune(value))
}

func scriptHeredocSpecs(value string, descriptors map[int]scriptOutputDestination) []scriptHeredoc {
	commands := splitScriptCommandParts(value)
	var heredocs []scriptHeredoc
	for index, command := range commands {
		redirects := scriptHeredocRedirects(command.text)
		if len(redirects) == 0 {
			continue
		}
		effectiveStdin := -1
		for redirectIndex, redirect := range redirects {
			if redirect.descriptor == 0 {
				effectiveStdin = redirectIndex
			}
		}
		for redirectIndex, redirect := range redirects {
			heredoc := scriptHeredoc{marker: redirect.marker, stripTabs: redirect.stripTabs}
			// The shell consumes every heredoc body in lexical order, but only
			// the last redirect for descriptor zero supplies that command's stdin.
			if redirectIndex == effectiveStdin && !scriptHasLaterStdinRedirect(command.text, redirect.operator) {
				captures := scriptCapturedVariableDestinations(command.text)
				substitutionCaptures := scriptCommandSubstitutionVariableDestinations(
					command.text, redirect.operator)
				switch {
				case scriptReadUsesAlternateDescriptor(command.text):
					// read -u consumes the selected descriptor rather than fd 0.
				case len(captures) > 0:
					heredoc.outputs = captures
				case len(substitutionCaptures) > 0:
					heredoc.outputs = substitutionCaptures
				case scriptCommandIgnoresHeredocInput(command.text):
					// The body is neither emitted nor retained for later output.
				default:
					heredoc.outputs = append(heredoc.outputs,
						scriptHeredocCommandOutputDestination(commands, index, descriptors))
					heredoc.outputs = append(heredoc.outputs,
						scriptPipelineTeeFileDestinations(commands, index)...)
				}
			}
			heredocs = append(heredocs, heredoc)
		}
	}
	return heredocs
}

func scriptCapturedVariableDestinations(command string) []scriptOutputDestination {
	fields := stripCommandPrefixes(parseShellWords(command))
	if len(fields) == 0 {
		return nil
	}
	executable := executableBase(fields[0])
	if executable != "read" && executable != "mapfile" && executable != "readarray" {
		return nil
	}
	captureLines := 0
	if executable == "read" && !readUsesNonDefaultTerminator(fields[1:]) {
		captureLines = 1
	}
	variables := fields[1:]
	if executable == "read" {
		variables = scriptReadCapturedVariables(variables)
	}
	var destinations []scriptOutputDestination
	for _, field := range variables {
		if shellVariableName.MatchString(field) {
			destinations = append(destinations, scriptOutputDestination{variable: field, captureLines: captureLines})
		}
	}
	if len(destinations) > 0 {
		return destinations
	}
	if executable == "read" {
		return []scriptOutputDestination{{variable: "REPLY", captureLines: captureLines}}
	}
	return []scriptOutputDestination{{variable: "MAPFILE"}}
}

func scriptReadCapturedVariables(arguments []string) []string {
	var variables []string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			variables = append(variables, arguments[index+1:]...)
			break
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			variables = append(variables, argument)
			continue
		}
		if strings.HasPrefix(argument, "--") {
			continue
		}
		options := strings.TrimPrefix(argument, "-")
		for optionIndex, option := range options {
			if !strings.ContainsRune("adinNptu", option) {
				continue
			}
			operand := options[optionIndex+1:]
			if operand == "" && index+1 < len(arguments) {
				index++
				operand = arguments[index]
			}
			if option == 'a' && shellVariableName.MatchString(operand) {
				variables = append(variables, operand)
			}
			break
		}
	}
	return variables
}

func scriptCommandSubstitutionVariableDestinations(
	command string,
	heredocOperator int,
) []scriptOutputDestination {
	if heredocOperator < 0 || heredocOperator > len(command) {
		return nil
	}
	assignment := stripShellDeclaration(strings.TrimSpace(command[:heredocOperator]))
	if !shellAssignment.MatchString(assignment) {
		return nil
	}
	name, right, found := strings.Cut(assignment, "=")
	if !found || !strings.Contains(right, "$(") && !strings.Contains(right, "`") {
		return nil
	}
	return []scriptOutputDestination{{variable: name}}
}

func scriptReadUsesAlternateDescriptor(command string) bool {
	fields := stripCommandPrefixes(parseShellWords(command))
	if len(fields) == 0 || executableBase(fields[0]) != "read" {
		return false
	}
	arguments := fields[1:]
	for index, argument := range arguments {
		if !strings.HasPrefix(argument, "-") || strings.HasPrefix(argument, "--") {
			continue
		}
		option := strings.IndexByte(strings.TrimPrefix(argument, "-"), 'u')
		if option < 0 {
			continue
		}
		value := strings.TrimPrefix(argument, "-")[option+1:]
		if value == "" && index+1 < len(arguments) {
			value = arguments[index+1]
		}
		return value != "0"
	}
	return false
}

func readUsesNonDefaultTerminator(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--delimiter" || strings.HasPrefix(argument, "--delimiter=") {
			return true
		}
		if strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") &&
			strings.ContainsAny(strings.TrimPrefix(argument, "-"), "dNn") {
			return true
		}
	}
	return false
}

func scriptCommandIgnoresHeredocInput(command string) bool {
	fields := stripCommandPrefixes(parseShellWords(command))
	return len(fields) > 0 && slices.Contains([]string{"echo", "false", "printf", "true", ":"}, executableBase(fields[0]))
}

func scriptTeeFileDestinations(command string) []scriptOutputDestination {
	fields := stripCommandPrefixes(scriptCommandWords(command))
	if len(fields) == 0 || executableBase(fields[0]) != "tee" {
		return nil
	}
	appendMode := false
	options := true
	var destinations []scriptOutputDestination
	for _, field := range fields[1:] {
		if options && field == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(field, "-") {
			appendMode = appendMode || field == "--append" ||
				(!strings.HasPrefix(field, "--") && strings.Contains(field, "a"))
			continue
		}
		redirect := strings.TrimLeft(field, "0123456789")
		if strings.HasPrefix(redirect, "<") || strings.HasPrefix(redirect, ">") {
			continue
		}
		if strings.ContainsAny(field, "$`") {
			return []scriptOutputDestination{{visible: true}}
		}
		destinations = append(destinations, scriptOutputDestination{path: filepath.Clean(field), append: appendMode})
	}
	return destinations
}

func scriptPipelineTeeFileDestinations(
	commands []scriptCommandPart,
	producer int,
) []scriptOutputDestination {
	pipelineEnd := producer
	for pipelineEnd < len(commands)-1 && commands[pipelineEnd].separator == "|" {
		pipelineEnd++
	}
	var destinations []scriptOutputDestination
	for index := producer; index <= pipelineEnd; index++ {
		destinations = append(destinations, scriptTeeFileDestinations(commands[index].text)...)
	}
	return destinations
}

func scriptCommandWords(command string) []string {
	tokens := parseShellTokens(command)
	words := make([]string, 0, len(tokens))
	skipTarget := false
	for _, token := range tokens {
		if skipTarget {
			skipTarget = false
			continue
		}
		if redirect, hasTarget := scriptRedirectionToken(token.raw); redirect {
			skipTarget = !hasTarget
			continue
		}
		words = append(words, token.text)
	}
	return words
}

func scriptRedirectionToken(raw string) (redirect, hasTarget bool) {
	value := strings.Trim(raw, "()")
	if strings.HasPrefix(value, "&>") {
		value = value[1:]
	} else {
		value = strings.TrimLeft(value, "0123456789")
	}
	if value == "" || value[0] != '<' && value[0] != '>' {
		return false, false
	}
	index := 0
	for index < len(value) && (value[index] == '<' || value[index] == '>') {
		index++
	}
	if index < len(value) && (value[index] == '|' || value[index] == '-') {
		index++
	}
	if index < len(value) && value[index] == '&' {
		index++
	}
	return true, index < len(value)
}

func scriptHeredocCommandOutputDestination(
	commands []scriptCommandPart,
	producer int,
	descriptors map[int]scriptOutputDestination,
) scriptOutputDestination {
	pipelineEnd := producer
	for pipelineEnd < len(commands)-1 && commands[pipelineEnd].separator == "|" {
		pipelineEnd++
	}
	destination := scriptOutputDestination{visible: true}
	for index := producer; index <= pipelineEnd; index++ {
		destination = scriptCommandOutputDestination(commands[index].text, descriptors)
		if destination.redirected {
			break
		}
	}
	if destination.visible {
		return destination
	}
	if destination.path == "" {
		return destination
	}
	for _, command := range commands[pipelineEnd+1:] {
		if scriptCommandPrintsPath(command.text, destination.path, descriptors) {
			return scriptOutputDestination{visible: true}
		}
	}
	return destination
}

type scriptOutputDestination struct {
	visible      bool
	path         string
	variable     string
	captureLines int
	redirected   bool
	append       bool
}

type scriptCommandPart struct {
	text      string
	separator string
}

func splitScriptCommandParts(value string) []scriptCommandPart {
	parts := make([]scriptCommandPart, 0, 2)
	start := 0
	scanner := shellScanState{}
	for index := 0; index < len(value); index++ {
		width := scanner.boundaryWidth(value, index)
		if width == 0 {
			continue
		}
		if command := strings.TrimSpace(value[start:index]); command != "" {
			parts = append(parts, scriptCommandPart{text: command, separator: value[index : index+width]})
		}
		index += width - 1
		start = index + 1
	}
	if command := strings.TrimSpace(value[start:]); command != "" {
		parts = append(parts, scriptCommandPart{text: command})
	}
	return parts
}

func defaultScriptDescriptors() map[int]scriptOutputDestination {
	visible := scriptOutputDestination{visible: true}
	return map[int]scriptOutputDestination{1: visible, 2: visible}
}

func cloneScriptDescriptors(source map[int]scriptOutputDestination) map[int]scriptOutputDestination {
	cloned := make(map[int]scriptOutputDestination, len(source))
	maps.Copy(cloned, source)
	return cloned
}

func scriptCommandOutputDestination(command string, inherited map[int]scriptOutputDestination) scriptOutputDestination {
	descriptors := cloneScriptDescriptors(inherited)
	applyScriptRedirections(parseShellTokens(command), descriptors)
	return descriptors[1]
}

type shellToken struct {
	raw  string
	text string
}

// parseShellTokens retains the raw spelling alongside the dequoted value.
// Redirection syntax is meaningful only when its operator is unquoted and
// unescaped; parseShellWords intentionally discards exactly that provenance.
func parseShellTokens(input string) []shellToken {
	parser := shellTokenParser{}
	for index := 0; index < len(input); index++ {
		parser.consume(input[index])
	}
	if parser.escaped {
		parser.value.WriteByte('\\')
	}
	parser.flush()
	return parser.tokens
}

type shellTokenParser struct {
	tokens         []shellToken
	raw            strings.Builder
	value          strings.Builder
	quote          byte
	escaped        bool
	started        bool
	redirect       bool
	redirectTarget bool
}

func (parser *shellTokenParser) consume(current byte) {
	switch {
	case parser.escaped:
		parser.consumeEscaped(current)
	case current == '\\' && parser.quote != '\'':
		parser.beginEscape()
	case parser.quote != 0:
		parser.consumeQuoted(current)
	case current == '\'' || current == '"':
		parser.beginQuote(current)
	case strings.ContainsRune(" \t\r\n", rune(current)):
		parser.flush()
	case current == '>':
		parser.consumeRedirectOperator()
	case parser.redirect && !parser.redirectTarget && current == '|' && strings.HasSuffix(parser.raw.String(), ">"):
		parser.write(current)
	default:
		if parser.redirect {
			parser.redirectTarget = true
		}
		parser.write(current)
	}
}

func (parser *shellTokenParser) consumeEscaped(current byte) {
	if parser.redirect {
		parser.redirectTarget = true
	}
	parser.raw.WriteByte(current)
	parser.value.WriteByte(current)
	parser.escaped = false
	parser.started = true
}

func (parser *shellTokenParser) beginEscape() {
	if parser.redirect {
		parser.redirectTarget = true
	}
	parser.raw.WriteByte('\\')
	parser.escaped = true
	parser.started = true
}

func (parser *shellTokenParser) consumeQuoted(current byte) {
	parser.raw.WriteByte(current)
	if current == parser.quote {
		parser.quote = 0
	} else {
		parser.value.WriteByte(current)
	}
	parser.started = true
}

func (parser *shellTokenParser) beginQuote(quote byte) {
	if parser.redirect {
		parser.redirectTarget = true
	}
	parser.raw.WriteByte(quote)
	parser.quote = quote
	parser.started = true
}

func (parser *shellTokenParser) consumeRedirectOperator() {
	if parser.redirect && !parser.redirectTarget && strings.HasSuffix(parser.raw.String(), ">") {
		parser.write('>')
		return
	}
	prefix := parser.value.String()
	if parser.started && (parser.redirect || (prefix != "&" && !asciiDigits(prefix))) {
		parser.flush()
	}
	parser.redirect = true
	parser.write('>')
}

func (parser *shellTokenParser) write(current byte) {
	parser.raw.WriteByte(current)
	parser.value.WriteByte(current)
	parser.started = true
}

func (parser *shellTokenParser) flush() {
	if !parser.started {
		return
	}
	parser.tokens = append(parser.tokens, shellToken{raw: parser.raw.String(), text: parser.value.String()})
	parser.raw.Reset()
	parser.value.Reset()
	parser.started = false
	parser.redirect = false
	parser.redirectTarget = false
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func applyScriptRedirections(tokens []shellToken, descriptors map[int]scriptOutputDestination) {
	for index := 0; index < len(tokens); index++ {
		raw := strings.Trim(tokens[index].raw, "()")
		value := strings.Trim(tokens[index].text, "()")
		both := false
		descriptor := 1
		operator := ""
		consumed := 0
		switch {
		case strings.HasPrefix(raw, "&>>"):
			both, operator, consumed = true, ">>", 3
		case strings.HasPrefix(raw, "&>"):
			both, operator, consumed = true, ">", 2
		default:
			remainder := strings.TrimLeft(raw, "0123456789")
			prefix := strings.TrimSuffix(raw, remainder)
			if !strings.HasPrefix(remainder, ">") {
				continue
			}
			if prefix != "" {
				parsed, err := strconv.Atoi(prefix)
				if err != nil {
					continue
				}
				descriptor = parsed
			}
			switch {
			case strings.HasPrefix(remainder, ">>"):
				operator = ">>"
			case strings.HasPrefix(remainder, ">|"):
				operator = ">|"
			default:
				operator = ">"
			}
			consumed = len(prefix) + len(operator)
		}

		target := ""
		if consumed <= len(value) {
			target = value[consumed:]
		}
		if target == "" && index+1 < len(tokens) {
			index++
			target = tokens[index].text
		}
		destination := scriptRedirectDestination(target, descriptors)
		destination.append = operator == ">>"
		if both {
			destination.redirected = true
			descriptors[1] = destination
			descriptors[2] = destination
			continue
		}
		descriptors[descriptor] = destination
		if descriptor == 1 {
			destination.redirected = true
			descriptors[1] = destination
		}
	}
}

func scriptRedirectDestination(target string, descriptors map[int]scriptOutputDestination) scriptOutputDestination {
	switch target {
	case "", "&-":
		return scriptOutputDestination{}
	case "/dev/stderr", "/dev/stdout":
		return scriptOutputDestination{visible: true}
	}
	// A shell-expanded target cannot be proven file-only without executing the
	// script. It may resolve to /dev/stdout, /dev/stderr, or a visible descriptor,
	// so the hard policy gate must classify it conservatively as visible.
	if strings.ContainsAny(target, "$`") {
		return scriptOutputDestination{visible: true}
	}
	if descriptor, ok := strings.CutPrefix(target, "&"); ok {
		number, err := strconv.Atoi(descriptor)
		if err == nil {
			return descriptors[number]
		}
	}
	if descriptor, ok := strings.CutPrefix(target, "/dev/fd/"); ok {
		number, err := strconv.Atoi(descriptor)
		if err == nil {
			return descriptors[number]
		}
	}
	return scriptOutputDestination{path: filepath.Clean(target)}
}

func scriptCommandPrintsPath(command, path string, descriptors map[int]scriptOutputDestination) bool {
	return scriptCommandReadsPath(command, path, descriptors) &&
		scriptHeredocCommandOutputDestination([]scriptCommandPart{{text: command}}, 0, descriptors).visible
}

func scriptAssignedSubstitutionReadsPath(
	command, path string,
	descriptors map[int]scriptOutputDestination,
) (string, bool) {
	assignment := stripShellDeclaration(strings.TrimSpace(command))
	if !shellAssignment.MatchString(assignment) {
		return "", false
	}
	name, right, found := strings.Cut(assignment, "=")
	if !found || !scriptCommandSubstitutionsReadPath(right, path, descriptors) {
		return "", false
	}
	return name, true
}

func scriptLineExecutesPath(value, path string) bool {
	for _, command := range splitScriptCommandParts(value) {
		fields := stripCommandPrefixes(parseShellWords(command.text))
		if len(fields) < 2 {
			continue
		}
		executable := executableBase(fields[0])
		if !slices.Contains([]string{".", "bash", "dash", "ksh", "sh", "source", "zsh"}, executable) {
			continue
		}
		payloadFields := slices.Clone(fields)
		payloadFields[0] = executable
		if _, inlinePayload := shellCommandPayload(payloadFields); inlinePayload {
			continue
		}
		for _, argument := range fields[1:] {
			if sameScriptPath(argument, path) || strings.ContainsAny(argument, "$`") {
				return true
			}
		}
	}
	return false
}

func scriptCommandReadsPath(command, path string, descriptors map[int]scriptOutputDestination) bool {
	if scriptCommandSubstitutionsReadPath(command, path, descriptors) {
		return true
	}
	fields := stripCommandPrefixes(parseShellWords(command))
	if len(fields) > 0 {
		payloadFields := slices.Clone(fields)
		payloadFields[0] = executableBase(payloadFields[0])
		if payload, ok := shellCommandPayload(payloadFields); ok && scriptLinePrintsPath(payload, path, descriptors) {
			return true
		}
	}
	if len(fields) == 0 || !slices.Contains([]string{
		"awk", "base64", "cat", "cut", "grep", "head", "jq", "less", "more",
		"nl", "od", "rg", "sed", "sort", "strings", "tail", "uniq", "wc", "xxd",
	}, executableBase(fields[0])) {
		return false
	}
	for _, field := range fields[1:] {
		// A dynamic reader argument may resolve to any pending fixture. Treat it
		// conservatively so agent-visible output cannot evade the hard gate.
		if strings.ContainsAny(field, "$`") {
			return true
		}
		if sameScriptPath(field, path) {
			return true
		}
		redirect := strings.TrimLeft(field, "0123456789")
		if strings.HasPrefix(redirect, "<") && sameScriptPath(strings.TrimLeft(redirect, "<"), path) {
			return true
		}
	}
	return false
}

func scriptCommandSubstitutionsReadPath(
	command, path string,
	descriptors map[int]scriptOutputDestination,
) bool {
	for _, matcher := range []*regexp.Regexp{shellCommandSubstitution, legacyShellCommandSubstitution} {
		for _, match := range matcher.FindAllStringSubmatch(command, -1) {
			if len(match) == 2 && (scriptLinePrintsPath(match[1], path, descriptors) ||
				scriptInputOnlySubstitutionReadsPath(match[1], path)) {
				return true
			}
		}
	}
	return false
}

func scriptInputOnlySubstitutionReadsPath(command, path string) bool {
	fields := parseShellWords(strings.TrimSpace(command))
	switch len(fields) {
	case 1:
		return strings.HasPrefix(fields[0], "<") &&
			!strings.HasPrefix(fields[0], "<<") && sameScriptPath(strings.TrimPrefix(fields[0], "<"), path)
	case 2:
		return fields[0] == "<" && sameScriptPath(fields[1], path)
	default:
		return false
	}
}

func sameScriptPath(left, right string) bool {
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}

func scriptReferencesVariable(value, variable string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] != '$' || index+1 >= len(value) {
			continue
		}
		start := index + 1
		if value[start] == '{' {
			start++
		}
		end := start
		for end < len(value) && (value[end] == '_' || value[end] >= 'A' && value[end] <= 'Z' ||
			value[end] >= 'a' && value[end] <= 'z' || value[end] >= '0' && value[end] <= '9') {
			end++
		}
		if value[start:end] == variable {
			return true
		}
	}
	return false
}

func scriptLinePrintsPath(value, path string, descriptors map[int]scriptOutputDestination) bool {
	commands := splitScriptCommandParts(value)
	for index, command := range commands {
		if scriptCommandReadsPath(command.text, path, descriptors) &&
			scriptHeredocCommandOutputDestination(commands, index, descriptors).visible {
			return true
		}
	}
	return false
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
		kind := SegmentProse
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
			kind = SegmentProse
			if continuing || commandShaped(normalized) {
				kind = SegmentShell
			}
		}
		if normalized != "" {
			segments = append(segments, Segment{Kind: kind, Line: line, Text: normalized})
		}
		continuing = !classifiedLine && normalized != "" && kind == SegmentShell && shellContinues(normalized)
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
