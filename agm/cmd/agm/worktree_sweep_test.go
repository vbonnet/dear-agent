package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

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
	printSweepReport(&b, res, false, false)
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
	printSweepReport(&e, res, true, false)
	if !strings.Contains(e.String(), "Reaped 1") {
		t.Errorf("execute report should say Reaped:\n%s", e.String())
	}
	if strings.Contains(e.String(), "Re-run with --execute") {
		t.Errorf("execute report must not nudge to --execute:\n%s", e.String())
	}

	var z strings.Builder
	printSweepReport(&z, &ops.SweepResult{}, false, false)
	if !strings.Contains(z.String(), "No worktrees found.") {
		t.Errorf("empty report wrong:\n%s", z.String())
	}
}

func TestPrintSweepReport_OrphanBranch(t *testing.T) {
	res := &ops.SweepResult{
		Worktrees: []ops.WorktreeStatus{
			{Path: "/w/m", Repo: "r", Branch: "claude/m", Class: ops.ClassMerged, Reason: "pr-merged", PRState: "MERGED"},
			{
				Path: "/w/ob", Repo: "r", Branch: "ce-task", Class: ops.ClassOrphaned, Reason: "no-pr",
				IsOrphanBranch: true, CommitsAboveMergeBase: 3,
			},
		},
		Removed: []string{"/w/m"},
	}

	var b strings.Builder
	printSweepReport(&b, res, false, false)
	out := b.String()
	for _, want := range []string{
		"orphan branch", "ce-task", "commits_above_main=3",
		"open a PR or run `agm worktree sweep --execute`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("orphan section missing %q:\n%s", want, out)
		}
	}

	// --orphan-only mode: only orphan lines
	var oo strings.Builder
	printSweepReport(&oo, res, false, true)
	oout := oo.String()
	if !strings.Contains(oout, "ORPHAN") || !strings.Contains(oout, "ce-task") {
		t.Errorf("orphan-only missing ORPHAN/ce-task:\n%s", oout)
	}
	if strings.Contains(oout, "claude/m") {
		t.Errorf("orphan-only must not list merged worktrees:\n%s", oout)
	}

	// --orphan-only with no orphans
	var no strings.Builder
	printSweepReport(&no, &ops.SweepResult{Worktrees: []ops.WorktreeStatus{
		{Path: "/w/m", Repo: "r", Branch: "claude/m", Class: ops.ClassMerged, Reason: "pr-merged"},
	}}, false, true)
	if !strings.Contains(no.String(), "No orphan branches found.") {
		t.Errorf("orphan-only with no orphans should report empty:\n%s", no.String())
	}
}

// withSweepFlags restores the package-level sweep flags and the live-set seam
// after a test mutates them, so ordering between tests stays irrelevant.
func withSweepFlags(t *testing.T, dir string, execute bool, lookup func(context.Context) (map[string]bool, error)) {
	t.Helper()
	prevDir, prevExecute, prevLookup := sweepWorktreesDir, sweepExecute, sweepActiveSessions
	t.Cleanup(func() {
		sweepWorktreesDir, sweepExecute, sweepActiveSessions = prevDir, prevExecute, prevLookup
	})
	sweepWorktreesDir, sweepExecute, sweepActiveSessions = dir, execute, lookup
}

func failingActiveSessions(context.Context) (map[string]bool, error) {
	return map[string]bool{}, errors.New("dolt unavailable and tmux fallback failed")
}

// TestWorktreeSweep_ExecuteFailsClosedOnActiveLookupFailure is the ce-3knl.1
// regression at the command layer. The lookup failure used to be a warning;
// the sweep then ran with an empty live set, which is precisely the state in
// which a live worktree at origin/main classifies as reapable.
func TestWorktreeSweep_ExecuteFailsClosedOnActiveLookupFailure(t *testing.T) {
	withSweepFlags(t, t.TempDir(), true, failingActiveSessions)

	err := runWorktreeSweep(worktreeSweepCmd, nil)
	if err == nil {
		t.Fatal("--execute must fail when the active-session lookup fails")
	}
	for _, want := range []string{"active sessions", "refusing to execute"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestWorktreeSweep_DryRunSurvivesActiveLookupFailure keeps the read-only
// path usable when Dolt and tmux are both unreachable.
func TestWorktreeSweep_DryRunSurvivesActiveLookupFailure(t *testing.T) {
	withSweepFlags(t, t.TempDir(), false, failingActiveSessions)

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	if err := runWorktreeSweep(cmd, nil); err != nil {
		t.Fatalf("dry run must still classify: %v", err)
	}
}
