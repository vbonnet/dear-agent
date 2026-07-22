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
			// The unsafe pattern: `cp bin/<name> $(HOME)/go/bin/`.
			unsafe := "cp bin/" + bin + " $(HOME)/go/bin/"
			if strings.Contains(makefile, unsafe) {
				t.Errorf("%s is launchd-managed but installed with a bare cp.\n"+
					"Found: %q\n"+
					"Use:   $(call install-go-bin,bin/%s)\n"+
					"A bare cp rewrites the existing inode; macOS then kills the binary with "+
					"OS_REASON_CODESIGNING before main() runs, silently disabling the launchd job.",
					bin, unsafe, bin)
			}

			// And it must positively use the macro, so deleting the cp line
			// without replacing it does not pass.
			want := "$(call install-go-bin,bin/" + bin + ")"
			if !strings.Contains(makefile, want) {
				t.Errorf("%s is launchd-managed but its install target does not use the hardened macro.\n"+
					"Expected the Makefile to contain: %s", bin, want)
			}
		})
	}
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

	// Prose that offers the helper as a destination for this binary. Checked
	// per-mention, not per-file: a file-level "does it contain a warning
	// somewhere" test is too weak, because the exact defect review found was a
	// package doc that carried a retirement note at the top AND still said
	// "designed for use as a Claude Code apiKeyHelper" further down.
	recommends := regexp.MustCompile(`(?i)(designed for use as|drives this on a schedule or as|points? Claude Code's [^.\n]*at)`)

	// A recommendation reads as retired when the surrounding lines disclaim it.
	disclaimed := regexp.MustCompile(`(?i)\b(not|never|retired|removed|no longer|void|originally|instead of)\b`)

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
			// Examine each apiKeyHelper mention with one line of context either
			// side, since prose wraps across lines.
			lines := strings.Split(body, "\n")
			for i, line := range lines {
				if !strings.Contains(strings.ToLower(line), "apikeyhelper") {
					continue
				}
				lo, hi := i-1, i+2
				if lo < 0 {
					lo = 0
				}
				if hi > len(lines) {
					hi = len(lines)
				}
				window := strings.Join(lines[lo:hi], "\n")
				if recommends.MatchString(window) && !disclaimed.MatchString(window) {
					t.Errorf("%s:%d presents apiKeyHelper as a supported wiring:\n\t%s\n"+
						"Describe it only as retired, with the reason, so a reader cannot "+
						"reconstruct the auth-shadowing configuration.",
						rel, i+1, strings.TrimSpace(line))
				}
			}
		})
	}
}

// operatorFacingFiles returns the repository files an operator could act on:
// the Makefile, Go sources, Markdown docs, and launchd templates. Vendored,
// generated, and VCS trees are skipped, as are this guard's own sources (which
// necessarily contain the forbidden strings as test fixtures).
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
		switch {
		case name == "Makefile",
			strings.HasSuffix(name, ".go"),
			strings.HasSuffix(name, ".md"),
			strings.HasSuffix(name, ".plist"):
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
