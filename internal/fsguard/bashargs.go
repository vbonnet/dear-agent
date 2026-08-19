package fsguard

// Command-argument classification: which tokens of a simple command name a
// path the guard must check.
//
// Split out of bash.go, which had grown past the structural-health size
// ratchet. This half answers "given `cp -aS bak SRC DEST`, what does it
// write?"; bash.go keeps the shell-level concerns (tokenizing, segmenting,
// tracking `cd`, and applying the policy).

import (
	"os"
	"strings"
)

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
