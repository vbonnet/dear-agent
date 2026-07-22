package deploy

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// programArgumentsBinary matches a ~/go/bin path inside a launchd template's
// ProgramArguments, e.g. <string>__HOME__/go/bin/token-refresher</string>.
var programArgumentsBinary = regexp.MustCompile(`(?:__HOME__|\$\{?HOME\}?|/Users/[^/<]+)/go/bin/([A-Za-z0-9._-]+)`)

// scannedExtensions are the file types the apiKeyHelper guard reads: prose and
// code, plus the configuration and script surfaces that can set the key or run
// the wiring command directly (.json above all — apiKeyHelper is a
// settings.json key).
var scannedExtensions = map[string]bool{
	".go": true, ".md": true, ".plist": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true,
	".sh": true, ".bash": true, ".zsh": true,
}

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

	if !strings.Contains(makefile, "define install-go-bin") {
		t.Fatal("Makefile has no install-go-bin macro: the hardened install path is gone")
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
	t.Run("no-bare-cp-installs", func(t *testing.T) {
		bareCopy := regexp.MustCompile(`(?m)^\t@?cp bin/.*\$\((?:HOME\)/go/bin|HOOKS_DIR)\)?/?\s*$`)
		for _, m := range bareCopy.FindAllString(makefile, -1) {
			t.Errorf("install target uses a bare cp instead of the hardened macro:\n\t%s\n"+
				"Use $(call install-go-bin,bin/<name>[,<dest-dir>]). A bare cp rewrites the "+
				"existing inode; macOS then kills the rebuilt binary with OS_REASON_CODESIGNING "+
				"before main() runs.", strings.TrimSpace(m))
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

// TestNoAPIKeyHelperInstructions asserts that no build or documentation path
// tells an operator to wire token-refresher in as Claude Code's apiKeyHelper.
//
// Since claude-code 2.1.205 a configured apiKeyHelper is treated as an external
// API key that shadows a healthy claude.ai OAuth login and refuses to fall back
// to it, so `claude -p` fails with "Invalid API key" even when
// ~/.claude/.credentials.json is fresh (anthropics/claude-code#11587, #9694,
// #23568). That wiring caused a multi-day mesh outage and was removed from the
// host on 2026-07-10.
//
// The Makefile used to print the wiring command as activation step 2, so an
// operator following the documented install path reconstructed the outage.
// Prose warnings sitting next to a copy-pasteable fatal command are not a
// control; this test is.
//
// It scans every operator-facing surface in the repository rather than a
// hand-listed pair of files: the first version of this guard checked only the
// Makefile and one README, and review caught three further surfaces
// (the package doc, the launchd template, and pkg/llm/auth/README.md) that
// still recommended the setting. A guard narrower than the invariant it
// defends is how the invariant comes back.
func TestNoAPIKeyHelperInstructions(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	// The setting command in any form — this is what must never reappear.
	setCommand := regexp.MustCompile(`configure-claude-settings\s+set\s+apiKeyHelper`)

	// Default-deny, per mention. An earlier version of this guard blocklisted a
	// handful of observed recommending phrasings, which is default-allow: a new
	// wording like "Use token-refresher as Claude Code's apiKeyHelper" sails
	// straight through. Enumerating the ways prose can recommend something is
	// unwinnable.
	//
	// So instead: EVERY mention of apiKeyHelper must be disclaimed by its own
	// local context. A file may discuss the helper as much as it likes, but
	// never without saying, right there, that the wiring is retired.
	//
	// Checked per mention rather than per file on purpose. The defect review
	// found was a package doc carrying a retirement note at the top while still
	// recommending the helper further down — a file-level check passes that.
	//
	// The accepted tokens must be genuinely NEGATIVE about the wiring. An
	// earlier revision also accepted polarity-free words — "instead", "stop",
	// "originally", "remove", "void" — which let affirmative guidance through:
	// "Instead, use token-refresher as Claude Code's apiKeyHelper" contains no
	// `set` command and would have passed. Every token below asserts the wiring
	// is absent, forbidden, or harmful; none of them can appear in a sentence
	// that recommends it.
	disclaimed := regexp.MustCompile(`(?i)(\b(not|never|retired|removed|deprecated|disabled|no longer|prohibit\w*|forbid\w*|must\s+not|do\s+not|don't)\b|shadow\w*|\bharmful\b)`)

	for _, path := range operatorFacingFiles(t, repoRoot) {
		rel, _ := filepath.Rel(repoRoot, path)
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			body := string(raw)

			if loc := setCommand.FindString(body); loc != "" {
				t.Errorf("%s instructs wiring apiKeyHelper (%q).\n"+
					"apiKeyHelper shadows healthy OAuth on claude-code >= 2.1.205 and was "+
					"retired on 2026-07-10 (anthropics/claude-code#11587). "+
					"The launchd idle backstop is the only sanctioned wiring.\n"+
					"Cleanup guidance (`configure-claude-settings remove apiKeyHelper`) is "+
					"still allowed — hosts that followed the old instructions need it.",
					rel, loc)
			}
			// Check EVERY mention against its OWN clause.
			//
			// The disclaimer must describe the helper, not merely share a
			// paragraph with it: "Do not use launchd; use token-refresher as
			// apiKeyHelper" puts the negation in a different clause entirely.
			// Clauses split on ";" and on sentence punctuation followed by
			// whitespace — the trailing-space requirement keeps version strings
			// ("2.1.205") and filenames ("settings.json") intact.
			//
			// Enumerating occurrences rather than lines matters: an earlier
			// implementation resolved each line to the FIRST clause mentioning
			// the helper, so in "apiKeyHelper is retired. Use token-refresher
			// as apiKeyHelper" the affirmative second mention re-validated
			// against the disclaimed first one and passed.
			for _, m := range clauseMentions(body, "apikeyhelper") {
				if disclaimed.MatchString(m.clause) {
					continue
				}
				t.Errorf("%s:%d mentions apiKeyHelper without disclaiming it:\n\t%s\n"+
					"Every mention must say, in its own clause, that this wiring is "+
					"retired — otherwise a reader can reconstruct the auth-shadowing "+
					"configuration. claude-code >= 2.1.205 treats a configured helper "+
					"as an external API key that shadows healthy OAuth "+
					"(anthropics/claude-code#11587); it was removed from the host on "+
					"2026-07-10. Cleanup guidance "+
					"(`configure-claude-settings remove apiKeyHelper`) is fine.",
					rel, m.line, strings.TrimSpace(m.clause))
			}
		})
	}
}

// operatorFacingFiles returns the repository files an operator could act on:
// prose, code, and — critically — the configuration and script surfaces that
// can enable the setting directly. Vendored, generated, and VCS trees are
// skipped, as are this guard's own sources (which necessarily contain the
// forbidden strings as test fixtures).
//
// Configuration types are included because they are the most direct route to
// the failure, not an afterthought: `apiKeyHelper` is a key in
// `.claude/settings.json`, so a tracked settings file could restore OAuth
// shadowing with no prose and no Makefile change at all. Shell installers can
// likewise print or run the wiring command. A guard limited to prose, Go, and
// plists would watch every door but the one the setting actually walks
// through.
func operatorFacingFiles(t *testing.T, repoRoot string) []string {
	t.Helper()

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "testdata": true,
		"bin": true, "dist": true,
	}

	var out []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are not this test's concern
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		// The guard's own file states the forbidden patterns verbatim.
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if name == "Makefile" || scannedExtensions[filepath.Ext(name)] {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", repoRoot, err)
	}
	if len(out) == 0 {
		t.Fatal("no operator-facing files found; the walk is broken")
	}
	return out
}

// clauseSplit ends a clause at ";" or at sentence punctuation followed by
// whitespace. The trailing-whitespace requirement is deliberate: it keeps
// "claude-code 2.1.205" and "settings.json" from being split mid-token.
var clauseSplit = regexp.MustCompile(`;|[.!?]\s`)

// mention is one occurrence of a term, with the clause it sits in and the
// 1-based line it starts on.
type mention struct {
	clause string
	line   int
}

// clauseMentions returns EVERY clause of text that contains needle (compared
// lower-cased), each with its line number.
//
// Returning every occurrence rather than the first is the point: a guard that
// resolves all mentions in a region to one clause lets a disclaimed mention
// vouch for an undisclaimed one next to it.
func clauseMentions(text, needle string) []mention {
	var out []mention

	add := func(clause string, start int) {
		if !strings.Contains(strings.ToLower(clause), needle) {
			return
		}
		out = append(out, mention{
			clause: clause,
			line:   1 + strings.Count(text[:start], "\n"),
		})
	}

	start := 0
	for _, sep := range clauseSplit.FindAllStringIndex(text, -1) {
		add(text[start:sep[0]], start)
		start = sep[1]
	}
	if start < len(text) {
		add(text[start:], start)
	}
	return out
}
