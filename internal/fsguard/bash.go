package fsguard

import (
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

// Shell control operators that separate one simple command from the next.
var separators = map[string]bool{
	";": true, "&&": true, "||": true, "|": true, "&": true,
	"(": true, ")": true, "{": true, "}": true,
}

// Redirection operators whose following token names the redirection target.
var redirOps = map[string]bool{
	">": true, ">>": true, "&>": true, "&>>": true, ">&": true, ">>&": true,
}

// Commands whose destination is the last non-option argument (sources are
// read-only).
var destLast = map[string]bool{
	"cp": true, "rsync": true, "install": true, "ln": true, "link": true,
}

// Commands where every non-option positional is a write/delete target. mv is
// here because it removes its sources as well as writing the destination.
var destAll = map[string]bool{
	"mkdir": true, "touch": true, "rm": true, "rmdir": true, "unlink": true,
	"shred": true, "mktemp": true, "mv": true,
}

// Commands whose every path-shaped positional is a mutation target.
var permsCmds = map[string]bool{
	"chmod": true, "chown": true, "chgrp": true, "truncate": true,
}

// git subcommands that only read; always allowed.
var gitReadonly = map[string]bool{
	"log": true, "diff": true, "status": true, "show": true, "blame": true,
	"cat-file": true, "ls-files": true, "ls-tree": true, "ls-remote": true,
	"rev-parse": true, "rev-list": true, "for-each-ref": true, "grep": true,
	"shortlog": true, "show-ref": true, "show-branch": true, "symbolic-ref": true,
	"describe": true, "remote": true, "config": true, "count-objects": true,
	"check-ignore": true, "merge-base": true, "name-rev": true,
	"whatchanged": true, "reflog": true, "annotate": true, "verify-commit": true,
}

// git write subcommands that are nonetheless permitted inside ~/src: they sync
// the golden checkout without rewriting its working tree or history.
var gitAllowedWriteInSrc = map[string]bool{
	"merge": true, "pull": true, "fetch": true, "clone": true,
	"worktree": true, "push": true,
}

// Command runners that prefix and exec another command. They must be stripped
// to reach the real command word, else `sudo rm ~/src/f` or `env rm ~/src/f`
// would slip past the per-command analysis.
var commandRunners = map[string]bool{
	"env": true, "sudo": true, "doas": true, "nohup": true, "setsid": true,
	"exec": true, "time": true, "nice": true, "ionice": true, "stdbuf": true,
	"command": true, "builtin": true,
}

// Shells whose `-c SCRIPT` argument is itself a command to inspect recursively.
var shellCmds = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true,
}

// maxShellDepth bounds recursive inspection of `bash -c`/`eval` nesting so a
// pathological command can never loop forever; beyond it we fail open and let
// the settings.json deny rules be the backstop.
const maxShellDepth = 8

// punctuation chars that, in a run, form a standalone operator token. Mirrors
// Python shlex's punctuation_chars default set.
const punctuation = ";|&<>()"

// tokenize splits a shell command into tokens, keeping operator runs as their
// own tokens and preserving statement boundaries across newlines (each
// physical line is tokenized independently with an explicit ";" inserted
// between lines, so one statement's arguments are never misattributed to the
// next). It returns ok=false on unterminated quotes so the caller can fail
// open.
func tokenize(command string) (tokens []string, ok bool) {
	// Merge backslash-newline line continuations first: otherwise splitting on
	// "\n" would scatter a single statement (e.g. `rm \<newline> ~/src/f`)
	// across lines and let the write target slip past the per-segment check.
	command = strings.ReplaceAll(command, "\\\n", "")
	for _, line := range strings.Split(command, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lineToks, lineOK := tokenizeLine(line)
		if !lineOK {
			return nil, false
		}
		if len(tokens) > 0 && len(lineToks) > 0 {
			tokens = append(tokens, ";")
		}
		tokens = append(tokens, lineToks...)
	}
	return tokens, true
}

