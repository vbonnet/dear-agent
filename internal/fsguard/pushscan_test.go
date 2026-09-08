package fsguard

import (
	"path/filepath"
	"reflect"
	"testing"
)

func pushScanGuard() *Guard { return &Guard{Home: "/home/u"} }

// A force-push named on the command line must be found with the right repo
// directory, whichever of the two push front-ends the caller used.
func TestScanPushesFindsBothFrontEnds(t *testing.T) {
	g := pushScanGuard()
	tests := []struct {
		name     string
		command  string
		cwd      string
		wantDir  string
		wantArgs []string
	}{
		{
			name:     "bare git push",
			command:  "git push --force-with-lease origin feature/foo",
			cwd:      "/w/repo",
			wantDir:  "/w/repo",
			wantArgs: []string{"--force-with-lease", "origin", "feature/foo"},
		},
		{
			name:     "git -C overrides cwd",
			command:  "git -C /w/other push -f origin fix/bar",
			cwd:      "/w/repo",
			wantDir:  "/w/other",
			wantArgs: []string{"-f", "origin", "fix/bar"},
		},
		{
			name:     "safe-push is a push front-end",
			command:  "safe-push --force-with-lease origin stack/rebased",
			cwd:      "/w/repo",
			wantDir:  "/w/repo",
			wantArgs: []string{"--force-with-lease", "origin", "stack/rebased"},
		},
		{
			name:     "safe-push -C and --timeout are consumed, not forwarded",
			command:  "safe-push -C /w/other --timeout 60s -f origin feature/x",
			cwd:      "/w/repo",
			wantDir:  "/w/other",
			wantArgs: []string{"-f", "origin", "feature/x"},
		},
		{
			name:     "cd tracking sets the repo directory",
			command:  "cd /w/other && git push -f origin feature/x",
			cwd:      "/w/repo",
			wantDir:  "/w/other",
			wantArgs: []string{"-f", "origin", "feature/x"},
		},
		{
			name:     "tilde in -C expands to home",
			command:  "git -C ~/worktrees/r/b push -f origin feature/x",
			cwd:      "/w/repo",
			wantDir:  filepath.Join("/home/u", "worktrees/r/b"),
			wantArgs: []string{"-f", "origin", "feature/x"},
		},
		{
			name:     "an absolute git path is still git",
			command:  "/usr/bin/git push -f origin feature/x",
			cwd:      "/w/repo",
			wantDir:  "/w/repo",
			wantArgs: []string{"-f", "origin", "feature/x"},
		},
		{
			name:     "a command runner prefix is peeled off",
			command:  "env GIT_TRACE=1 git push -f origin feature/x",
			cwd:      "/w/repo",
			wantDir:  "/w/repo",
			wantArgs: []string{"-f", "origin", "feature/x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := g.ScanPushes(tt.command, tt.cwd)
			if !ok {
				t.Fatalf("ScanPushes(%q) not parseable", tt.command)
			}
			if len(got) != 1 {
				t.Fatalf("got %d invocations, want 1: %+v", len(got), got)
			}
			if got[0].RepoDir != tt.wantDir {
				t.Errorf("RepoDir = %q, want %q", got[0].RepoDir, tt.wantDir)
			}
			if !reflect.DeepEqual(got[0].Args, tt.wantArgs) {
				t.Errorf("Args = %q, want %q", got[0].Args, tt.wantArgs)
			}
		})
	}
}

// The defect this scanner replaces: a compound command was flattened into a
// single argument list, so operands from one command leaked into another and a
// legitimate feature-branch force-push was refused because an unrelated
// earlier command in the same chain mentioned main.
func TestScanPushesKeepsCompoundCommandsSeparate(t *testing.T) {
	g := pushScanGuard()
	cmd := "safe-push -u origin HEAD:main && git checkout -b feature/x && " +
		"safe-push --force-with-lease origin feature/x"
	got, ok := g.ScanPushes(cmd, "/w/repo")
	if !ok {
		t.Fatal("not parseable")
	}
	if len(got) != 2 {
		t.Fatalf("got %d invocations, want 2: %+v", len(got), got)
	}
	if want := []string{"-u", "origin", "HEAD:main"}; !reflect.DeepEqual(got[0].Args, want) {
		t.Errorf("first Args = %q, want %q", got[0].Args, want)
	}
	if want := []string{"--force-with-lease", "origin", "feature/x"}; !reflect.DeepEqual(got[1].Args, want) {
		t.Errorf("second Args = %q, want %q", got[1].Args, want)
	}
}

// The other half of the same defect: matching command *text* rather than
// parsing it blocked commands that only mentioned a push inside a quoted
// string, which is how a grep over the guard's own message became unrunnable.
func TestScanPushesIgnoresNonPushCommands(t *testing.T) {
	g := pushScanGuard()
	for _, cmd := range []string{
		`grep -rn "git push --force origin main" docs/`,
		`echo "never git push --force to main"`,
		`git log --oneline -3`,
		`git fetch origin`,
		`gh pr view 1218`,
	} {
		got, ok := g.ScanPushes(cmd, "/w/repo")
		if !ok {
			t.Fatalf("ScanPushes(%q) not parseable", cmd)
		}
		if len(got) != 0 {
			t.Errorf("ScanPushes(%q) = %+v, want no invocations", cmd, got)
		}
	}
}

// A `cd` that may not have run leaves the repository ambiguous. The scanner
// reports every candidate directory so the caller can refuse when any one of
// them is protected, rather than trusting the post-cd guess. Here the `cd`
// itself is guarded by &&, so the shell may still be in the original
// directory when the push runs.
func TestScanPushesReportsAmbiguousDirectories(t *testing.T) {
	g := pushScanGuard()
	got, ok := g.ScanPushes("git fetch && cd /w/other; git push -f origin feature/x", "/w/repo")
	if !ok {
		t.Fatal("not parseable")
	}
	if len(got) != 1 {
		t.Fatalf("got %d invocations, want 1", len(got))
	}
	if want := []string{"/w/repo"}; !reflect.DeepEqual(got[0].AlsoDirs, want) {
		t.Errorf("AlsoDirs = %q, want %q", got[0].AlsoDirs, want)
	}
}

// An unconditional `cd` definitely ran, so it leaves no ambiguity behind.
func TestScanPushesUnconditionalCdResolvesDirectory(t *testing.T) {
	g := pushScanGuard()
	got, ok := g.ScanPushes("cd /w/other; git push -f origin feature/x", "/w/repo")
	if !ok {
		t.Fatal("not parseable")
	}
	if len(got) != 1 {
		t.Fatalf("got %d invocations, want 1", len(got))
	}
	if got[0].RepoDir != "/w/other" || len(got[0].AlsoDirs) != 0 {
		t.Errorf("RepoDir = %q, AlsoDirs = %q; want /w/other and none",
			got[0].RepoDir, got[0].AlsoDirs)
	}
}

// An unterminated quote is not a parse the caller may act on.
func TestScanPushesFailsOpenOnUnparseableCommand(t *testing.T) {
	g := pushScanGuard()
	if _, ok := g.ScanPushes(`git push -f origin "unterminated`, "/w/repo"); ok {
		t.Error("ScanPushes reported ok on an unterminated quote")
	}
}
