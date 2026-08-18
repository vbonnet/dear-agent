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
