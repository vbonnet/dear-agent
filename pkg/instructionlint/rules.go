package instructionlint

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

type rule struct {
	id          string
	replacement string
	applies     func(SegmentKind) bool
	detect      func(string) bool
}

var retiredWayfinderPhase = regexp.MustCompile(`(?i)(?:\bW0(?:\b|-)|\bD[1-6](?:\b|-)|\bS(?:1[01]|[1-9])(?:\b|-))`)
var retiredWayfinderV1 = regexp.MustCompile(`(?i)\bV1\b`)
var environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var embeddedShellCommand = regexp.MustCompile("\\$\\(([^()]*)\\)|`([^`]*)`")

var instructionRules = []rule{
	{id: "wayfinder-v1", replacement: "CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, and RETRO", applies: proseOrShell, detect: retiredWayfinderToken},
	{id: "bare-beads", replacement: "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>", applies: commandSegment, detect: bareBeads},
	{id: "raw-git-push", replacement: "safe-push", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool { return commandHasPrefix(fields, "git", "push") })
	}},
	{id: "raw-gh-merge", replacement: "safe-merge", applies: commandSegment, detect: rawGHMerge},
	{id: "raw-gh-pr-lifecycle", replacement: "safe-pr create or safe-pr close; ask a human to reopen", applies: shellSegment, detect: rawGHPRLifecycle},
	{id: "safe-pr-emergency", replacement: "safe-pr normally or run agm escalate ask <question>; there is no bypass flag", applies: commandSegment, detect: func(text string) bool {
		normalized := shellText(text)
		return strings.Contains(normalized, "safe-pr") && strings.Contains(normalized, "--emergency")
	}},
	{id: "agm-health-json", replacement: "agm -o json session health <name>", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool {
			return commandHasPrefix(fields, "agm", "session", "health") && slices.Contains(fields, "--json")
		})
	}},
	{id: "agm-root-status", replacement: "agm session status --format json", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool { return commandHasPrefix(fields, "agm", "status") })
	}},
	{id: "agm-positional-search", replacement: "agm search --query <text>", applies: commandSegment, detect: positionalAGMSearch},
	{id: "agm-root-new", replacement: "agm session new --workspace <name>", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool { return commandHasPrefix(fields, "agm", "new") })
	}},
	{id: "agm-session-send", replacement: "agm send <session> <message>", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool { return commandHasPrefix(fields, "agm", "session", "send") })
	}},
}

func retiredWayfinderToken(text string) bool {
	if retiredWayfinderPhase.MatchString(text) {
		return true
	}
	for _, location := range retiredWayfinderV1.FindAllStringIndex(text, -1) {
		precededBySlash := location[0] > 0 && text[location[0]-1] == '/'
		followedByVersionSuffix := location[1]+1 < len(text) && text[location[1]] == '.' && isVersionSuffix(text[location[1]+1])
		if !precededBySlash && !followedByVersionSuffix {
			return true
		}
	}
	return false
}

func isVersionSuffix(value byte) bool {
	return value >= '0' && value <= '9' || value == 'x' || value == 'X'
}

func knownRule(id string) bool {
	for _, candidate := range instructionRules {
		if candidate.id == id {
			return true
		}
	}
	return false
}

func evaluateSegment(path string, segment Segment) []Violation {
	var violations []Violation
	for _, candidate := range instructionRules {
		if candidate.applies(segment.Kind) && candidate.detect(segment.Text) {
			violations = append(violations, Violation{
				Path:        path,
				Line:        segment.Line,
				Rule:        candidate.id,
				Excerpt:     strings.TrimSpace(segment.Text),
				Replacement: candidate.replacement,
			})
		}
	}
	return violations
}

func proseOrShell(kind SegmentKind) bool {
	return kind == SegmentProse || kind == SegmentShell
}

func commandSegment(kind SegmentKind) bool {
	return kind == SegmentInline || kind == SegmentShell
}

func shellSegment(kind SegmentKind) bool {
	return kind == SegmentShell
}

func shellText(text string) string {
	normalized := strings.TrimSpace(text)
	normalized = strings.TrimPrefix(normalized, "$ ")
	return strings.TrimSpace(normalized)
}

