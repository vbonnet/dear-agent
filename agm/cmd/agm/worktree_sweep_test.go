package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestWorktreeSweepCmd_Metadata(t *testing.T) {
	if worktreeSweepCmd.Use != "sweep" {
		t.Errorf("Use = %q", worktreeSweepCmd.Use)
	}
	if worktreeSweepCmd.Short == "" || worktreeSweepCmd.RunE == nil {
		t.Error("Short and RunE must be set")
	}
	// sweep hangs off `agm worktree`, which hangs off root.
	foundSweep := false
	for _, c := range worktreeCmd.Commands() {
		if c == worktreeSweepCmd {
			foundSweep = true
		}
	}
	if !foundSweep {
		t.Error("sweep not registered under worktreeCmd")
	}
	foundGroup := false
	for _, c := range rootCmd.Commands() {
		if c == worktreeCmd {
			foundGroup = true
		}
	}
	if !foundGroup {
		t.Error("worktree group not registered under rootCmd")
	}
}

func TestWorktreeSweepCmd_DefaultsToDryRun(t *testing.T) {
	if sweepExecute {
		t.Error("sweep must default to dry-run (sweepExecute false)")
	}
}

func TestAge(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Time{}, "?"},
		{now.Add(-30 * time.Minute), "30m"},
		{now.Add(-5 * time.Hour), "5h"},
		{now.Add(-3 * 24 * time.Hour), "3d"},
	}
	for _, c := range cases {
		if got := age(c.in); got != c.want {
			t.Errorf("age(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	if prCol("") != "-" || prCol("MERGED") != "MERGED" {
		t.Error("prCol formatting wrong")
	}
}

func TestPrintSweepReport(t *testing.T) {
	res := &ops.SweepResult{
		Worktrees: []ops.WorktreeStatus{
			{Path: "/w/m", Repo: "r", Branch: "claude/m", Class: ops.ClassMerged, Reason: "pr-merged", PRState: "MERGED"},
			{Path: "/w/d", Repo: "r", Branch: "claude/d", Class: ops.ClassDirty, Reason: "uncommitted-changes"},
			{Path: "/w/o1", Repo: "r", Branch: "claude/o1", Class: ops.ClassOrphaned, Reason: "no-pr", DupCount: 2},
		},
		Removed: []string{"/w/m"},
	}

	var b strings.Builder
	printSweepReport(&b, res, false)
	out := b.String()
	for _, want := range []string{
		"CLASS", "MERGED=1 DIRTY=1 ORPHANED=1",
		"Would reap 1", "/w/m", "[dup x2]",
		"Re-run with --execute",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run report missing %q:\n%s", want, out)
		}
	}

	var e strings.Builder
	printSweepReport(&e, res, true)
	if !strings.Contains(e.String(), "Reaped 1") {
		t.Errorf("execute report should say Reaped:\n%s", e.String())
	}
	if strings.Contains(e.String(), "Re-run with --execute") {
		t.Errorf("execute report must not nudge to --execute:\n%s", e.String())
	}

	var z strings.Builder
	printSweepReport(&z, &ops.SweepResult{}, false)
	if !strings.Contains(z.String(), "No worktrees found.") {
		t.Errorf("empty report wrong:\n%s", z.String())
	}
}
