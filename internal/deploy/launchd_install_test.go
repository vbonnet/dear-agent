package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// programArgumentsBinary matches a ~/go/bin path inside a launchd template's
// ProgramArguments, e.g. <string>__HOME__/go/bin/token-refresher</string>.
var programArgumentsBinary = regexp.MustCompile(`(?:__HOME__|\$\{?HOME\}?|/Users/[^/<]+)/go/bin/([A-Za-z0-9._-]+)`)

// TestLaunchdBinariesUseHardenedInstall asserts that every binary launchd runs
// is installed into ~/go/bin through the install-go-bin macro rather than a
// bare `cp` (ce-77ip.8).
//
// Why this matters, and why a comment in the Makefile is not enough:
//
// `cp` rewrites the destination's EXISTING inode in place. If that binary has
// already been executed, macOS may still hold the code-signing identity cached
// for that vnode, so the newly written bytes fail validation and the kernel
// SIGKILLs the process with OS_REASON_CODESIGNING *before main() runs*. There
// is no stderr, no exit message, and no log line — for a launchd job, which has
// no terminal, the only symptom is that it silently stops working.
//
// The kill is INTERMITTENT — it depends on the cached signature still being
// live for that vnode. Measured here: bare cp killed 1/30, staged-and-renamed
// 0/30. That intermittency is the point. A rebuild usually works, so when it
// does not the failure reads as anything but the install step.
//
// On 2026-07-19 this disabled the OAuth token-refresher for 17 hours. Because
// the symptom (credentials go stale, sessions 401) is identical to a dead token
// family, it was misdiagnosed and cost a day plus a manual re-auth.
//
// The install-go-bin macro stages to a temp path, ad-hoc signs it, and renames
// it into place — a new inode, so no stale cdhash can be cached against it, and
// an atomic swap so a launchd tick firing mid-install cannot observe a
// half-written file.
//
// This test exists so the next person who copies a neighbouring install target
// cannot silently re-arm the failure.
func TestLaunchdBinariesUseHardenedInstall(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	binaries := launchdManagedBinaries(t, filepath.Join(repoRoot, "deploy", "launchd"))
	if len(binaries) == 0 {
		t.Fatal("no launchd-managed ~/go/bin binaries found; the plist scan is broken")
	}

	raw, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(raw)

	var macroDefined bool
	for _, rel := range trackedMakefiles(t, repoRoot) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err == nil && strings.Contains(string(raw), "define install-go-bin") {
			macroDefined = true
			break
		}
	}
	if !macroDefined {
		t.Fatal("no tracked build file defines install-go-bin: the hardened install path is gone")
	}

	for _, bin := range binaries {
		t.Run(bin, func(t *testing.T) {
			// It must positively use the macro, so deleting the cp line
			// without replacing it does not pass.
			want := "$(call install-go-bin,bin/" + bin + ")"
			if !strings.Contains(makefile, want) {
				t.Errorf("%s is launchd-managed but its install target does not use the hardened macro.\n"+
					"Expected the Makefile to contain: %s", bin, want)
			}
		})
	}

	// No install target anywhere may use a bare cp into an install root. The
	// code-signing kill is not specific to launchd binaries — review pointed
	// out that rebuilding an already-executed safe-push or safe-pr reproduces
	// it identically, and those fail in the middle of a developer's workflow.
	// The launchd set is merely where the failure is *silent*.
	//
	// Matching on the INSTALL side rather than resolving plists is deliberate.
	// Several launch-agent templates reference their program through a
	// placeholder (__AGM_BINARY__, __BUMBLEBEE_BINARY__), so a path-derived
	// guard cannot see them at all — which is how the bumblebee wrapper,
	// installed with `install -m` into ~/.local/bin, escaped the first version
	// of this check. Asserting that no install target uses a raw copy into any
	// install root covers every scheduled binary regardless of how its plist
	// names it.
	// Scanned across EVERY tracked Makefile, not just the root one. agm/Makefile
	// had its own `install -m 755` path for agm, agm-reaper and agm-mcp-server
	// -- agm being launchd-scheduled -- and a root-only scan never saw it. Its
	// comment even claimed `install` gave "atomic replacement", which is the
	// misconception this whole change exists to correct.
	t.Run("no-bare-installs", func(t *testing.T) {
		// Every spelling of an install root, not just $(HOME)/go/bin: a Makefile
		// may write ~/go/bin, $HOME/go/bin, ${HOME}/go/bin or $(HOME)/.local/bin,
		// and recognising only one of them leaves the retired path reachable
		// through a synonym.
		bareCopy := regexp.MustCompile(`(?m)^\t@?(?:cp|install)\b[^\n]*` + installRootPattern + `[^\n]*$`)
		for _, rel := range trackedMakefiles(t, repoRoot) {
			raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
			if err != nil {
				continue
			}
			for _, m := range bareCopy.FindAllString(stripRedirections(joinContinuations(string(raw))), -1) {
				// Scripts are immune to the stale-signature kill; see
				// interpretedSource.
				if allSourcesInterpreted(repoRoot, m) {
					continue
				}
				t.Errorf("%s copies into an install root without the hardened macro:\n\t%s\n"+
					"Use $(call install-go-bin,bin/<name>[,<dest-dir>]) from mk/install-go-bin.mk. "+
					"A raw cp/install rewrites the existing inode; macOS then kills the rebuilt "+
					"binary with OS_REASON_CODESIGNING before main() runs — silently, for a "+
					"launchd job.", rel, strings.TrimSpace(m))
			}
		}
	})
}

