package fsguard

import (
	"os"
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

// inputRedirOps read from their target instead of writing to it. They are never
// classified as write targets, but they and their target must still be removed
// from a command's operands: `cp a ~/src/<repo>/f < in` would otherwise pick
// `in` as cp's destination and miss the protected write.
var inputRedirOps = map[string]bool{
	"<": true, "<<": true, "<<<": true, "|&": true,
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

// shellOperators are the operator tokens recognized inside a run of punctuation
// characters, ordered longest-first so maximal munch splits a run correctly.
var shellOperators = []string{
	"&>>", ">>&", "<<<",
	"&&", "||", ";;", ">>", "<<", "&>", ">&", "|&",
	";", "|", "&", "(", ")", "<", ">",
}

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
		case c == '#' && !haveWord:
			// A '#' that opens a word starts a comment: Bash discards the rest
			// of the line, so those words must not become operands (otherwise
			// `cp a ~/src/f # backup` would read `backup` as the destination).
			// Inside a word (`file#1`) it is an ordinary character.
			i = len(runes)
		case strings.ContainsRune(punctuation, c):
			// A digit word glued straight onto a redirection operator is that
			// redirection's file descriptor (`2>&1`), not an operand. Keeping
			// the two in one token preserves the adjacency the shell relies on,
			// so `rm 2 > log` — where `2` really is a file operand — stays
			// distinguishable after tokenization.
			fd := ""
			if haveWord && (c == '>' || c == '<') && isAllDigits(cur.String()) {
				fd = cur.String()
				cur.Reset()
				haveWord = false
			}
			flush()
			j := i
			for j < len(runes) && strings.ContainsRune(punctuation, runes[j]) {
				j++
			}
			// Split the run into individual operators by maximal munch: a run
			// like `);` is two operators, and emitting it whole would hide the
			// `)` from subshell tracking and the `;` from segment splitting.
			for run := string(runes[i:j]); run != ""; {
				op := run
				for _, cand := range shellOperators {
					if strings.HasPrefix(run, cand) {
						op = cand
						break
					}
				}
				tokens = append(tokens, fd+op)
				fd = "" // the descriptor belongs to the first operator only
				run = run[len(op):]
			}
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

// isRedirOp reports whether tok is a redirection operator, allowing the
// file-descriptor prefix the tokenizer keeps glued on (`2>&1` -> `2>&`).
func isRedirOp(tok string) bool {
	return redirOps[stripFDPrefix(tok)]
}

// isInputRedirOp reports whether tok is an input redirection, with or without
// a file-descriptor prefix. The tokenizer keeps `3<` as one token, so matching
// only the bare operators left `cp a ~/src/<repo>/f 3< in` with `in` as the
// apparent destination and the protected write unexamined.
func isInputRedirOp(tok string) bool {
	return inputRedirOps[stripFDPrefix(tok)]
}

// stripFDPrefix removes a leading file-descriptor number from a redirection
// token, so `2>`, `3<`, and `>` all reduce to their operator.
func stripFDPrefix(tok string) string {
	i := 0
	for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
		i++
	}
	return tok[i:]
}

// segment is one simple command plus the control operator that introduced it.
// The operator is what tells checkSegments whether a `cd` in this segment is
// certain to have run.
type segment struct {
	tokens []string
	// sep is the control operator immediately before this segment ("" for the
	// first) and nextSep the one immediately after it (""" for the last).
	// `&&` and `||` make a segment conditional; `|` on either side puts it in
	// a pipeline, whose subshell directory change does not outlive it.
	sep     string
	nextSep string
}

// conditional reports whether this segment may not execute, or executes in a
// subshell, so a `cd` in it cannot be assumed to have moved the shell.
func (s segment) conditional() bool {
	return s.sep == "&&" || s.sep == "||" || s.sep == "|" || s.nextSep == "|"
}

// splitSegments groups tokens into simple-command segments, recording the
// control operator that separated each from the previous one. Subshell
// parentheses survive as single-token marker segments so checkSegments can
// restore the tracked working directory on `)` — a `cd` inside `( … )` does
// not outlive it.
func splitSegments(tokens []string) []segment {
	var segments []segment
	var current []string
	pending := ""
	flush := func() {
		if len(current) > 0 {
			segments = append(segments, segment{tokens: current, sep: pending})
			current = nil
			pending = ""
		}
	}
	for _, tok := range tokens {
		if separators[tok] {
			flush()
			if tok == "(" || tok == ")" {
				segments = append(segments, segment{tokens: []string{tok}})
				pending = ""
				continue
			}
			pending = tok
			continue
		}
		current = append(current, tok)
	}
	flush()
	// Record each segment's trailing operator: a `cd` on the left of a pipe
	// runs in a subshell just as surely as one on the right.
	for i := range segments[:max(len(segments)-1, 0)] {
		segments[i].nextSep = segments[i+1].sep
	}
	return segments
}

func isOption(tok string) bool {
	return strings.HasPrefix(tok, "-") && tok != "-" && tok != "--"
}

// pathLike reports tokens that are unambiguously path-shaped: absolute, ~, or
// $HOME paths and the . / .. specials. Redirection and sed/perl target
// detection use it to skip fd-dups and inline scripts. Write-command target
// detection no longer relies on it alone — bare relative names (e.g.
// `rm AGENTS.md`) are also targets, resolved against cwd (see targetOperands).
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

// optsWithValue lists, per normalized command basename, the options that
// consume the following token as a value, so a scalar value is never mistaken
// for a write target (e.g. the 755 in `mkdir -m 755 dir`). Only the
// space-separated form leaks a value as a positional; the glued (`-m755`) and
// `=`-joined (`--mode=755`) forms are single option tokens already dropped by
// isOption. Reference paths (-r/--reference) are read-only sources, so skipping
// them here is also correct.
// GNU coreutils accept value-taking options after the operands too
// (`cp SRC DEST --suffix bak`), so an unconsumed value would otherwise become
// the "last" operand and displace the real destination.
var optsWithValue = map[string]map[string]bool{
	"mkdir":    {"-m": true, "--mode": true},
	"install":  {"-m": true, "--mode": true, "-o": true, "--owner": true, "-g": true, "--group": true, "-S": true, "--suffix": true},
	"shred":    {"-n": true, "--iterations": true, "-s": true, "--size": true},
	"truncate": {"-s": true, "--size": true, "-r": true, "--reference": true},
	"cp":       {"-S": true, "--suffix": true},
	"mv":       {"-S": true, "--suffix": true},
	"ln":       {"-S": true, "--suffix": true},
	"chmod":    {"--reference": true},
	"chown":    {"--reference": true},
	"chgrp":    {"--reference": true},
	"touch":    {"-d": true, "--date": true, "-t": true, "-r": true, "--reference": true},
	"rsync": {
		"--exclude": true, "--include": true, "--exclude-from": true,
		"--include-from": true, "--files-from": true, "--filter": true, "-f": true,
		"--rsh": true, "-e": true, "--chmod": true, "--chown": true,
		"--usermap": true, "--groupmap": true, "--suffix": true,
		"--timeout": true, "--contimeout": true, "--bwlimit": true,
		"--max-size": true, "--min-size": true, "--block-size": true, "-B": true,
		"--modify-window": true, "--port": true, "--sockopts": true,
		"--out-format": true, "--log-file-format": true, "--info": true,
		"--debug": true, "--outbuf": true, "--protocol": true, "--iconv": true,
		"--checksum-seed": true, "--compress-level": true,
		"--skip-compress": true, "--password-file": true,
		"--remote-option": true, "-M": true,
		// --rsync-path=PROGRAM and the daemon-side options all take a value
		// in the space-separated form too; an unconsumed value becomes the
		// apparent final operand and displaces the real destination.
		"--rsync-path": true, "--address": true, "--config": true,
		"--dparam": true, "--secrets-file": true,
		"--max-delete": true, "--max-alloc": true, "--stop-after": true,
		"--stop-at": true, "--time-limit": true, "--write-devices": true,
		"--early-input": true, "--copy-as": true, "--iconv-from": true,
		"--preallocate-size": true, "--append-verify-size": true,
		// Basis directories are read from, never written to, so they are
		// consumed as ordinary values rather than classified as destinations.
		"--compare-dest": true, "--copy-dest": true, "--link-dest": true,
	},
	"mktemp": {"--suffix": true},
}

// destValueOpts lists, per command, the options whose value is itself a write
// destination rather than an inert scalar. Consuming them like an ordinary
// option value would silently drop a real target, so they are consumed *and*
// classified (see optionTargets). These *supplement* the command's positional
// destination rather than replacing it — unlike `-t`, an rsync run with
// `--backup-dir` still writes to its ordinary DEST as well.
var destValueOpts = map[string]map[string]bool{
	"rsync": {
		"--backup-dir": true, "--temp-dir": true, "-T": true,
		"--partial-dir": true, "--log-file": true, "--write-batch": true,
	},
}

// tmpDirOpts lists options that relocate a command's output directory, so the
// command's bare template is created there rather than in the cwd. Unlike
// destValueOpts these *replace* the positional target: `mktemp -p /tmp t.XXX`
// writes under /tmp even when the shell sits in a protected checkout.
var tmpDirOpts = map[string]map[string]bool{
	"mktemp": {"-p": true, "--tmpdir": true},
}

// optionalValueOpts lists options whose value is optional and therefore only
// ever supplied glued with `=`. mktemp's help spells it `--tmpdir[=DIR]`, and
// `mktemp --tmpdir scratch.XXXXXX` creates the template under $TMPDIR — the
// template is not the option's value. Consuming the next token here would both
// lose the template and classify it against the cwd, blocking a safe command
// run from a protected checkout.
var optionalValueOpts = map[string]map[string]bool{
	"mktemp": {"--tmpdir": true},
}

// defaultTempDir is the directory a valueless --tmpdir selects.
func defaultTempDir() string {
	if dir := os.Getenv("TMPDIR"); dir != "" {
		return dir
	}
	return os.TempDir()
}

// shortClusterValue classifies a single-dash short-option cluster. matched
// reports that tok is such a cluster for cmd; skipValue reports that the
// following token is a value rather than an operand.
//
// A value-taking letter swallows the rest of its own word, so `-Sbak` carries
// its value glued and consumes nothing further, while `-aS` ends on the
// value-taking letter and takes the next token.
func shortClusterValue(cmd, tok string) (skipValue, matched bool) {
	if len(tok) < 2 || tok[0] != '-' || tok[1] == '-' {
		return false, false
	}
	letters := shortOptsWithValue[cmd]
	if letters == "" {
		return false, false
	}
	runes := []rune(tok[1:])
	for i, c := range runes {
		if !strings.ContainsRune(letters, c) {
			continue
		}
		// Last letter in the word: the value is the next token. Otherwise the
		// remainder of this word is the value.
		return i == len(runes)-1, true
	}
	return false, false
}

// optionValue reports whether tok names one of opts, either exactly (its value
// is the following token) or in the `--opt=value` form (returned as glued).
func optionValue(opts map[string]bool, tok string) (glued string, ok bool) {
	if opts[tok] {
		return "", true
	}
	if i := strings.IndexByte(tok, '='); i > 0 && opts[tok[:i]] {
		return tok[i+1:], true
	}
	return "", false
}

// leadingSpecOperand lists commands whose first non-option positional is a
// non-path spec (a permission mode, owner, or group) rather than a write
// target, e.g. the 755 in `chmod 755 file` or the user in `chown user file`.
var leadingSpecOperand = map[string]bool{
	"chmod": true, "chown": true, "chgrp": true,
}

// targetDirCmds lists the commands whose GNU `-t`/`--target-directory` option
// names the destination directory. Its value is a mutation target rather than
// an inert scalar, so it must be consumed as an option value *and* classified
// (see optionTargets); merely skipping it would drop the destination and let
// `cp -t ~/src/dear-agent f` through with only its read-only source inspected.
var targetDirCmds = map[string]bool{
	"cp": true, "mv": true, "ln": true, "install": true,
}

// shortOptsWithValue lists, per command, the short-option letters that consume
// a value. Within a cluster the first such letter swallows the rest of the word
// (GNU accepts `-Stext` for `-S text`), so scanning must stop there: without
// this, the `t` in `cp -Stext` reads as `--target-directory` and the suffix's
// own text is mistaken for the destination.
var shortOptsWithValue = map[string]string{
	"cp": "St", "mv": "St", "ln": "St", "install": "Stmog",
	"mkdir": "m", "shred": "ns", "truncate": "sr", "touch": "dtr",
	"rsync": "feBMT",
}

// targetDirOption reports whether tok is cmd's -t/--target-directory option.
// inline holds a destination glued to the option (`--target-directory=DIR`,
// `-tDIR`, `-rtDIR`); an empty inline means the destination is the next token.
func targetDirOption(cmd, tok string) (inline string, ok bool) {
	if !targetDirCmds[cmd] {
		return "", false
	}
	switch {
	case tok == "-t" || tok == "--target-directory":
		return "", true
	case strings.HasPrefix(tok, "--target-directory="):
		return strings.TrimPrefix(tok, "--target-directory="), true
	case strings.HasPrefix(tok, "--") || !strings.HasPrefix(tok, "-") || tok == "-":
		return "", false
	}
	// Short-option cluster. `-t` takes a required directory, so the first `t`
	// ends the cluster: whatever follows it in the same word is the glued
	// destination (`-tDIR`, `-atDIR`), and an empty remainder means the
	// destination is the next token (`-t`, `-rt DIR`).
	// Short-option cluster: walk the letters in order and stop at the first one
	// that takes a value, because it consumes the remainder of the word. Only a
	// `t` reached that way is the target directory; anything glued after it is
	// the destination (`-atDIR`), and an empty remainder means the next token is
	// (`-t`, `-rt DIR`).
	body := tok[1:]
	for i, c := range body {
		if !strings.ContainsRune(shortOptsWithValue[cmd], c) {
			continue
		}
		if c != 't' {
			return "", false // e.g. -S swallows the rest as its suffix
		}
		return body[i+1:], true
	}
	return "", false
}

// isSymbolicMode reports whether tok is a chmod symbolic mode that begins with
// an operator (`-w`, `+x`, `=rw`, `u-w,g+r`). Such a mode is option-shaped, so
// without this test isOption would discard it and the following real target
// would be dropped as the "leading spec" instead — letting `chmod -w
// ~/src/<repo>/AGENTS.md` past the guard. chmod's own flags (`-R`, `-v`, `-c`,
// `-f`) use letters outside the mode alphabet, so they are unaffected.
func isSymbolicMode(tok string) bool {
	if len(tok) < 2 || !strings.ContainsRune("-+=", rune(tok[0])) {
		return false
	}
	return strings.IndexFunc(tok[1:], func(r rune) bool {
		return !strings.ContainsRune("rwxXstugoa,+-=", r)
	}) < 0
}

// hasReferenceOption reports whether a chmod/chown/chgrp invocation takes its
// mode/owner/group from `--reference=RFILE` instead of a leading spec operand.
// In that form (`chmod --reference=R FILE...`) the first positional is already
// a mutation target, so dropping it would silently lose the real target.
func hasReferenceOption(rest []string) bool {
	for _, a := range rest {
		if a == "--reference" || strings.HasPrefix(a, "--reference=") {
			return true
		}
	}
	return false
}

// targetOperands returns the operands of a write command that name filesystem
// targets, preserving order. Unlike pathyArgs it keeps bare relative names
// (which the caller resolves against cwd), while skipping options, the values
// of value-taking options, and any leading non-path spec operand (a chmod mode
// or chown owner). This is what lets `rm AGENTS.md` be classified against cwd
// without misreading `chmod 755 f`'s 755 or `mkdir -m 755 d`'s 755 as targets.
func targetOperands(cmd string, rest []string) []string {
	out := make([]string, 0, len(rest))
	leadingSkipped := !leadingSpecOperand[cmd] || hasReferenceOption(rest)
	operandsOnly := false
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !operandsOnly {
			// `--` ends option parsing: every later token is an operand even
			// when it starts with '-' (`rm -- -logfile`), and `--` itself names
			// nothing, so it must not be classified as a relative path.
			if a == "--" {
				operandsOnly = true
				continue
			}
			// A chmod symbolic mode is option-shaped (`chmod -w f`); it is the
			// leading spec, not a flag, so it must be recognized before the
			// generic option filter discards it.
			if !leadingSkipped && cmd == "chmod" && isSymbolicMode(a) {
				leadingSkipped = true
				continue
			}
			if skipValue, handled := consumeOption(cmd, a); handled {
				if skipValue {
					i++ // the value is not an operand
				}
				continue
			}
		}
		if !leadingSkipped {
			leadingSkipped = true
			continue // drop the leading mode/owner/group spec
		}
		out = append(out, a)
	}
	return out
}

