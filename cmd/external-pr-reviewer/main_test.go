package main

import (
	"path/filepath"
	"slices"
	"testing"
)

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

func TestSplitCommandBuildsArgumentVector(t *testing.T) {
	tests := map[string][]string{
		"":               nil,
		"   ":            nil,
		"codex exec -":   {"codex", "exec", "-"},
		"  agy   run - ": {"agy", "run", "-"},
	}
	for input, want := range tests {
		if got := splitCommand(input); !slices.Equal(got, want) {
			t.Fatalf("splitCommand(%q) = %#v, want %#v", input, got, want)
		}
	}
}
