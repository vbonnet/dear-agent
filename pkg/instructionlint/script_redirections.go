package instructionlint

import (
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

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
