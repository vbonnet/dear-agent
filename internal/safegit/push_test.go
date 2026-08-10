package safegit

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCredentialResetArgs_ResetsThenSetsGhOnly(t *testing.T) {
	got := CredentialResetArgs("/opt/homebrew/bin/gh")
	want := []string{
		"-c", "credential.helper=",
		"-c", "credential.helper=!/opt/homebrew/bin/gh auth git-credential",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("CredentialResetArgs = %q, want %q", got, want)
	}

	// The empty reset MUST come before the gh helper; otherwise osxkeychain is
	// not cleared from the chain and the hang this package prevents returns.
	emptyIdx := slices.Index(got, "credential.helper=")
	ghIdx := -1
	for i, a := range got {
		if strings.HasPrefix(a, "credential.helper=!") {
			ghIdx = i
		}
	}
	if emptyIdx == -1 || ghIdx == -1 || emptyIdx > ghIdx {
		t.Fatalf("empty reset (idx %d) must precede gh helper (idx %d): %q", emptyIdx, ghIdx, got)
	}
}

func TestForceFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		flag string
		want bool
	}{
		{"plain push", []string{"-u", "origin", "main"}, "", false},
		{"short force", []string{"-f", "origin", "main"}, "-f", true},
		{"long force", []string{"--force", "origin", "main"}, "--force", true},
		{"force-with-lease", []string{"--force-with-lease", "origin", "main"}, "--force-with-lease", true},
		{"force-with-lease=ref", []string{"--force-with-lease=main", "origin"}, "--force-with-lease=main", true},
		{"force-if-includes", []string{"--force-if-includes", "origin"}, "--force-if-includes", true},
		{"refspec containing force substring is not a flag", []string{"origin", "feature/force-cleanup"}, "", false},
		{"mirror", []string{"--mirror"}, "--mirror", true},
		{"force refspec +HEAD:main", []string{"origin", "+HEAD:main"}, "+HEAD:main", true},
		{"force refspec wildcard", []string{"origin", "+refs/heads/*:refs/heads/*"}, "+refs/heads/*:refs/heads/*", true},
		{"plain refspec HEAD:main not force", []string{"origin", "HEAD:main"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flag, ok := ForceFlag(tc.args)
			if ok != tc.want || flag != tc.flag {
				t.Fatalf("ForceFlag(%q) = (%q, %v), want (%q, %v)", tc.args, flag, ok, tc.flag, tc.want)
			}
		})
	}
}

func TestPushArgs_Structure(t *testing.T) {
	got := PushArgs("/repo", "/gh", []string{"-u", "origin", "main"})
	want := []string{
		"-C", "/repo",
		"-c", "credential.helper=",
		"-c", "credential.helper=!/gh auth git-credential",
		"push", "-u", "origin", "main",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("PushArgs = %q, want %q", got, want)
	}

	// "push" must come after the credential reset, else the reset is parsed as
	// a push argument and ignored.
	pushIdx := slices.Index(got, "push")
	resetIdx := slices.Index(got, "credential.helper=")
	if resetIdx == -1 || pushIdx == -1 || resetIdx > pushIdx {
		t.Fatalf("credential reset (idx %d) must precede push (idx %d): %q", resetIdx, pushIdx, got)
	}
}

func TestPushArgs_NoRepoDirOmitsDashC(t *testing.T) {
	got := PushArgs("", "/gh", []string{"origin", "main"})
	if slices.Contains(got, "-C") {
		t.Fatalf("expected no -C when repoDir empty, got %q", got)
	}
	if got[0] != "-c" {
		t.Fatalf("expected first arg to be credential -c, got %q", got)
	}
}

func TestPush_RejectsForceBeforeRunning(t *testing.T) {
	// Force is rejected up front, so this never shells out to git.
	err := Push("", []string{"--force", "origin", "main"}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "protected/default") {
		t.Fatalf("expected force rejection, got %v", err)
	}
}

func TestForcePushViolation_AllowsFeatureBranches(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"force-with-lease branch", []string{"--force-with-lease", "origin", "feature/rebased"}},
		{"short force branch", []string{"-f", "origin", "fix/stale-pr"}},
		{"force refspec to feature", []string{"origin", "+HEAD:refs/heads/feature/rebased"}},
		{"implicit current feature", []string{"--force-with-lease", "origin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if target, blocked := ForcePushViolation("", "feature/current", tc.args); blocked {
				t.Fatalf("ForcePushViolation(%q) blocked %q", tc.args, target)
			}
		})
	}
}

func TestForcePushViolation_BlocksProtectedBranches(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"force main", []string{"--force", "origin", "main"}},
		{"force master", []string{"--force-with-lease", "origin", "master"}},
		{"force lease equals main", []string{"--force-with-lease=main", "origin"}},
		{"force refspec to main", []string{"origin", "+HEAD:refs/heads/main"}},
		{"mirror", []string{"--mirror"}},
		{"implicit current main", []string{"--force-with-lease", "origin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := "feature/current"
			if tc.name == "implicit current main" {
				current = "main"
			}
			if _, blocked := ForcePushViolation("", current, tc.args); !blocked {
				t.Fatalf("ForcePushViolation(%q) allowed protected force-push", tc.args)
			}
		})
	}
}