// consumeOption classifies an option-shaped token for targetOperands: handled
// reports that the token is an option rather than an operand, and skipValue
// that the following token is its value and must be skipped too. Options whose
// value is a path (`-t DIR`, rsync's `--backup-dir DIR`, mktemp's `-p DIR`) are
// skipped here and classified separately by the optionTargets helpers.
func consumeOption(cmd, tok string) (skipValue, handled bool) {
	if inline, ok := targetDirOption(cmd, tok); ok {
		return inline == "", true
	}
	if glued, ok := optionValue(destValueOpts[cmd], tok); ok {
		return glued == "", true
	}
	if glued, ok := optionValue(tmpDirOpts[cmd], tok); ok {
		// An optional-value option in its bare form takes nothing: its value
		// only ever arrives glued with `=`.
		if glued == "" && optionalValueOpts[cmd][tok] {
			return false, true
		}
		return glued == "", true
	}
	if isOption(tok) {
		if optsWithValue[cmd][tok] {
			return true, true
		}
		if skip, ok := shortClusterValue(cmd, tok); ok {
			return skip, true
		}
		return false, true
	}
	return false, false
}

// replacingOptionTargets returns targets named by an option that *stands in for*
// the command's positional destination: GNU's `-t`/`--target-directory` and
// mktemp's `-p`/`--tmpdir`. When one is present the positional operands are
// sources or templates, not destinations.
func replacingOptionTargets(cmd string, rest []string) []string {
	return scanOptionTargets(rest, func(a string) (string, bool) {
		if inline, ok := targetDirOption(cmd, a); ok {
			return inline, true
		}
		if optionalValueOpts[cmd][a] {
			// Bare --tmpdir: the destination is $TMPDIR, and it still
			// replaces the positional template as the write target.
			return defaultTempDir(), true
		}
		return optionValue(tmpDirOpts[cmd], a)
	})
}