func bareBeads(text string) bool {
	for _, fields := range commandFields(text) {
		if len(fields) > 0 && fields[0] == "bd" && beadsCommandIsBare(fields) {
			return true
		}
	}
	return false
}

func beadsCommandIsBare(fields []string) bool {
	const canonical = "~/beads/context-engine/.beads"
	if len(fields) >= 3 && fields[1] == "--db" {
		return strings.Trim(fields[2], `"'`) != canonical
	}
	if len(fields) >= 2 {
		if value, ok := strings.CutPrefix(fields[1], "--db="); ok {
			return strings.Trim(value, `"'`) != canonical
		}
	}
	return true
}

func rawGHMerge(text string) bool {
	for _, fields := range commandFields(text) {
		if len(fields) >= 3 && fields[0] == "gh" && fields[1] == "pr" && fields[2] == "merge" {
			return true
		}
	}
	return false
}

func rawGHPRLifecycle(text string) bool {
	return anyCommand(text, func(fields []string) bool {
		return len(fields) >= 3 && fields[0] == "gh" && fields[1] == "pr" &&
			slices.Contains([]string{"create", "close", "reopen"}, fields[2])
	})
}

func commandFields(text string) [][]string {
	return commandFieldsDepth(text, 0)
}

func commandFieldsDepth(text string, depth int) [][]string {
	var commands [][]string
	inputs := []string{shellText(text)}
	for _, match := range embeddedCommandStart.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			inputs = append(inputs, match[1])
		}
	}
	for _, match := range embeddedShellCommand.FindAllStringSubmatch(text, -1) {
		if match[1] != "" {
			inputs = append(inputs, match[1])
		} else if match[2] != "" {
			inputs = append(inputs, match[2])
		}
	}
	for _, input := range inputs {
		for _, command := range splitShellCommands(input) {
			fields := normalizeAGMCommand(stripCommandPrefixes(parseShellWords(strings.TrimSpace(command))))
			if len(fields) > 0 {
				commands = append(commands, fields)
			}
			if payload, ok := shellCommandPayload(fields); ok && depth < 4 {
				commands = append(commands, commandFieldsDepth(payload, depth+1)...)
			}
		}
	}
	return commands
}

func parseShellWords(input string) []string {
	var fields []string
	var word strings.Builder
	var quote byte
	escaped := false
	started := false
	flush := func() {
		if started {
			fields = append(fields, word.String())
			word.Reset()
			started = false
		}
	}
	for i := 0; i < len(input); i++ {
		current := input[i]
		if escaped {
			word.WriteByte(current)
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
				word.WriteByte(current)
			}
			started = true
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			started = true
			continue
		}
		if current == ' ' || current == '\t' || current == '\r' || current == '\n' {
			flush()
			continue
		}
		word.WriteByte(current)
		started = true
	}
	if escaped {
		word.WriteByte('\\')
	}
	flush()
	return fields
}

func shellCommandPayload(fields []string) (string, bool) {
	if len(fields) < 3 || !slices.Contains([]string{"bash", "dash", "ksh", "sh", "zsh"}, fields[0]) {
		return "", false
	}
	for index := 1; index+1 < len(fields); index++ {
		option := fields[index]
		if option == "-c" || option == "--command" ||
			(strings.HasPrefix(option, "-") && !strings.HasPrefix(option, "--") && strings.Contains(option[1:], "c")) {
			return fields[index+1], true
		}
		if !strings.HasPrefix(option, "-") {
			return "", false
		}
	}
	return "", false
}

func splitShellCommands(input string) []string {
	commands := make([]string, 0, 2)
	start := 0
	scanner := shellScanState{}
	appendCommand := func(end int) {
		if command := strings.TrimSpace(input[start:end]); command != "" {
			commands = append(commands, command)
		}
	}
	for i := 0; i < len(input); i++ {
		width := scanner.boundaryWidth(input, i)
		if width == 0 {
			continue
		}
		appendCommand(i)
		i += width - 1
		start = i + 1
	}
	appendCommand(len(input))
	return commands
}

type shellScanState struct {
	quote   byte
	escaped bool
}

