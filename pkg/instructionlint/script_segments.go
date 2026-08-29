package instructionlint

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

func parseScriptSegments(source []byte) []segment {
	var segments []segment
	state := newScriptParseState(source)
	line := 0
	for raw := range strings.SplitSeq(string(source), "\n") {
		line++
		segments = state.consumeLine(segments, line, raw)
	}
	sort.SliceStable(segments, func(left, right int) bool {
		return segments[left].Line < segments[right].Line
	})
	return segments
}

func newScriptParseState(source []byte) scriptParseState {
	return scriptParseState{
		visibleHelpers:   agentVisibleScriptHelpers(source),
		pendingHeredocs:  map[string][]segment{},
		pendingVariables: map[string][]segment{},
		variableAliases:  map[string]string{},
		descriptors:      defaultScriptDescriptors(),
	}
}

type scriptParseState struct {
	quote               byte
	heredocs            []scriptHeredoc
	pendingHeredocs     map[string][]segment
	pendingVariables    map[string][]segment
	variableAliases     map[string]string
	descriptors         map[int]scriptOutputDestination
	visibleContinuation bool
	continuedCommand    string
	visibleHelpers      map[string]bool
}

type scriptHeredoc struct {
	marker    string
	stripTabs bool
	body      []segment
	outputs   []scriptOutputDestination
}

func (state *scriptParseState) consumeLine(segments []segment, line int, raw string) []segment {
	value := strings.TrimSpace(raw)
	if len(state.heredocs) > 0 {
		return state.consumeHeredocLine(segments, line, raw, value)
	}
	if value == "" {
		return segments
	}
	state.observeLine(value)
	segments = state.revealPendingSegments(segments, value)
	if state.beginContinuedHeredoc(value) {
		return segments
	}
	if include, handled := state.consumeOngoing(raw, value); handled {
		if include {
			segments = append(segments, segment{Kind: segmentShell, Line: line, Text: value})
		}
		return segments
	}
	return state.consumeNewLine(segments, line, raw, value)
}

func (state *scriptParseState) consumeHeredocLine(
	segments []segment,
	line int,
	raw, value string,
) []segment {
	heredoc := &state.heredocs[0]
	terminator := raw
	if heredoc.stripTabs {
		terminator = strings.TrimLeft(terminator, "\t")
	}
	if terminator == heredoc.marker {
		segments = state.commitHeredoc(segments, *heredoc)
		state.heredocs = state.heredocs[1:]
		return segments
	}
	heredoc.body = append(heredoc.body, segment{Kind: segmentShell, Line: line, Text: value})
	return segments
}

func (state *scriptParseState) observeLine(value string) {
	state.updatePersistentDescriptors(value)
	state.updatePendingFileWrites(value)
	state.propagatePendingVariables(value)
	state.updateVariableAliases(value)
}

func (state *scriptParseState) revealPendingSegments(segments []segment, value string) []segment {
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
	return segments
}

func (state *scriptParseState) beginContinuedHeredoc(value string) bool {
	if !state.visibleContinuation {
		return false
	}
	heredocs := scriptHeredocSpecs(state.continuedWith(value), state.descriptors)
	if len(heredocs) == 0 {
		return false
	}
	state.heredocs = heredocs
	state.visibleContinuation = false
	state.continuedCommand = ""
	return true
}

func (state *scriptParseState) consumeNewLine(
	segments []segment,
	line int,
	raw, value string,
) []segment {
	if strings.HasPrefix(value, "#") {
		return append(segments, segment{Kind: segmentProse, Line: line, Text: value})
	}
	if heredocs := scriptHeredocSpecs(value, state.descriptors); len(heredocs) > 0 {
		state.heredocs = heredocs
		return segments
	}
	assignment := stripShellDeclaration(value)
	if shellAssignment.MatchString(assignment) && strings.Contains(assignment, "$(") {
		return append(segments, segment{Kind: segmentShell, Line: line, Text: value})
	}
	if assignmentQuote := scriptAssignmentQuote(assignment); assignmentQuote != 0 {
		segments = append(segments, segment{Kind: segmentShell, Line: line, Text: value})
		if unescapedByteCount(raw, assignmentQuote)%2 == 1 {
			state.quote = assignmentQuote
		}
		return segments
	}
	if !agentVisibleScriptCommand(value, state.visibleHelpers) {
		return segments
	}
	segments = append(segments, segment{Kind: segmentShell, Line: line, Text: value})
	if commandQuote := unclosedScriptQuote(value); commandQuote != 0 {
		state.quote = commandQuote
		return segments
	}
	state.visibleContinuation = hasShellLineContinuation(raw)
	if state.visibleContinuation {
		state.continuedCommand = stripShellLineContinuation(value)
	}
	return segments
}

func (state *scriptParseState) commitHeredoc(segments []segment, heredoc scriptHeredoc) []segment {
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
		var copied []segment
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
	var propagated []segment
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