// launchdManagedBinaries returns the set of ~/go/bin binary names referenced by
// ProgramArguments across the launchd templates, so the guard tracks whatever
// is actually scheduled rather than a hand-maintained list that drifts.
func launchdManagedBinaries(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	seen := map[string]bool{}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".plist") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, m := range programArgumentsBinary.FindAllStringSubmatch(string(raw), -1) {
			name := m[1]
			// Wrapper scripts (e.g. *.sh) are not Go binaries installed by the
			// Makefile; only guard things we actually build and install.
			if strings.Contains(name, ".") || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// apiKeyHelperAllowlist names the file recording every sanctioned mention of
// apiKeyHelper in the repository, keyed by a hash of the surrounding clause.
const apiKeyHelperAllowlist = "testdata/apikeyhelper-allowlist.txt"

// setCommand is the operator command that must never reappear anywhere.
var setCommand = regexp.MustCompile(`configure-claude-settings\s+set\s+apiKeyHelper`)

// configKey matches apiKeyHelper used as a settings key in JSON/YAML/TOML —
// the most direct route to the failure, needing no prose and no Makefile.
var configKey = regexp.MustCompile(`["']?apiKeyHelper["']?\s*[:=]`)

// installRootPattern matches any directory binaries get installed into, in any
// spelling. Enumerating roots is how this guard kept leaking: it began with
// $(HOME)/go/bin, then needed ~/.local/bin, then $(HOOKS_DIR), then $(GOPATH)/bin
// and /usr/local/bin. The list below is still a list, but it now covers the
// system roots and the GOPATH form as well as the home-relative ones, and both
// the Makefile and the documentation scanners share it so they cannot drift
// apart.
//
// Hook directories belong here for the same reason binaries do: a compiled hook
// overwritten while it is executing hits the identical stale-signature kill,
// and hooks are installed by shell scripts using $HOOKS_DIR, ~/.claude/hooks
// and .git/hooks rather than the Make-style $(HOOKS_DIR) this once matched.
const installRootPattern = `(?:(?:\$\(HOME\)|\$\{HOME\}|\$HOME|~)/(?:go/bin|\.local/bin|bin|\.claude/hooks|\.config/claude-code/hooks)` +
	`|\$\((?:HOOKS_DIR|GOPATH)\)(?:/bin)?` +
	`|(?:\$GOPATH|\$\{GOPATH\})/bin` +
	`|(?:\$HOOKS_DIR|\$\{HOOKS_DIR\})` +
	`|\.git/hooks` +
	`|/usr/local/bin|/opt/homebrew/bin)`

// clauseSplit ends a clause at ";" or at sentence punctuation followed by
// whitespace. Requiring the trailing whitespace keeps "claude-code 2.1.205"
// and "settings.json" from splitting mid-token.
// commandSplit separates the commands within one shell line.
var commandSplit = regexp.MustCompile(`&&|\|\||;|\|`)

// stripRedirections removes shell redirections from every line of text.
func stripRedirections(text string) string {
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimRight(redirection.ReplaceAllString(line, ""), " \t"))
	}
	return b.String()
}

