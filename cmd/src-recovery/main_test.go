package main

import (
	"strings"
	"testing"
	"time"

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

// TestParseUnlockArgs covers the unlock subcommand's flag parsing in isolation,
// including help, the defaults, and each error branch.
func TestParseUnlockArgs(t *testing.T) {
	t.Run("help short-circuits", func(t *testing.T) {
		_, showed, err := parseUnlockArgs([]string{"--help"})
		if err != nil || !showed {
			t.Fatalf("parseUnlockArgs(--help) = (showed=%v, err=%v), want (true, nil)", showed, err)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		opts, _, err := parseUnlockArgs([]string{"~/src/x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.dryRun || opts.repoArg != "~/src/x" || opts.minAge != safesrc.DefaultMinLockAge {
			t.Fatalf("defaults wrong: %+v", opts)
		}
	})
	t.Run("dry-run and min-age", func(t *testing.T) {
		opts, _, err := parseUnlockArgs([]string{"--dry-run", "--min-age", "5m", "~/src/x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !opts.dryRun || opts.minAge != 5*time.Minute {
			t.Fatalf("flags not applied: %+v", opts)
		}
	})
	errCases := []struct {
		name, want string
		argv       []string
	}{
		{"min-age missing", "requires a duration", []string{"--min-age"}},
		{"min-age invalid", "invalid --min-age", []string{"--min-age", "nope", "~/src/x"}},
		{"unknown flag", "unknown flag", []string{"--bogus"}},
		{"two paths", "exactly one", []string{"~/src/a", "~/src/b"}},
	}
	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := parseUnlockArgs(c.argv)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("parseUnlockArgs(%v) err = %v, want containing %q", c.argv, err, c.want)
			}
		})
	}
}

// TestRunUnlock_RejectsPathOutsideSrc confirms the unlock path also enforces the
// ~/src boundary end to end.
func TestRunUnlock_RejectsPathOutsideSrc(t *testing.T) {
	if err := runUnlock([]string{t.TempDir()}); err == nil {
		t.Fatal("runUnlock(<tempdir outside ~/src>) = nil, want a boundary-rejection error")
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