func tokenizeLine(line string) (tokens []string, ok bool) {
	var cur strings.Builder
	haveWord := false
	flush := func() {
		if haveWord {
			tokens = append(tokens, cur.String())
			cur.Reset()
			haveWord = false
		}
	}

	runes := []rune(line)
	for i := 0; i < len(runes); {
		c := runes[i]
		switch {
		case c == ' ' || c == '\t':
			flush()
			i++
		case c == '\'' || c == '"':
			content, next, qok := scanQuote(runes, i)
			if !qok {
				return nil, false // unterminated quote
			}
			cur.WriteString(content)
			haveWord = true
			i = next
		case c == '\\':
			if i+1 < len(runes) {
				cur.WriteRune(runes[i+1])
				haveWord = true
				i += 2
			} else {
				i++
			}
		case strings.ContainsRune(punctuation, c):
			flush()
			j := i
			for j < len(runes) && strings.ContainsRune(punctuation, runes[j]) {
				j++
			}
			tokens = append(tokens, string(runes[i:j]))
			i = j
		default:
			cur.WriteRune(c)
			haveWord = true
			i++
		}
	}
	flush()
	return tokens, true
}

// scanQuote reads a single- or double-quoted run starting at runes[start]
// (which must be the opening quote) and returns the unquoted content, the
// index just past the closing quote, and ok=false if the quote is never
// closed. Backslash escapes are honoured inside double quotes only.
func scanQuote(runes []rune, start int) (content string, next int, ok bool) {
	quote := runes[start]
	var b strings.Builder
	j := start + 1
	for j < len(runes) && runes[j] != quote {
		if quote == '"' && runes[j] == '\\' && j+1 < len(runes) {
			b.WriteRune(runes[j+1])
			j += 2
			continue
		}
		b.WriteRune(runes[j])
		j++
	}
	if j >= len(runes) {
		return "", 0, false
	}
	return b.String(), j + 1, true
}

// splitSegments groups tokens into simple-command segments, dropping the
// control operators that separate them.
func splitSegments(tokens []string) [][]string {
	var segments [][]string
	var current []string
	for _, tok := range tokens {
		if separators[tok] {
			if len(current) > 0 {
				segments = append(segments, current)
				current = nil
			}
			continue
		}
		current = append(current, tok)
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

func isOption(tok string) bool {
	return strings.HasPrefix(tok, "-") && tok != "-" && tok != "--"
}

// pathLike reports tokens worth path-classifying: explicit paths, not bare
// scalars like 755. Restricting to path-shaped tokens avoids misreading option
// values (the 755 in `mkdir -m 755 dir`) as write targets. The environment
// convention is absolute / ~ paths, so bare in-cwd filenames are intentionally
// out of scope.
func pathLike(tok string) bool {
	return strings.Contains(tok, "/") ||
		strings.HasPrefix(tok, "~") ||
		strings.HasPrefix(tok, "$HOME") ||
		tok == "." || tok == ".."
}

// pathyArgs returns the path-shaped, non-option arguments, preserving order.
func pathyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if !isOption(a) && pathLike(a) {
			out = append(out, a)
		}
	}
	return out
}

// realArgs strips leading VAR=value environment assignments (e.g. the FOO=bar
// in `FOO=bar cmd`) so the actual command word lands at index 0.
func realArgs(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	started := false
	for _, tok := range tokens {
		if !started && !isOption(tok) && isEnvAssignment(tok) {
			continue
		}
		started = true
		out = append(out, tok)
	}
	return out
}

// stripRunners peels off leading command-runner prefixes (env, sudo, nohup,
// setsid, exec, ...) along with their options/assignments so the returned
// slice begins with the actual command word. `sudo -u root rm ~/src/f` and
// `env FOO=bar rm ~/src/f` both reduce to `rm ~/src/f`.
func stripRunners(args []string) []string {
	for len(args) > 0 && commandRunners[args[0]] {
		runner := args[0]
		args = args[1:]
		for len(args) > 0 {
			a := args[0]
			if a == "--" {
				args = args[1:]
				break
			}
			if isOption(a) {
				args = args[1:]
				if runnerOptTakesValue(runner, a) && len(args) > 0 {
					args = args[1:]
				}
				continue
			}
			if runner == "env" && isEnvAssignment(a) {
				args = args[1:]
				continue
			}
			break
		}
	}
	return args
}