// redirection matches a shell redirection operand: >file, >>file, 2>file,
// &>file, <file, spaced or not, with the target either bare or QUOTED.
//
// The quoted alternatives matter: consuming only the first whitespace-delimited
// fragment of `> "$LOG_DIR/install log"` left a dangling `log"` behind, which
// displaced the destination from the end of the command and let the overwrite
// through.
var redirection = regexp.MustCompile(`\s*[0-9]*(?:&?>{1,2}|<)\s*(?:"[^"]*"|'[^']*'|\S+)`)

// splitCommands breaks a shell line into its individual commands, so a check
// applied to one cannot be satisfied or excused by another, and strips trailing
// redirections from each.
//
// Redirections are removed rather than added to the terminator set. A
// destination can be followed by any of `>/dev/null`, `2>/dev/null`, `&>log`,
// `>> log` — enumerating those in the match itself is the same losing game as
// enumerating install roots or comment characters. Removing them first leaves
// each command ending where its arguments end.
func splitCommands(line string) []string {
	parts := commandSplit.Split(line, -1)
	for i, p := range parts {
		parts[i] = strings.TrimSpace(redirection.ReplaceAllString(p, ""))
	}
	return parts
}

var clauseSplit = regexp.MustCompile(`;|[.!?]\s`)

// TestNoAPIKeyHelperInstructions asserts that nothing in the repository can
// lead an operator to wire token-refresher in as Claude Code's apiKeyHelper.
//
// Since claude-code 2.1.205 a configured apiKeyHelper is treated as an external
// API key that shadows a healthy claude.ai OAuth login and refuses to fall back
// to it, so `claude -p` fails with "Invalid API key" even when
// ~/.claude/.credentials.json is fresh (anthropics/claude-code#11587, #9694,
// #23568). That wiring caused a multi-day mesh outage and was removed from the
// host on 2026-07-10.
//
// The check has two tiers, because they carry different weight:
//
//   - HARD, and not gameable: the `configure-claude-settings set apiKeyHelper`
//     command, and apiKeyHelper used as a config KEY. These are the forms that
//     actually re-enable the shadowing.
//
//   - STRUCTURED: every remaining mention must be listed in an allowlist keyed
//     by a hash of its clause. Earlier revisions tried to judge prose polarity
//     and were defeated four times over — by a new phrasing, by a disclaimer in
//     a neighbouring clause, by a first-occurrence shortcut, and finally by
//     "apiKeyHelper is retired, but use token-refresher as apiKeyHelper", where
//     one clause holds both a disclaimer and a recommendation. Deciding that
//     last case needs parsing, not pattern matching. So the guard stops
//     guessing: a new or reworded mention fails until a human adds it to the
//     allowlist, which is the deliberate review step the invariant deserves.
func TestNoAPIKeyHelperInstructions(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	allowed, err := loadAllowlist(filepath.Join("testdata", "apikeyhelper-allowlist.txt"))
	if err != nil {
		t.Fatalf("read allowlist: %v", err)
	}
	seen := map[string]bool{}

	for _, rel := range trackedTextFiles(t, repoRoot) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			continue // deleted between listing and reading
		}
		body := string(raw)
		if !strings.Contains(strings.ToLower(body), "apikeyhelper") {
			continue
		}

		if loc := setCommand.FindString(body); loc != "" {
			t.Errorf("%s instructs wiring apiKeyHelper (%q).\n"+
				"It shadows healthy OAuth on claude-code >= 2.1.205 and was retired on "+
				"2026-07-10 (anthropics/claude-code#11587). The launchd idle backstop is "+
				"the only sanctioned wiring. Cleanup (`... remove apiKeyHelper`) is fine.",
				rel, loc)
		}
		if isConfig(rel) {
			if loc := configKey.FindString(body); loc != "" {
				t.Errorf("%s sets apiKeyHelper as a configuration key (%q).\n"+
					"This re-enables OAuth shadowing directly, with no prose and no "+
					"Makefile change. It must not appear in tracked configuration.",
					rel, loc)
			}
		}

		occurrence := map[string]int{}
		for _, m := range clauseMentions(body) {
			norm := normalizeText(m.clause)
			occurrence[norm]++
			key := siteKey(rel, m.clause, occurrence[norm])
			seen[key] = true
			if allowed[key] {
				continue
			}
			t.Errorf("%s:%d has an unrecognised apiKeyHelper mention:\n\t%s\n\n"+
				"Every mention is allowlisted by clause hash so that adding or rewording "+
				"one is a deliberate, reviewed act. If this mention makes clear the wiring "+
				"is RETIRED, add this line to internal/deploy/%s:\n\n\t%s  # %s\n\n"+
				"If it recommends the wiring, delete it instead: claude-code >= 2.1.205 "+
				"treats a configured helper as an external API key that shadows healthy "+
				"OAuth (anthropics/claude-code#11587).",
				rel, m.line, strings.TrimSpace(m.clause), apiKeyHelperAllowlist, key, rel)
		}
	}

	for key := range allowed {
		if !seen[key] {
			t.Errorf("allowlist entry %s in internal/deploy/%s matches no mention any more.\n"+
				"Remove it so the file keeps describing the repository as it is.",
				key, apiKeyHelperAllowlist)
		}
	}
}

