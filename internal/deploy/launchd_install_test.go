package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
// already been executed, macOS still holds the code-signing identity it cached
// for that vnode, so the newly written bytes fail validation and the kernel
// SIGKILLs the process with OS_REASON_CODESIGNING *before main() runs*. There
// is no stderr, no exit message, and no log line — for a launchd job, which has
// no terminal, the only symptom is that it silently stops working.
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
		bareCopy := regexp.MustCompile(
			`(?m)^\t@?(?:cp|install)\b[^\n]*\$\((?:HOME\)/(?:go/bin|\.local/bin)|HOOKS_DIR\))[^\n]*$`)
		for _, rel := range trackedMakefiles(t, repoRoot) {
			raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
			if err != nil {
				continue
			}
			for _, m := range bareCopy.FindAllString(string(raw), -1) {
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

// clauseSplit ends a clause at ";" or at sentence punctuation followed by
// whitespace. Requiring the trailing whitespace keeps "claude-code 2.1.205"
// and "settings.json" from splitting mid-token.
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

		for _, m := range clauseMentions(body) {
			key := clauseKey(m.clause)
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