// runnerOptTakesValue reports whether a runner option consumes the following
// token as its value (so the value is not mistaken for the command word).
func runnerOptTakesValue(runner, opt string) bool {
	switch runner {
	case "sudo", "doas":
		return opt == "-u" || opt == "--user" || opt == "-g" || opt == "--group"
	case "nice":
		return opt == "-n" || opt == "--adjustment"
	case "ionice":
		return opt == "-c" || opt == "-n" || opt == "-p"
	default:
		return false
	}
}

// shellWrapped reports whether cmd runs another command supplied as a string
// (a shell's `-c SCRIPT`, or `eval ...`), returning that nested command so the
// caller can inspect it recursively.
func shellWrapped(cmd string, rest []string) (nested string, ok bool) {
	if cmd == "eval" {
		return strings.Join(rest, " "), len(rest) > 0
	}
	if shellCmds[filepath.Base(cmd)] {
		for i, a := range rest {
			if a == "-c" && i+1 < len(rest) {
				return rest[i+1], true
			}
		}
	}
	return "", false
}

func isEnvAssignment(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range tok[:eq] {
		valid := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9' && i > 0)
		if !valid {
			return false
		}
	}
	return true
}

// writeTargets returns the filesystem paths a write-command would mutate. Only
// path-shaped positionals are considered, so option values are never mistaken
// for targets.
func writeTargets(cmd string, rest []string) []string {
	switch {
	case destAll[cmd] || cmd == "tee":
		return pathyArgs(rest)
	case destLast[cmd]:
		// cp/mv/rsync/install/ln SRC... DEST -> the destination is last.
		pathy := pathyArgs(rest)
		if len(pathy) == 0 {
			return nil
		}
		return pathy[len(pathy)-1:]
	case permsCmds[cmd]:
		return pathyArgs(rest)
	case cmd == "dd":
		return ddTargets(rest)
	case cmd == "sed" || cmd == "gsed":
		return sedTargets(rest)
	case cmd == "perl":
		return perlTargets(rest)
	default:
		return nil
	}
}

// ddTargets returns dd's output file, named by its of= operand.
func ddTargets(rest []string) []string {
	var out []string
	for _, a := range rest {
		if strings.HasPrefix(a, "of=") {
			out = append(out, a[len("of="):])
		}
	}
	return out
}

// perlTargets returns perl's in-place edit targets — only when -i is present.
func perlTargets(rest []string) []string {
	for _, a := range rest {
		if a == "-i" {
			return pathyArgs(rest)
		}
	}
	return nil
}

// sedTargets returns the files a `sed` invocation would rewrite in place.
// Only `sed -i` mutates files; the script itself must never be mistaken for a
// path (a substitution like `s/a/b/` is path-shaped — it contains slashes —
// but names no file). The script arrives either inline as the first positional
// (no -e/-f) or via -e SCRIPT / -f FILE options, whose values are skipped.
func sedTargets(rest []string) []string {
	inPlace, scriptViaOpt, positionals := parseSed(rest)
	if !inPlace {
		return nil
	}
	if !scriptViaOpt && len(positionals) > 0 {
		positionals = positionals[1:] // drop the inline script positional
	}
	out := positionals[:0]
	for _, a := range positionals {
		if pathLike(a) {
			out = append(out, a)
		}
	}
	return out
}

func parseSed(rest []string) (inPlace, scriptViaOpt bool, positionals []string) {
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "-i" || strings.HasPrefix(a, "-i"):
			inPlace = true
		case a == "-e" || a == "-f" || a == "--expression" || a == "--file":
			scriptViaOpt = true
			i++ // the script / script-file is the next token, not a target
		case strings.HasPrefix(a, "-e") || strings.HasPrefix(a, "-f"):
			scriptViaOpt = true // glued form, e.g. -es/a/b/
		case isOption(a):
			// other flag, ignore
		default:
			positionals = append(positionals, a)
		}
	}
	return inPlace, scriptViaOpt, positionals
}