// mention is one clause containing apiKeyHelper, with the line it starts on.
type mention struct {
	clause string
	line   int
}

// clauseMentions returns every clause of body containing apiKeyHelper.
func clauseMentions(body string) []mention {
	var out []mention
	add := func(clause string, start int) {
		if !strings.Contains(strings.ToLower(clause), "apikeyhelper") {
			return
		}
		out = append(out, mention{clause: clause, line: 1 + strings.Count(body[:start], "\n")})
	}
	start := 0
	for _, sep := range clauseSplit.FindAllStringIndex(body, -1) {
		add(body[start:sep[0]], start)
		start = sep[1]
	}
	if start < len(body) {
		add(body[start:], start)
	}
	return out
}

// scriptExtension matches a copy source that is immune to the stale-signature
// kill because it is interpreted.
//
// A glob is NOT immune and is deliberately absent: `cp bin/* /usr/local/bin/`
// expands to whatever is in that directory, Mach-O binaries included, so
// exempting a command merely for containing `*` waved through exactly the
// overwrite this guard exists to catch.
var scriptExtension = regexp.MustCompile(`\.(?:sh|py|rb|pl|js|ts|bash|zsh)$`)

// flagTakesArg lists install(1) flags whose following token is a value, not a
// source path.
var flagTakesArg = map[string]bool{"-m": true, "-o": true, "-g": true, "--mode": true, "--owner": true, "--group": true}

// allSourcesInterpreted reports whether EVERY source operand of a copy is an
// interpreted script.
//
// Checking "any source is interpreted" was bypassable: `cp hook.sh agm
// /usr/local/bin/` exempted the whole command on hook.sh while agm was
// overwritten in place. An exemption is only sound if it covers every operand
// it excuses.
func allSourcesInterpreted(repoRoot, cmd string) bool {
	if i := strings.Index(cmd, "#"); i >= 0 {
		cmd = cmd[:i]
	}
	fields := strings.Fields(cmd)
	for len(fields) > 0 && fields[0] == "sudo" {
		fields = fields[1:]
	}
	if len(fields) < 3 {
		return false // verb + at least one source + destination
	}
	fields = fields[1:]             // drop the verb
	fields = fields[:len(fields)-1] // drop the destination

	var sources []string
	for i := 0; i < len(fields); i++ {
		f := strings.Trim(fields[i], `"'`)
		if strings.HasPrefix(f, "-") {
			if flagTakesArg[f] {
				i++
			}
			continue
		}
		sources = append(sources, f)
	}
	if len(sources) == 0 {
		return false
	}
	for _, src := range sources {
		if !isInterpretedSource(repoRoot, src) {
			return false
		}
	}
	return true
}

