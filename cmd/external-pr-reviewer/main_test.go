package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestRunReportsSuccessForHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		if got := run([]string{arg}); got != 0 {
			t.Fatalf("run(%s) = %d, want 0", arg, got)
		}
	}
}

func TestEmptyProviderFlagsLeaveTheSupportedDefaults(t *testing.T) {
	argv, err := splitCommand("")
	if err != nil {
		t.Fatalf("splitCommand(\"\") error = %v", err)
	}
	if len(argv) != 0 {
		t.Fatalf("splitCommand(\"\") = %#v, want nothing so the package default applies", argv)
	}
}

func TestRunRejectsUnparseableFlags(t *testing.T) {
	if got := run([]string{"--not-a-flag"}); got != 2 {
		t.Fatalf("run(--not-a-flag) = %d, want 2", got)
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	if got := run([]string{"--repo", "owner/repo", "extra"}); got != 2 {
		t.Fatalf("run(positional) = %d, want 2", got)
	}
}

func TestRunFailsWithoutRepo(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	if got := run([]string{"--state", state}); got != 1 {
		t.Fatalf("run(no --repo) = %d, want 1", got)
	}
}

func TestRunRejectsUnsupportedReviewEvent(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	if got := run([]string{"--repo", "owner/repo", "--event", "merge", "--state", state}); got != 1 {
		t.Fatalf("run(bad --event) = %d, want 1", got)
	}
}

func TestMultiFlagAccumulatesReposInOrder(t *testing.T) {
	var repos multiFlag
	for _, repo := range []string{"owner/one", " owner/two "} {
		if err := repos.Set(repo); err != nil {
			t.Fatalf("Set(%q) error = %v", repo, err)
		}
	}
	if !slices.Equal(repos, multiFlag{"owner/one", "owner/two"}) {
		t.Fatalf("repos = %#v, want [owner/one owner/two]", repos)
	}
	if got := repos.String(); got != "owner/one,owner/two" {
		t.Fatalf("String() = %q, want %q", got, "owner/one,owner/two")
	}
}

func TestMultiFlagRejectsEmptyRepo(t *testing.T) {
	var repos multiFlag
	if err := repos.Set("   "); err == nil {
		t.Fatal("Set(blank) error = nil, want error")
	}
	if len(repos) != 0 {
		t.Fatalf("repos = %#v, want empty", repos)
	}
}

func TestRunRejectsNonPositiveWatchInterval(t *testing.T) {
	for _, interval := range []string{"0", "-1m"} {
		args := []string{"--repo", "owner/repo", "--watch", "--interval", interval}
		if got := run(args); got != 2 {
			t.Fatalf("run(--interval %s) = %d, want 2", interval, got)
		}
	}
}

func TestRunRejectsUnbalancedProviderCommand(t *testing.T) {
	if got := run([]string{"--repo", "owner/repo", "--codex-cmd", `codex "exec`}); got != 2 {
		t.Fatalf("run(unbalanced --codex-cmd) = %d, want 2", got)
	}
}

func TestSplitCommandBuildsArgumentVector(t *testing.T) {
	tests := map[string][]string{
		"":                               nil,
		"   ":                            nil,
		"codex exec -":                   {"codex", "exec", "-"},
		"  agy   run - ":                 {"agy", "run", "-"},
		`"/opt/my tools/codex" exec -`:   {"/opt/my tools/codex", "exec", "-"},
		`agy run --prompt 'two words' -`: {"agy", "run", "--prompt", "two words", "-"},
		`codex --flag=""`:                {"codex", "--flag="},
	}
	for input, want := range tests {
		got, err := splitCommand(input)
		if err != nil {
			t.Fatalf("splitCommand(%q) error = %v", input, err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("splitCommand(%q) = %#v, want %#v", input, got, want)
		}
	}
}

func TestSplitCommandRejectsUnbalancedQuotes(t *testing.T) {
	for _, input := range []string{`codex "exec`, "agy 'run"} {
		if _, err := splitCommand(input); err == nil {
			t.Fatalf("splitCommand(%q) error = nil, want error", input)
		}
	}
}