// parseGit splits a git invocation's arguments (everything after the literal
// "git") into the -C directory override (if any), the subcommand, and its
// arguments.
func parseGit(args []string) (cFlag, sub string, subArgs []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if (a == "-C" || a == "--git-dir" || a == "--work-tree") && i+1 < len(args) {
			if a == "-C" {
				cFlag = args[i+1]
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return cFlag, a, args[i+1:]
	}
	return cFlag, "", nil
}

// checkGit applies the ~/src-specific git rules to a git invocation's
// arguments (everything after the literal "git"). currentDir is the effective
// working directory, used when the command has no -C flag.
func (g *Guard) checkGit(args []string, currentDir string) (allowed bool, message string) {
	cFlag, sub, subArgs := parseGit(args)
	if sub == "" {
		return true, ""
	}

	repoDir := g.expand(currentDir, currentDir)
	if cFlag != "" {
		repoDir = g.expand(cFlag, currentDir)
	}
	src := filepath.Join(g.Home, "src")
	if !under(repoDir, src) {
		return true, "" // outside ~/src: not this hook's concern
	}
	if gitReadonly[sub] {
		return true, ""
	}

	repo := g.repoName(repoDir, src)
	if sub == "push" {
		if target, blocked := safegit.ForcePushViolation(repoDir, "", subArgs); blocked {
			return false, "You're trying to force-push protected/default branch `" + target +
				"` in ~/src. Use normal PR/merge flow for main/master/default; " +
				"force-with-lease is only allowed for non-default PR branches."
		}
		return true, ""
	}
	if gitAllowedWriteInSrc[sub] {
		return true, ""
	}
	return false, "You're trying to run `git " + sub + "` in ~/src, which is " +
		"protected (only merge / pull / fetch / push / worktree are allowed " +
		"there). Create a worktree first: git -C ~/src/" + repo +
		" worktree add ~/worktrees/" + repo + "/{branch} -b {branch}, then run " +
		"git in the worktree."
}

// InspectCommand parses a Bash command for filesystem-mutating operations and
// returns the first policy violation as (false, guidance). A command that
// trips no write pattern — pure reads like cat/grep/ls/git log — is allowed.
// cwd anchors relative paths and `cd` tracking.
func (g *Guard) InspectCommand(command, cwd string) (allowed bool, message string) {
	return g.inspect(command, cwd, 0)
}

func (g *Guard) inspect(command, cwd string, depth int) (allowed bool, message string) {
	if depth > maxShellDepth {
		return true, "" // runaway nesting -> fail open, deny rules backstop
	}
	if strings.TrimSpace(command) == "" {
		return true, ""
	}
	tokens, ok := tokenize(command)
	if !ok {
		return true, "" // unparseable -> defer to deny rules (fail open)
	}
	if allowed, msg := g.checkRedirections(tokens, cwd); !allowed {
		return false, msg
	}
	return g.checkSegments(tokens, cwd, depth)
}

// checkRedirections classifies the target of every redirection operator,
// skipping pure file-descriptor duplications such as `2>&1` or `>&2`.
func (g *Guard) checkRedirections(tokens []string, cwd string) (allowed bool, message string) {
	for idx, tok := range tokens {
		if !redirOps[tok] || idx+1 >= len(tokens) {
			continue
		}
		target := tokens[idx+1]
		if separators[target] || strings.HasPrefix(target, "&") ||
			isAllDigits(target) || !pathLike(target) {
			continue
		}
		if ok, msg := g.Classify(target, cwd); !ok {
			return false, msg
		}
	}
	return true, ""
}

// checkSegments runs per-simple-command write analysis, tracking `cd` so a
// chained `cd ~/src/repo && git commit` is attributed to the right directory.
func (g *Guard) checkSegments(tokens []string, cwd string, depth int) (allowed bool, message string) {
	currentDir := g.expand(cwd, cwd)
	for _, segment := range splitSegments(tokens) {
		args := stripRunners(realArgs(segment))
		if len(args) == 0 {
			continue
		}
		cmd := args[0]

		if cmd == "cd" && len(args) > 1 {
			currentDir = g.expand(args[1], currentDir)
			continue
		}
		if cmd == "git" {
			if ok, msg := g.checkGit(args[1:], currentDir); !ok {
				return false, msg
			}
			continue
		}
		if cmd == "gh" {
			if ok, msg := checkGh(args[1:]); !ok {
				return false, msg
			}
			continue
		}
		if nested, ok := shellWrapped(cmd, args[1:]); ok {
			if allowed, msg := g.inspect(nested, currentDir, depth+1); !allowed {
				return false, msg
			}
			continue
		}
		for _, target := range writeTargets(cmd, args[1:]) {
			if ok, msg := g.Classify(target, currentDir); !ok {
				return false, msg
			}
		}
	}
	return true, ""
}

// ghMergeBlocked is the teaching message returned when a gh merge attempt is
// detected. It directs agents to the safe-merge atomic wrapper (AGENTS.md
// principle 9) instead of the raw gh command.
const ghMergeBlocked = "You're trying to merge a PR directly with gh. " +
	"Raw `gh pr merge` is denied — use the safe-merge wrapper instead, " +
	"which enforces all CI gates, soak time, and bot review before merging:\n\n" +
	"  safe-merge --pr <number> [--repo owner/repo] [--watch] [--dry-run]\n\n" +
	"safe-merge is in ~/go/bin/safe-merge (build: go install ./cmd/safe-merge). " +
	"It blocks on all required CI checks, unresolved review threads, soak time ≥5 min, " +
	"and the Gemini bot review — the raw gh call bypasses all of these."

// ghAPIFlagTakesValue reports whether a gh api flag consumes the following
// token as its value, so the value is not mistaken for the endpoint path.
// Boolean flags (--paginate / -p, --silent, --include, etc.) are NOT listed
// here — they stand alone and must not consume the next token.
// Note: --preview takes a name value and IS listed here.
func ghAPIFlagTakesValue(flag string) bool {
	switch flag {
	case "-X", "--method",
		"-H", "--header",
		"-q", "--jq",
		"-F", "--field",
		"-f", "--raw-field",
		"--input",
		"--template", "-t",
		"--preview":
		return true
	}
	return false
}

// checkGh blocks direct PR merge operations via the gh CLI, directing agents
// to the safe-merge atomic wrapper. It catches three bypass vectors:
//
//  1. gh pr merge ...        — the direct merge subcommand.
//  2. gh api repos/.../pulls/.../merge — REST PUT merge endpoint.
//  3. gh api graphql with mergePullRequest/enablePullRequestAutoMerge mutations.
func checkGh(args []string) (allowed bool, message string) {
	if len(args) == 0 {
		return true, ""
	}

	// gh pr merge ...
	if len(args) >= 2 && args[0] == "pr" && args[1] == "merge" {
		return false, ghMergeBlocked
	}

	if args[0] != "api" {
		return true, ""
	}

	// Walk args after "api", skipping flags and their values, to find
	// the endpoint path or "graphql" verb.
	apiPath := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if strings.HasPrefix(a, "-") {
			if ghAPIFlagTakesValue(a) {
				i++ // skip the flag's value token
			}
			continue
		}
		apiPath = a
		break
	}

	// gh api repos/<owner>/<repo>/pulls/<number>/merge
	if strings.HasSuffix(apiPath, "/merge") {
		return false, ghMergeBlocked
	}

	// gh api graphql with a merge mutation anywhere in the argument list.
	if apiPath == "graphql" {
		full := strings.Join(args, " ")
		if strings.Contains(full, "mergePullRequest") || strings.Contains(full, "enablePullRequestAutoMerge") {
			return false, ghMergeBlocked
		}
	}

	return true, ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
