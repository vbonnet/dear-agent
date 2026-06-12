package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/safesrc"
)

// TestRun_HelpExitsClean covers the --help/-h fast path: it must print usage and
// return nil without touching the filesystem or git.
func TestRun_HelpExitsClean(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		if err := run([]string{flag}); err != nil {
			t.Errorf("run(%q) = %v, want nil", flag, err)
		}
	}
}

// TestRun_ArgParsingErrors covers each argument-parsing failure branch. None of
// these reach git, so they are deterministic and side-effect free.
func TestRun_ArgParsingErrors(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"unknown flag", []string{"--nope"}, "unknown flag"},
		{"timeout missing arg", []string{"--timeout"}, "requires a duration"},
		{"timeout invalid", []string{"--timeout", "bogus", "~/src/x"}, "invalid --timeout"},
		{"two repo paths", []string{"~/src/a", "~/src/b"}, "exactly one"},
		{"no path", []string{}, "required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := run(c.argv)
			if err == nil {
				t.Fatalf("run(%v) = nil, want error containing %q", c.argv, c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("run(%v) error = %q, want it to contain %q", c.argv, err, c.want)
			}
		})
	}
}

// TestRun_RejectsPathOutsideSrc confirms run() refuses a real path that is not
// under ~/src/ — the boundary safesrc.ValidateRepo enforces, exercised end to
// end through the binary's entry point. A t.TempDir() is never under ~/src.
func TestRun_RejectsPathOutsideSrc(t *testing.T) {
	err := run([]string{t.TempDir()})
	if err == nil {
		t.Fatal("run(<tempdir outside ~/src>) = nil, want a boundary-rejection error")
	}
}

func TestDirtyNote(t *testing.T) {
	if got := dirtyNote(false); got != "" {
		t.Errorf("dirtyNote(false) = %q, want empty", got)
	}
	if got := dirtyNote(true); !strings.Contains(got, "stashed") {
		t.Errorf("dirtyNote(true) = %q, want it to mention stashing", got)
	}
}

func TestStashNote(t *testing.T) {
	if got := stashNote(safesrc.Plan{Stashed: false}, "/x"); got != "" {
		t.Errorf("stashNote(Stashed=false) = %q, want empty", got)
	}
	got := stashNote(safesrc.Plan{Stashed: true}, "/repo")
	if !strings.Contains(got, "stash pop") || !strings.Contains(got, "/repo") {
		t.Errorf("stashNote(Stashed=true) = %q, want it to mention `stash pop` and the repo path", got)
	}
}
