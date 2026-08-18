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
var unambiguousRetiredWayfinderPhase = regexp.MustCompile(`(?i)(?:\bW0-|\bD[1-6]-|\bS(?:1[01]|[1-9])-)`)
var retiredWayfinderV1 = regexp.MustCompile(`(?i)\bV1\b`)
var environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var embeddedShellCommand = regexp.MustCompile("\\$\\(([^()]*)\\)|`([^`]*)`")
var markdownListPrefix = regexp.MustCompile(`^(?:[-+*]|[0-9]+[.)])[ \t]+`)
var markdownTaskPrefix = regexp.MustCompile(`^\[[ xX]\][ \t]+`)
var pullRequestAPIEndpoint = regexp.MustCompile(`^/?repos/[^/]+/[^/]+/pulls(?:/([^/]+))?/?$`)

var instructionRules = []rule{
	{id: "wayfinder-v1", replacement: "CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, and RETRO", applies: proseOrShell, detect: retiredWayfinderToken},
	{id: "bare-beads", replacement: "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>", applies: commandSegment, detect: bareBeads},
	{id: "raw-git-push", replacement: "safe-push", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool { return commandHasPrefix(fields, "git", "push") })
	}},
	{id: "raw-gh-merge", replacement: "safe-merge", applies: commandSegment, detect: rawGHMerge},
	{id: "raw-gh-pr-lifecycle", replacement: "safe-pr create or safe-pr close; ask a human to reopen", applies: commandSegment, detect: rawGHPRLifecycle},
	{id: "safe-pr-emergency", replacement: "safe-pr normally or run agm escalate ask <question>; there is no bypass flag", applies: commandSegment, detect: func(text string) bool {
		normalized := shellText(text)
		return strings.Contains(normalized, "safe-pr") && strings.Contains(normalized, "--emergency")
	}},
	{id: "agm-escalate-obsolete-flags", replacement: "agm escalate ask --kind blocked-action --context <reason> <question>", applies: commandSegment, detect: func(text string) bool {
		return anyCommand(text, func(fields []string) bool {
			return commandHasPrefix(fields, "agm", "escalate") && slices.ContainsFunc(fields, func(field string) bool {
				return field == "--action" || strings.HasPrefix(field, "--action=") ||
					field == "--reason" || strings.HasPrefix(field, "--reason=")
			})
		})
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
	wayfinderContext := strings.Contains(strings.ToLower(text), "wayfinder")
	if unambiguousRetiredWayfinderPhase.MatchString(text) || wayfinderContext && retiredWayfinderPhase.MatchString(text) {
		return true
	}
	return wayfinderContext && retiredWayfinderV1.MatchString(text)
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
		if candidate.applies(segment.Kind) && ruleDetects(candidate, path, segment.Text) {
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

func ruleDetects(candidate rule, sourcePath, text string) bool {
	if candidate.detect(text) {
		return true
	}
	return candidate.id == "wayfinder-v1" &&
		strings.Contains(strings.ToLower(sourcePath), "wayfinder") &&
		unqualifiedRetiredWayfinderV1(text)
}

func unqualifiedRetiredWayfinderV1(text string) bool {
	for _, location := range retiredWayfinderV1.FindAllStringIndex(text, -1) {
		if location[0] == 0 || text[location[0]-1] != '/' {
			return true
		}
	}
	return false
}

func proseOrShell(kind SegmentKind) bool {
	return kind == SegmentProse || kind == SegmentShell
}

func commandSegment(kind SegmentKind) bool {
	return kind == SegmentInline || kind == SegmentShell
}

func shellText(text string) string {
	normalized := stripMarkdownContainerPrefixes(text)
	normalized = strings.TrimPrefix(normalized, "$ ")
	return strings.TrimSpace(normalized)
}

func stripMarkdownContainerPrefixes(text string) string {
	normalized := strings.TrimSpace(text)
	for {
		previous := normalized
		if withoutQuote, found := strings.CutPrefix(normalized, ">"); found {
			normalized = strings.TrimSpace(withoutQuote)
		}
		normalized = markdownListPrefix.ReplaceAllString(normalized, "")
		normalized = markdownTaskPrefix.ReplaceAllString(normalized, "")
		normalized = strings.TrimSpace(normalized)
		if normalized == previous {
			return normalized
		}
	}
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
	return len(fields) < 5 ||
		fields[1] != "--db" || strings.Trim(fields[2], `"'`) != canonical ||
		fields[3] != "--dolt-auto-commit" || fields[4] != "on"
}

func rawGHMerge(text string) bool {
	return anyCommand(text, rawGHMergeFields)
}

func rawGHMergeFields(fields []string) bool {
	if commandHasPrefix(fields, "gh", "pr", "merge") {
		return true
	}
	if !commandHasPrefix(fields, "gh", "api") {
		return false
	}
	apiArgs := stripLauncherOptions(fields[2:], map[string]bool{
		"-X": true, "--method": true, "-H": true, "--header": true,
		"-q": true, "--jq": true, "-F": true, "--field": true,
		"-f": true, "--raw-field": true, "--input": true,
		"--template": true, "-t": true, "--preview": true,
	})
	if len(apiArgs) == 0 {
		return false
	}
	if strings.HasSuffix(strings.TrimSuffix(apiArgs[0], "/"), "/merge") {
		return ghAPIMethod(fields[2:]) == "PUT"
	}
	return apiArgs[0] == "graphql" &&
		(strings.Contains(strings.Join(fields, " "), "mergePullRequest") ||
			strings.Contains(strings.Join(fields, " "), "enablePullRequestAutoMerge"))
}

func rawGHPRLifecycle(text string) bool {
	return anyCommand(text, rawGHPRLifecycleFields)
}

func rawGHPRLifecycleFields(fields []string) bool {
	if len(fields) >= 3 && fields[0] == "gh" && fields[1] == "pr" &&
		slices.Contains([]string{"create", "close", "reopen"}, fields[2]) {
		return true
	}
	if !commandHasPrefix(fields, "gh", "api") {
		return false
	}
	return rawGHAPIPRLifecycle(fields[2:])
}

func rawGHAPIPRLifecycle(apiFields []string) bool {
	apiArgs := stripLauncherOptions(apiFields, map[string]bool{
		"-X": true, "--method": true, "-H": true, "--header": true,
		"-q": true, "--jq": true, "-F": true, "--field": true,
		"-f": true, "--raw-field": true, "--input": true,
		"--template": true, "-t": true, "--preview": true,
	})
	if len(apiArgs) == 0 {
		return false
	}
	if apiArgs[0] == "graphql" {
		query := strings.Join(apiFields, " ")
		return strings.Contains(query, "createPullRequest") ||
			strings.Contains(query, "closePullRequest") ||
			strings.Contains(query, "reopenPullRequest") ||
			strings.Contains(query, "updatePullRequest") &&
				(strings.Contains(query, "state:OPEN") || strings.Contains(query, "state:CLOSED"))
	}

	match := pullRequestAPIEndpoint.FindStringSubmatch(strings.Trim(apiArgs[0], `"'`))
	if match == nil {
		return false
	}
	method := ghAPIMethod(apiFields)
	if match[1] == "" {
		return method == "POST"
	}
	return method == "PATCH" && ghAPIHasLifecycleState(apiFields)
}

func ghAPIMethod(fields []string) string {
	method := ""
	hasFields := false
	for i := 0; i < len(fields); i++ {
		field := strings.Trim(fields[i], `"'`)
		if strings.HasPrefix(field, "-X") && field != "-X" {
			method = strings.TrimPrefix(field, "-X")
			continue
		}
		name, value, inline := strings.Cut(field, "=")
		switch name {
		case "-X", "--method":
			if inline {
				method = value
			} else if i+1 < len(fields) {
				i++
				method = strings.Trim(fields[i], `"'`)
			}
		case "-f", "--raw-field", "-F", "--field":
			hasFields = true
		}
	}
	if method != "" {
		return strings.ToUpper(method)
	}
	if hasFields {
		return "POST"
	}
	return "GET"
}

func ghAPIHasLifecycleState(fields []string) bool {
	for _, field := range fields {
		field = strings.ToLower(strings.Trim(fieldsValue(field), `"'`))
		if field == "state=open" || field == "state=closed" {
			return true
		}
	}
	return false
}

func fieldsValue(field string) string {
	for _, prefix := range []string{"-f=", "-F=", "--raw-field=", "--field="} {
		if value, ok := strings.CutPrefix(field, prefix); ok {
			return value
		}
	}
	return field
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
	for _, match := range embeddedProseCommand.FindAllStringSubmatch(text, -1) {
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
			fields := normalizeCommand(stripCommandPrefixes(parseShellWords(strings.TrimSpace(command))))
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

func normalizeCommand(fields []string) []string {
	fields = normalizeAGMCommand(fields)
	fields = normalizeGitCommand(fields)
	return normalizeGHCommand(fields)
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
	if len(fields) > 1 && fields[0] == "eval" {
		return strings.Join(fields[1:], " "), true
	}
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

func normalizeGitCommand(fields []string) []string {
	if len(fields) == 0 || fields[0] != "git" {
		return fields
	}
	args := stripLauncherOptions(fields[1:], map[string]bool{
		"-C": true, "-c": true, "--config-env": true, "--exec-path": true,
		"--git-dir": true, "--namespace": true, "--super-prefix": true, "--work-tree": true,
	})
	return append([]string{"git"}, args...)
}

func normalizeGHCommand(fields []string) []string {
	if len(fields) == 0 || fields[0] != "gh" {
		return fields
	}
	args := stripLauncherOptions(fields[1:], map[string]bool{
		"-R": true, "--hostname": true, "--repo": true,
	})
	return append([]string{"gh"}, args...)
}

func stripCommandPrefixes(fields []string) []string {
	for len(fields) > 0 {
		fields = trimShellGroupPrefixes(fields)
		fields[0] = executableBase(fields[0])
		var stripped bool
		fields, stripped = stripCommandPrefix(fields)
		if !stripped {
			return fields
		}
	}
	return fields
}

func stripCommandPrefix(fields []string) ([]string, bool) {
	command := fields[0]
	switch {
	case command == "if" || command == "while" || command == "until" || command == "then" || command == "do" || command == "!":
		return fields[1:], true
	case environmentAssignment.MatchString(command):
		return fields[1:], true
	case command == "env":
		return stripEnvPrefix(fields[1:]), true
	case command == "timeout" || command == "gtimeout":
		return stripTimeoutPrefix(fields[1:]), true
	case command == "sudo":
		return stripLauncherOptions(fields[1:], map[string]bool{
			"-C": true, "--close-from": true, "-D": true, "--chdir": true,
			"-g": true, "--group": true, "-h": true, "--host": true,
			"-p": true, "--prompt": true, "-R": true, "--chroot": true,
			"-T": true, "--command-timeout": true, "-u": true, "--user": true,
		}), true
	case command == "command" || command == "nohup":
		return stripLauncherOptions(fields[1:], nil), true
	case command == "exec":
		return stripLauncherOptions(fields[1:], map[string]bool{"-a": true}), true
	default:
		return fields, false
	}
}

func stripEnvPrefix(fields []string) []string {
	for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
		option := fields[0]
		fields = fields[1:]
		name, inlineValue, hasInlineValue := strings.Cut(option, "=")
		if name == "-S" || name == "--split-string" {
			payload := inlineValue
			if !hasInlineValue {
				if len(fields) == 0 {
					return fields
				}
				payload = fields[0]
				fields = fields[1:]
			}
			return append(parseShellWords(payload), fields...)
		}
		if (name == "-C" || name == "--chdir" || name == "-u" || name == "--unset") &&
			!hasInlineValue && len(fields) > 0 {
			fields = fields[1:]
		}
	}
	return fields
}

func trimShellGroupPrefixes(fields []string) []string {
	for len(fields) > 1 {
		fields[0] = strings.TrimLeft(fields[0], "({")
		if fields[0] != "" {
			return fields
		}
		fields = fields[1:]
	}
	fields[0] = strings.TrimLeft(fields[0], "({")
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
