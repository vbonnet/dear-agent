package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

func TestRun_UsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "two positionals", args: []string{"1", "2"}},
		{name: "zero pr", args: []string{"--pr", "0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(tc.args, os.Stdout); got != 2 {
				t.Fatalf("run(%v) = %d, want 2", tc.args, got)
			}
		})
	}
}

func captureHuman(t *testing.T, d safegit.Diagnosis) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	printHuman(f, d)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Clean(f.Name()))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPrintHuman_ReadyNamesSafeMerge(t *testing.T) {
	out := captureHuman(t, safegit.Diagnosis{
		Repo:    "owner/repo",
		PR:      safegit.PRState{Number: 7, State: "OPEN", MergeStateStatus: "CLEAN"},
		Verdict: safegit.VerdictReady,
	})
	if !strings.Contains(out, "safe-merge --pr 7") {
		t.Errorf("READY output must point at safe-merge, got:\n%s", out)
	}
}

func TestPrintHuman_BlockedListsFixesAndForbidsGuessing(t *testing.T) {
	out := captureHuman(t, safegit.Diagnosis{
		Repo:    "owner/repo",
		PR:      safegit.PRState{Number: 7, State: "OPEN", MergeStateStatus: "BEHIND"},
		Verdict: safegit.VerdictBlocked,
		Blockers: []safegit.Blocker{
			{Code: safegit.BlockBehind, Detail: "out of date", Fix: "gh pr update-branch 7"},
		},
	})
	for _, want := range []string{"BEHIND", "gh pr update-branch 7", "Do not investigate anything else"} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked output missing %q, got:\n%s", want, out)
		}
	}
}

func TestUsageUsesExternalTemporaryBodyFileLifecycle(t *testing.T) {
	assertExternalTemporaryReplyFileLifecycle(t, usage)
}

func TestSkillUsesExternalTemporaryBodyFileLifecycle(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", ".claude", "skills", "pr-merge-blockers", "SKILL.md"))
	if err != nil {
		t.Fatalf("read pr-merge-blockers skill: %v", err)
	}
	source := string(content)
	for _, token := range []string{
		`reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`,
		`--body-file "$reply_file"`,
		`rm -f -- "$reply_file"`,
	} {
		if got := strings.Count(source, token); got != 2 {
			t.Fatalf("pr-merge-blockers skill contains %d occurrences of %q, want table and detailed recipe", got, token)
		}
	}

	var tableRecipe string
	for line := range strings.SplitSeq(source, "\n") {
		if strings.Contains(line, "| `UNRESOLVED_THREADS` |") {
			tableRecipe = line
			break
		}
	}
	if tableRecipe == "" {
		t.Fatal("pr-merge-blockers skill has no UNRESOLVED_THREADS table recipe")
	}
	t.Run("table recipe", func(t *testing.T) {
		assertExternalTemporaryReplyFileLifecycle(t, tableRecipe)
	})

	const blockMarker = "# for each thread, create a fresh reply source"
	start := strings.Index(source, blockMarker)
	if start < 0 {
		t.Fatalf("pr-merge-blockers skill has no detailed per-thread recipe marked by %q", blockMarker)
	}
	end := strings.Index(source[start:], "\n```")
	if end < 0 {
		t.Fatal("pr-merge-blockers skill detailed per-thread recipe has no closing code fence")
	}
	detailedRecipe := source[start : start+end]
	t.Run("detailed recipe", func(t *testing.T) {
		assertExternalTemporaryReplyFileLifecycle(t, detailedRecipe)
	})
}

func assertExternalTemporaryReplyFileLifecycle(t *testing.T, got string) {
	t.Helper()
	normalized := strings.ToLower(got)
	for _, want := range []string{
		`reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`,
		`--body-file "$reply_file"`,
		`rm -f -- "$reply_file"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reply-file guidance missing %q:\n%s", want, got)
		}
	}
	for _, want := range []string{
		"for each thread",
		"exact-body retry remains valid",
		"file unchanged",
		"terminal resolution is confirmed",
		"only then",
		"safe-merge",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("reply-file guidance missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"reply.md",
		`reply-resolve <threadId> "`,
		`--body-file $reply_file`,
		"mktemp -t",
		"mktemp -u",
		"trap ",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("reply-file guidance contains unsafe form %q:\n%s", forbidden, got)
		}
	}
	create := strings.Index(got, `reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`)
	bodyFile := strings.Index(got, `--body-file "$reply_file"`)
	retention := strings.Index(normalized, "file unchanged")
	confirmation := strings.Index(normalized, "terminal resolution is confirmed")
	cleanup := strings.Index(got, `rm -f -- "$reply_file"`)
	nextThread := strings.Index(normalized, "next thread")
	merge := strings.LastIndex(normalized, "safe-merge")
	if create >= bodyFile || bodyFile >= retention || retention >= confirmation || confirmation >= cleanup || cleanup >= nextThread || cleanup >= merge {
		t.Errorf("reply-file guidance lifecycle is out of order (create=%d body=%d retain=%d confirm=%d cleanup=%d next=%d merge=%d):\n%s",
			create, bodyFile, retention, confirmation, cleanup, nextThread, merge, got)
	}
	if sweep := strings.Index(got, "resolve-review-threads resolve-all"); sweep >= 0 && cleanup >= sweep {
		t.Errorf("reply-file cleanup must precede resolve-all (cleanup=%d sweep=%d):\n%s", cleanup, sweep, got)
	}
}