// isInterpretedSource reports whether a copy source is an interpreted script.
//
// Extension alone is not enough: this repository has many tracked, executable,
// EXTENSIONLESS shebang scripts (.claude/hooks/*, scripts/codegraph), and
// treating those as compiled binaries would make the guard block a safe
// operation — with no way out, since the allowlist only exempts warnings. So
// when the extension is inconclusive, read the tracked file and look for a
// shebang. A path that is not a tracked text file (a build output such as
// `agm`) stays classified as a binary, which is the safe default.
func isInterpretedSource(repoRoot, src string) bool {
	if scriptExtension.MatchString(src) {
		return true
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.Clean(src)))
	if err != nil {
		return false
	}
	return bytes.HasPrefix(raw, []byte("#!"))
}

// siteKey identifies one sanctioned occurrence: its file, its text, AND which
// occurrence of that text it is.
//
// File+text alone still let a warning duplicated verbatim later in the SAME
// file exempt an active copy, because both occurrences hash identically. The
// ordinal makes each site reviewable on its own.
//
// Keying on text alone would let a single allowlisted entry authorise the same
// command or clause ANYWHERE in the repository -- so a warning example could
// silently bless real install guidance elsewhere, and moving the command out of
// its warning would still satisfy the stale-entry check. Binding the exemption
// to a location means each site is reviewed on its own.
func siteKey(path, text string, ordinal int) string {
	return clauseKey(fmt.Sprintf("%s\x00%s\x00%d", path, text, ordinal))
}

// normalizeText collapses whitespace exactly as clauseKey does, so occurrence
// counters and hashes agree. Counting raw text while hashing normalised text
// gave two mentions differing only in reflow the same ordinal AND the same
// hash, so one reviewed warning could exempt an unreviewed duplicate.
func normalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// retiredExampleMarker is the explicit annotation an author must place beside a
// sanctioned example of the retired install form.
const retiredExampleMarker = "RETIRED-EXAMPLE"

// hasRetiredExampleMarker reports whether the marker appears on the exempted
// line or the two lines above it.
//
// The allowlist says "this text at this place is sanctioned"; the marker says
// "and it is still presented as a warning". Without it, deleting the
// surrounding "do NOT use it" prose -- or rewriting it as a recommendation --
// leaves the key unchanged and the exemption intact, so retired guidance could
// go live under an old approval.
//
// A marker rather than a hash of the surrounding prose: hashing context would
// churn the allowlist on any nearby edit, and would re-import the judge-the-prose
// problem this design exists to avoid. The marker is a deliberate author
// assertion, which is exactly what an exemption should require.
func hasRetiredExampleMarker(lg logicalLine, body string) bool {
	lines := strings.Split(body, "\n")
	lo := max(lg.line-3, 0)
	hi := min(lg.line, len(lines))
	return strings.Contains(strings.Join(lines[lo:hi], "\n"), retiredExampleMarker)
}

// clauseKey hashes a clause with whitespace collapsed, so reflowing prose does
// not churn the allowlist but changing the words does.
func clauseKey(clause string) string {
	norm := strings.Join(strings.Fields(clause), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])[:16]
}

// loadAllowlist reads the sanctioned clause hashes. Blank lines and comments
// are ignored; each entry is the hash, optionally followed by a comment.
func loadAllowlist(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[strings.Fields(line)[0]] = true
	}
	return out, nil
}

// isConfig reports whether a path is a configuration file, where apiKeyHelper
// appearing as a key would directly re-enable the shadowing.
func isConfig(rel string) bool {
	switch filepath.Ext(rel) {
	case ".json", ".yaml", ".yml", ".toml":
		return true
	}
	return false
}

// trackedTextFiles lists every git-tracked text file, so the guard covers
// extensionless executables too — the .claude/hooks/* scripts are tracked,
// operator-facing, and have no extension, so an extension allowlist silently
// skipped them.
func trackedTextFiles(t *testing.T, repoRoot string) []string {
	t.Helper()

	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = repoRoot
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}

	var out []string
	for rel := range strings.SplitSeq(string(raw), "\x00") {
		if rel == "" || strings.HasSuffix(rel, "_test.go") {
			continue // the guard's own fixtures state the forbidden strings
		}
		if strings.Contains(rel, "/testdata/") || strings.HasPrefix(rel, "testdata/") ||
			strings.Contains(rel, "node_modules/") || strings.Contains(rel, "vendor/") {
			continue
		}
		if isBinary(filepath.Join(repoRoot, rel)) {
			continue
		}
		out = append(out, rel)
	}
	if len(out) == 0 {
		t.Fatal("git ls-files returned nothing; the scan is broken")
	}
	return out
}

