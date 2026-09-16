package main

import (
	"io/fs"
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

func TestUsageRoutesReviewThreadLifecycleToOwner(t *testing.T) {
	assertRoutesReplyFileLifecycleToOwner(t, usage)
}

func TestSkillRoutesReviewThreadLifecycleToOwner(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", ".claude", "skills", "pr-merge-blockers", "SKILL.md"))
	if err != nil {
		t.Fatalf("read pr-merge-blockers skill: %v", err)
	}
	assertRoutesReplyFileLifecycleToOwner(t, string(content))
}

func assertRoutesReplyFileLifecycleToOwner(t *testing.T, got string) {
	t.Helper()
	if !strings.Contains(got, "resolve-review-threads --help") {
		t.Errorf("review-thread guidance does not route to its owning command:\n%s", got)
	}
}

func TestReplyFileLifecycleHasOneProductionOwner(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	ownerPath := filepath.Join(repoRoot, "cmd", "resolve-review-threads", "recovery_guidance.go")
	owner, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatalf("read canonical reply-file guidance owner: %v", err)
	}
	for _, want := range []string{
		`reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`,
		`--body-file "$reply_file"`,
		`rm -f -- "$reply_file"`,
	} {
		if !strings.Contains(string(owner), want) {
			t.Errorf("canonical reply-file guidance owner missing %q", want)
		}
	}

	walkErr := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".beads", "node_modules", "test", "tests", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !isProductionGuidanceSource(path) || filepath.Clean(path) == filepath.Clean(ownerPath) {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if duplicatesInitialReplyFileLifecycle(string(source)) {
			relative, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				relative = path
			}
			t.Errorf("production guidance %s duplicates the complete owner recipe", relative)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan production guidance: %v", walkErr)
	}
}

func isProductionGuidanceSource(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".json", ".md", ".sh", ".toml", ".txt", ".yaml", ".yml":
		return true
	default:
		return filepath.Base(path) == "Makefile"
	}
}

func duplicatesInitialReplyFileLifecycle(source string) bool {
	normalized := strings.ReplaceAll(source, `\"`, `"`)
	for _, required := range []string{
		`reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`,
		`--body-file "$reply_file"`,
		`rm -f -- "$reply_file"`,
	} {
		if !strings.Contains(normalized, required) {
			return false
		}
	}
	return true
}

func TestCompleteReplyFileLifecycleDetectionDoesNotRejectReferences(t *testing.T) {
	if duplicatesInitialReplyFileLifecycle(`See --body-file "$reply_file" for the parameter contract.`) {
		t.Fatal("a body-file reference alone must not be treated as a duplicated lifecycle")
	}
	if !duplicatesInitialReplyFileLifecycle(`
reply_file=\"$(mktemp /tmp/resolve-review-thread.XXXXXX)\"
resolve-review-threads reply-resolve ID --body-file \"$reply_file\"
rm -f -- \"$reply_file\"
`) {
		t.Fatal("the complete normalized lifecycle must be detected")
	}
}
