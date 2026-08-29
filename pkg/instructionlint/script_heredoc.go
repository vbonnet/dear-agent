package instructionlint

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

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