// auxOptionTargets returns targets named by an option that *supplements* the
// command's positional destination — rsync still writes to its ordinary DEST
// alongside `--backup-dir`, so both must be classified.
func auxOptionTargets(cmd string, rest []string) []string {
	return scanOptionTargets(rest, func(a string) (string, bool) {
		return optionValue(destValueOpts[cmd], a)
	})
}

// scanOptionTargets collects the values of the options match accepts, taking
// either the value glued to the option or the following token.
func scanOptionTargets(rest []string, match func(string) (string, bool)) []string {
	var out []string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			break // options end here; the rest are operands
		}
		inline, ok := match(a)
		if !ok {
			continue
		}
		if inline != "" {
			out = append(out, inline)
			continue
		}
		if i+1 < len(rest) {
			out = append(out, rest[i+1])
			i++
		}
	}
	return out
}

// stripRedirections removes redirection syntax from a simple-command segment:
// the operator (with any file descriptor the tokenizer kept glued to it) and
// the token it redirects to. Redirect targets are classified separately by
// checkRedirections; left in the segment they would masquerade as command
// operands, so `cp a ~/src/dear-agent/f 2>&1` would pick `1` as cp's
// destination and miss the protected write entirely. A whitespace-separated
// digit is a real operand (`rm 2 > log` removes the file `2`) and survives,
// because the tokenizer only glues a descriptor that was lexically adjacent.
func stripRedirections(segment []string) []string {
	out := make([]string, 0, len(segment))
	for i := 0; i < len(segment); i++ {
		tok := segment[i]
		if !isRedirOp(tok) && !isInputRedirOp(tok) {
			out = append(out, tok)
			continue
		}
		i++ // drop the redirect target as well
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
	for len(args) > 0 && commandRunners[filepath.Base(args[0])] {
		runner := filepath.Base(args[0])
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

// writeTargets returns the filesystem paths a write-command would mutate,
// including bare relative names (resolved against cwd by the caller). Option
// values and leading spec operands are excluded (see targetOperands); dd, sed,
// and perl keep their bespoke path-shaped extraction.
func writeTargets(cmd string, rest []string) []string {
	switch {
	case destAll[cmd] || cmd == "tee" || permsCmds[cmd]:
		relocated := replacingOptionTargets(cmd, rest)
		out := make([]string, 0, len(relocated)+len(rest))
		out = append(out, relocated...)
		out = append(out, auxOptionTargets(cmd, rest)...)
		if cmd == "mktemp" && len(relocated) > 0 {
			return out // -p/--tmpdir relocates the template out of the cwd
		}
		return append(out, targetOperands(cmd, rest)...)
	case destLast[cmd]:
		// cp/rsync/install/ln SRC... DEST -> the destination is last. A
		// -t/--target-directory replaces that positional destination, while
		// rsync's auxiliary output directories are written in addition to it.
		out := auxOptionTargets(cmd, rest)
		if dirs := replacingOptionTargets(cmd, rest); len(dirs) > 0 {
			return append(out, dirs...)
		}
		if ops := targetOperands(cmd, rest); len(ops) > 0 {
			out = append(out, ops[len(ops)-1])
		}
		return out
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
		// Two hardening rounds meet here. Detection is the complete
		// safe-push parser, so short-option clusters (`-uf`), abbreviations,
		// `--mirror`, and leading-plus force refspecs all count. The decision
		// is target-aware, so a force-push to a PR branch inside ~/src is
		// allowed while one that can reach the default branch is not, and an
		// unresolvable destination is refused rather than assumed safe. Both
		// live in safegit.ForcePushViolation, so this guard, safe-push, and
		// the PreToolUse hooks share one answer.
		if reason, blocked := safegit.ForcePushViolation(repoDir, "", subArgs); blocked {
			return false, "You're trying to force-push in ~/src where it can reach a " +
				"protected/default branch (" + reason + "), which would clobber " +
				"the golden reference. " +
				"Force-push, --mirror, and +refspec are all evaluated against the " +
				"resolved destination. Push a non-default PR branch instead " +
				"(force-with-lease is fine), use a plain `git -C ~/src/" + repo +
				" push`, or do the work in a worktree."
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
	return g.checkSegments(tokens, cwd, depth)
}

// checkRedirections classifies the target of every redirection operator,
// skipping pure file-descriptor duplications such as `2>&1` or `>&2`. It runs
// per segment, against the directory `cd` tracking has reached at that point,
// so `cd ~/src/dear-agent && echo x > README.md` resolves the bare target
// inside the protected checkout rather than against the original cwd.
func (g *Guard) checkRedirections(tokens []string, cwd string) (allowed bool, message string) {
	return g.checkRedirectionsAt(tokens, func(target string) (bool, string) {
		return g.Classify(target, cwd)
	})
}

// checkRedirectionsAt is checkRedirections with the directory resolution
// supplied by the caller, so a segment whose cwd is uncertain can be checked
// against every candidate directory.
func (g *Guard) checkRedirectionsAt(tokens []string, classify func(string) (bool, string)) (allowed bool, message string) {
	for idx, tok := range tokens {
		if !isRedirOp(tok) || idx+1 >= len(tokens) {
			continue
		}
		target := tokens[idx+1]
		// Skip fd-dups (`2>&1`, `>&2`); everything else is a filename target,
		// including a bare relative name like `> README.md`, which Classify
		// resolves against cwd. An all-digit target counts as a descriptor only
		// for a dup operator — plain `> 2` creates the relative file `2`.
		if separators[target] || strings.HasPrefix(target, "&") ||
			(isAllDigits(target) && strings.ContainsRune(tok, '&')) {
			continue
		}
		if ok, msg := classify(target); !ok {
			return false, msg
		}
	}
	return true, ""
}

// checkSegments runs per-simple-command write analysis, tracking `cd` so a
// chained `cd ~/src/repo && git commit` is attributed to the right directory.
func (g *Guard) checkSegments(tokens []string, cwd string, depth int) (allowed bool, message string) {
	currentDir := g.expand(cwd, cwd)
	// alsoCheck holds directories a relative target might *also* resolve
	// against, because a `cd` that would have moved the shell there may not
	// have run. `false && cd /tmp; rm AGENTS.md` leaves the shell in the
	// original directory, and tracking only the post-`cd` path would classify
	// the removal against /tmp and miss the protected file. A `cd` in a
	// pipeline has the same mismatch: it runs in a subshell.
	var alsoCheck []string
	var subshellDirs []string // saved cwd per open `(`
	classify := func(target string) (bool, string) {
		if ok, msg := g.Classify(target, currentDir); !ok {
			return false, msg
		}
		for _, dir := range alsoCheck {
			if ok, msg := g.Classify(target, dir); !ok {
				return false, msg
			}
		}
		return true, ""
	}
	for _, seg := range splitSegments(tokens) {
		// A `cd` inside `( … )` is undone when the subshell exits, so restore
		// the caller's directory on `)`; otherwise `(cd /tmp); rm AGENTS.md`
		// would classify the removal against /tmp while it really runs in cwd.
		if len(seg.tokens) == 1 && (seg.tokens[0] == "(" || seg.tokens[0] == ")") {
			if seg.tokens[0] == "(" {
				subshellDirs = append(subshellDirs, currentDir)
			} else if n := len(subshellDirs); n > 0 {
				currentDir = subshellDirs[n-1]
				subshellDirs = subshellDirs[:n-1]
				alsoCheck = nil
			}
			continue
		}
		segment := seg.tokens
		if ok, msg := g.checkRedirectionsAt(segment, classify); !ok {
			return false, msg
		}
		args := stripRunners(realArgs(stripRedirections(segment)))
		if len(args) == 0 {
			continue
		}
		// Classify by the command's basename so an absolute or PATH-qualified
		// executable (`/bin/rm`, `/usr/bin/git`) is recognized the same as its
		// bare name; otherwise `/bin/rm ~/src/f` would slip past every map lookup.
		cmd := filepath.Base(args[0])

		// `cd` is matched on the literal command word, not the basename: only
		// the shell builtin changes the shell's directory. An external program
		// that merely happens to be named `cd` (`/tmp/cd /tmp`) cannot change
		// its parent's cwd, so tracking it would move the guard's idea of the
		// directory away from where the following commands actually run.
		if args[0] == "cd" && len(args) > 1 {
			next := g.expand(args[1], currentDir)
			if seg.conditional() {
				// The shell may still be in the previous directory, so keep
				// it as a candidate rather than replacing it.
				alsoCheck = append(alsoCheck, currentDir)
			} else {
				// An unconditional `cd` definitely ran; earlier uncertainty
				// is resolved and the candidate set collapses.
				alsoCheck = nil
			}
			currentDir = next
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
			if ok, msg := classify(target); !ok {
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
