package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/cleanup"
)

func TestSessionReapWorktreesCmd_Metadata(t *testing.T) {
	if sessionReapWorktreesCmd.Use != "reap-worktrees" {
		t.Errorf("Use = %q", sessionReapWorktreesCmd.Use)
	}
	if sessionReapWorktreesCmd.Short == "" {
		t.Error("Short must not be empty")
	}
	if sessionReapWorktreesCmd.RunE == nil {
		t.Error("RunE must be set")
	}
	// Must hang off `agm session`.
	found := false
	for _, c := range sessionCmd.Commands() {
		if c == sessionReapWorktreesCmd {
			found = true
		}
	}
	if !found {
		t.Error("reap-worktrees not registered under sessionCmd")
	}
}

func TestIsParentEscape(t *testing.T) {
	cases := map[string]bool{
		"sub":        false,
		"sub/deeper": false,
		".":          false,
		"..":         true,
		"../sibling": true,
		"/abs/path":  true,
	}
	for in, want := range cases {
		if got := isParentEscape(in); got != want {
			t.Errorf("isParentEscape(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBuildReaperOptions_DefaultsAndFlags(t *testing.T) {
	// Point repo at a non-repo temp dir so currentWorktreeTopLevel resolves
	// to "" (no worktrees) and the test stays deterministic.
	tmp := t.TempDir()
	reapRepoPath = tmp
	reapWorktreesDir = "/custom/worktrees"
	reapDryRun = true
	t.Cleanup(func() {
		reapRepoPath, reapWorktreesDir, reapDryRun = "", "", false
	})

	opts := buildReaperOptions()
	if opts.RepoPath != tmp {
		t.Errorf("RepoPath = %q, want %q", opts.RepoPath, tmp)
	}
	if opts.WorktreesBase != "/custom/worktrees" {
		t.Errorf("WorktreesBase = %q", opts.WorktreesBase)
	}
	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
	if opts.SelfPath != "" {
		t.Errorf("SelfPath should be empty for a non-repo dir, got %q", opts.SelfPath)
	}
}

func TestPrintReapSummary(t *testing.T) {
	var b strings.Builder
	printReapSummary(&b, &cleanup.ReapResult{
		Removed: []string{"/w/b", "/w/a"},
		Kept:    map[string]string{"/w/dirty": "uncommitted-changes"},
	}, false)
	out := b.String()
	// Removed list is sorted.
	if !strings.Contains(out, "Removed: /w/a\nRemoved: /w/b") {
		t.Errorf("removed list not sorted/printed:\n%s", out)
	}
	if !strings.Contains(out, "Removed 2 worktree(s).") {
		t.Errorf("missing removed count:\n%s", out)
	}
	if !strings.Contains(out, "/w/dirty (uncommitted-changes)") {
		t.Errorf("missing kept reason:\n%s", out)
	}

	var d strings.Builder
	printReapSummary(&d, &cleanup.ReapResult{Kept: map[string]string{}}, true)
	if !strings.Contains(d.String(), "No stale agent worktrees to reap.") {
		t.Errorf("empty/dry summary wrong:\n%s", d.String())
	}
}