// isBinary reports whether a file looks binary (a NUL byte in its first 8KB).
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

// TestNoRawCopyIntoInstallRoots asserts that no tracked doc or script tells an
// operator to `cp` a binary straight over an install root.
//
// Scope note, established by measurement rather than assumption: `go build -o
// ~/go/bin/agm` is SAFE and is deliberately not flagged. The Go toolchain does
// not leave a stale code-signing cache entry, and 3/3 trials rebuilding over an
// already-executed binary ran fine. A raw `cp` over that same binary was killed
// 1/30. So the docs' many `go build -o` lines are fine; only raw copies are not.
func TestNoRawCopyIntoInstallRoots(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	// Documentation must be able to SHOW the retired form in order to warn
	// against it. Without an escape hatch this guard forbids explaining the very
	// failure it exists to prevent, so each sanctioned example is allowlisted by
	// a hash of the command — the same structured-exemption approach used for
	// apiKeyHelper mentions, and for the same reason: deciding from surrounding
	// prose whether a command is endorsed or prohibited is not something a
	// regex can do.
	allowed, err := loadAllowlist(filepath.Join("testdata", "rawcopy-allowlist.txt"))
	if err != nil {
		t.Fatalf("read allowlist: %v", err)
	}
	seenExempt := map[string]bool{}

	// Match on the copy's DESTINATION, not on tokens that happen to share the
	// line.
	//
	// Two earlier attempts exempted "safe-looking" neighbours — first any later
	// `mv`, then `mktemp` and `mv` matched independently — and both were
	// bypassable, because neither was tied to the copy. `mktemp /tmp/x.XXXXXX;
	// cp agm /usr/local/bin/agm; mv log log.old` satisfies both while
	// overwriting the binary in place.
	//
	// Anchoring on the destination removes the need for an exemption entirely:
	// the safe form copies into the staging path (`cp bin/foo "$stage"`), whose
	// destination is not an install root, so it never matches. Only a copy whose
	// final argument IS an install-root path is flagged. An mktemp allocating a
	// staging name under an install root is likewise not a copy, so it does not
	// match either.
	//
	// Intervening tokens are consumed non-greedily and must not cross a command
	// separator, so `install -m 755 agm <root>/agm` is caught (flags may take
	// arguments) while `cp agm "$stage" && mv ...` is not (the separator stops
	// the scan before any install root later on the line).
	//
	// The trailing group accepts only end-of-command, never another argument, so
	// the matched path must be the DESTINATION. That distinction matters:
	// `cp ~/go/bin/agm.backup "$stage"` reads FROM an install root and is
	// perfectly safe.
	//
	// The destination may be quoted — `cp foo "/usr/local/bin/foo"` and
	// `install -m 755 foo "$HOME/go/bin/foo"` are ordinary shell — so optional
	// quotes are allowed around it.
	//
	// Copies of INTERPRETED files and directory globs are skipped. Measured on
	// this host: overwriting an executed shell script 0/30 killed, versus 1/30
	// for a Mach-O binary. Code-signing validation applies to the interpreter,
	// not the script text, so demanding staged installs for .sh/.py hooks would
	// be noise — and a guard that cries wolf on safe operations gets suppressed
	// rather than obeyed.
	rawCopy := regexp.MustCompile(
		`\b(?:sudo\s+)?(?:cp|install)\s+(?:[^\s;|&]+\s+)*?(?:sudo\s+)?["']?` +
			installRootPattern + `[^\s"';|&#]*["']?\s*(?:$|[;|&#])`)

	for _, rel := range trackedTextFiles(t, repoRoot) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			continue
		}
		occurrence := map[string]int{}
		for _, lg := range logicalLines(string(raw)) {
			line, i := lg.text, lg.line-1
			// Evaluate each COMMAND separately. Checking the whole logical line
			// let an exemption earned by one command excuse another:
			// `cp hook.sh "$HOOKS_DIR/hook" && cp agm /usr/local/bin/agm` matched
			// rawCopy on the second and interpretedSource on the first, so the
			// binary overwrite was skipped. Splitting is what binds each
			// exemption to the command that earned it.
			var offending string
			for _, cmd := range splitCommands(line) {
				if rawCopy.MatchString(cmd) && !allSourcesInterpreted(repoRoot, cmd) {
					offending = cmd
					break
				}
			}
			if offending == "" {
				continue
			}
			norm := normalizeText(offending)
			occurrence[norm]++
			if key := siteKey(rel, offending, occurrence[norm]); allowed[key] && hasRetiredExampleMarker(lg, string(raw)) {
				seenExempt[key] = true
				continue
			}
			line = offending
			t.Errorf("%s:%d tells an operator to copy straight over an install root:\n\t%s\n"+
				"Copying over an already-executed binary can leave a stale code-signing "+
				"cache entry, and macOS then kills it before main() runs — intermittently, "+
				"which is what makes it hard to diagnose. Use `make -C agm install` (or the "+
				"relevant install target), or stage into a UNIQUE path and rename: "+
				"`stage=$(mktemp <dest>.XXXXXX) && cp X \"$stage\" && chmod 755 \"$stage\" "+
				"&& mv -f \"$stage\" <dest>` — a fixed `<dest>.new` is itself racy when two "+
				"jobs run it at once.\n\n"+
				"If this is a WARNING showing the retired form rather than guidance to "+
				"follow, add this line to internal/deploy/testdata/rawcopy-allowlist.txt:\n\n"+
				"\t%s  # %s\n",
				rel, i+1, strings.TrimSpace(line), siteKey(rel, line, occurrence[norm]), rel)
		}
	}

	for key := range allowed {
		if !seenExempt[key] {
			t.Errorf("allowlist entry %s in internal/deploy/testdata/rawcopy-allowlist.txt "+
				"matches no command any more. Remove it so the file keeps describing the "+
				"repository as it is.", key)
		}
	}
}