func (state *shellScanState) boundaryWidth(input string, index int) int {
	current := input[index]
	if state.escaped {
		state.escaped = false
		return 0
	}
	if current == '\\' {
		state.escaped = true
		return 0
	}
	if state.quote != 0 {
		if current == state.quote {
			state.quote = 0
		}
		return 0
	}
	if current == '\'' || current == '"' {
		state.quote = current
		return 0
	}
	return shellOperatorWidth(input, index)
}

func shellOperatorWidth(input string, index int) int {
	current := input[index]
	switch current {
	case ';':
		return 1
	case '|':
		if index > 0 && input[index-1] == '>' {
			return 0
		}
		if index+1 < len(input) && input[index+1] == '|' {
			return 2
		}
		return 1
	case '&':
		if (index > 0 && (input[index-1] == '>' || input[index-1] == '<')) || (index+1 < len(input) && input[index+1] == '>') {
			return 0
		}
		if index+1 < len(input) && input[index+1] == '&' {
			return 2
		}
		return 1
	default:
		return 0
	}
}

func normalizeAGMCommand(fields []string) []string {
	if len(fields) == 0 || fields[0] != "agm" {
		return fields
	}
	args := stripLauncherOptions(fields[1:], map[string]bool{
		"-C": true, "--directory": true, "--config": true, "--sessions-dir": true,
		"--log-level": true, "--timeout": true, "--workspace": true,
		"-o": true, "--output": true, "--fields": true,
	})
	return append([]string{"agm"}, args...)
}

func stripCommandPrefixes(fields []string) []string {
	for len(fields) > 0 {
		fields[0] = strings.TrimLeft(fields[0], "(")
		fields[0] = executableBase(fields[0])
		switch {
		case fields[0] == "if" || fields[0] == "while" || fields[0] == "until" || fields[0] == "then" || fields[0] == "do" || fields[0] == "!":
			fields = fields[1:]
		case environmentAssignment.MatchString(fields[0]):
			fields = fields[1:]
		case fields[0] == "env":
			fields = stripLauncherOptions(fields[1:], map[string]bool{
				"-C": true, "--chdir": true, "-S": true, "--split-string": true,
				"-u": true, "--unset": true,
			})
		case fields[0] == "timeout" || fields[0] == "gtimeout":
			fields = stripTimeoutPrefix(fields[1:])
		case fields[0] == "sudo":
			fields = stripLauncherOptions(fields[1:], map[string]bool{
				"-C": true, "--close-from": true, "-D": true, "--chdir": true,
				"-g": true, "--group": true, "-h": true, "--host": true,
				"-p": true, "--prompt": true, "-R": true, "--chroot": true,
				"-T": true, "--command-timeout": true, "-u": true, "--user": true,
			})
		case fields[0] == "command" || fields[0] == "nohup":
			fields = stripLauncherOptions(fields[1:], nil)
		default:
			return fields
		}
	}
	return fields
}

func executableBase(value string) string {
	return path.Base(strings.Trim(value, `"'`))
}

func stripLauncherOptions(fields []string, consumesValue map[string]bool) []string {
	for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
		option := fields[0]
		fields = fields[1:]
		name, _, hasInlineValue := strings.Cut(option, "=")
		if consumesValue[name] && !hasInlineValue && len(fields) > 0 {
			fields = fields[1:]
		}
	}
	return fields
}

func stripTimeoutPrefix(fields []string) []string {
	fields = stripLauncherOptions(fields, map[string]bool{
		"-k": true, "--kill-after": true, "-s": true, "--signal": true,
	})
	if len(fields) > 0 {
		fields = fields[1:]
	}
	return fields
}

func positionalAGMSearch(text string) bool {
	return anyCommand(text, func(fields []string) bool {
		return len(fields) > 2 && commandHasPrefix(fields, "agm", "search") && !strings.HasPrefix(fields[2], "-")
	})
}

func anyCommand(text string, predicate func([]string) bool) bool {
	return slices.ContainsFunc(commandFields(text), predicate)
}

func commandHasPrefix(fields []string, prefix ...string) bool {
	if len(fields) < len(prefix) {
		return false
	}
	for i, want := range prefix {
		if strings.Trim(fields[i], "()") != want {
			return false
		}
	}
	return true
}