// trackedMakefiles lists every tracked Makefile and .mk fragment, so a nested
// build file cannot keep its own unhardened install path out of sight.
func trackedMakefiles(t *testing.T, repoRoot string) []string {
	t.Helper()

	var out []string
	for _, rel := range trackedTextFiles(t, repoRoot) {
		base := filepath.Base(rel)
		if base == "Makefile" || strings.HasSuffix(base, ".mk") {
			out = append(out, rel)
		}
	}
	if len(out) == 0 {
		t.Fatal("no tracked Makefiles found; the scan is broken")
	}
	return out
}

// logicalLine is a command with its shell/Make line continuations joined, and
// the 1-based line it starts on.
type logicalLine struct {
	text string
	line int
}

// logicalLines splits text into commands, joining backslash continuations.
//
// Operator guidance conventionally wraps long commands:
//
//	cp bin/foo \
//	  /usr/local/bin/foo
//
// Scanning raw lines sees neither the verb and the destination together, so the
// pattern matches nothing and the guard reports success on the very thing it
// forbids.
func logicalLines(text string) []logicalLine {
	var out []logicalLine
	var buf strings.Builder
	start := 0

	for i, line := range strings.Split(text, "\n") {
		if buf.Len() == 0 {
			start = i + 1
		}
		trimmed := strings.TrimRight(line, " \t")
		if cut, ok := strings.CutSuffix(trimmed, "\\"); ok {
			buf.WriteString(cut)
			buf.WriteString(" ")
			continue
		}
		buf.WriteString(line)
		out = append(out, logicalLine{text: buf.String(), line: start})
		buf.Reset()
	}
	if buf.Len() > 0 {
		out = append(out, logicalLine{text: buf.String(), line: start})
	}
	return out
}

// joinContinuations rebuilds text with continuations joined, preserving line
// starts so the Makefile scanner's anchored patterns still work.
func joinContinuations(text string) string {
	var b strings.Builder
	for _, lg := range logicalLines(text) {
		b.WriteString(lg.text)
		b.WriteString("\n")
	}
	return b.String()
}
