package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// TestLifecycleCommitsLeaveIgnoredMarkersUntracked pins the repository ignore
// policy as authoritative. Force-adding ignored canonical markers made the PR
// pipeline block itself: `wayfinder session start` committed paths that
// routing-guard's temporal-debt gate then rejected, so `preflight-full` failed
// and `safe-pr` refused to open the PR until the operator manually ran
// `git rm --cached` on the files Wayfinder had just committed (ce-2sgej).
func TestLifecycleCommitsLeaveIgnoredMarkersUntracked(t *testing.T) {
	lifecycles := map[string]func(*GitIntegrator) (bool, error){
		"session-init":   func(g *GitIntegrator) (bool, error) { return g.CommitSessionInit("my-project") },
		"phase-start":    func(g *GitIntegrator) (bool, error) { return g.CommitPhaseStart("CHARTER") },
		"phase-complete": func(g *GitIntegrator) (bool, error) { return g.CommitPhaseCompletion("CHARTER", "success", "") },
	}

	for name, commitLifecycle := range lifecycles {
		t.Run(name, func(t *testing.T) {
			repoDir := setupGitRepo(t)
			g := New(repoDir)

			for path, content := range map[string]string{
				".gitignore":              "WAYFINDER-STATUS.md\nWAYFINDER-HISTORY.jsonl\n",
				"README.md":               "# Test Project\n",
				"WAYFINDER-STATUS.md":     "# Status\n",
				"WAYFINDER-HISTORY.jsonl": "{}\n",
			} {
				if err := os.WriteFile(filepath.Join(repoDir, path), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			if err := gittest.Command(t, repoDir, "add", ".gitignore", "README.md").Run(); err != nil {
				t.Fatalf("stage initial files: %v", err)
			}
			if err := gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run(); err != nil {
				t.Fatalf("initial commit: %v", err)
			}

			committed, err := commitLifecycle(g)
			if err != nil {
				t.Fatalf("lifecycle commit error = %v", err)
			}
			// The caller prints "Git commit created" off this value, so a skip
			// must report false rather than letting the CLI announce a commit
			// that does not exist.
			if committed {
				t.Error("lifecycle reported a commit while every candidate artifact was ignored")
			}

			tracked, err := gittest.Command(t, repoDir, "ls-files").Output()
			if err != nil {
				t.Fatalf("git ls-files: %v", err)
			}
			for _, marker := range []string{"WAYFINDER-STATUS.md", "WAYFINDER-HISTORY.jsonl"} {
				if strings.Contains(string(tracked), marker) {
					t.Errorf("ignored marker %s became tracked:\n%s", marker, tracked)
				}
			}

			// Skipping the ignored markers must still leave a clean worktree:
			// Git does not report ignored files, so the next start-phase has
			// nothing to refuse over.
			porcelain, err := gittest.Command(t, repoDir, "status", "--porcelain").Output()
			if err != nil {
				t.Fatalf("git status: %v", err)
			}
			if len(strings.TrimSpace(string(porcelain))) != 0 {
				t.Errorf("worktree not clean after lifecycle commit:\n%s", porcelain)
			}
		})
	}
}

// TestLifecycleCommitsStageAlreadyTrackedIgnoredMarkers covers the migration
// case: a repository that already tracks a marker which a later ignore rule
// also matches. Git ignore rules do not apply to tracked paths, so the marker
// must keep receiving its lifecycle updates rather than silently drifting.
func TestLifecycleCommitsStageAlreadyTrackedIgnoredMarkers(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	for path, content := range map[string]string{
		".gitignore":          "WAYFINDER-STATUS.md\n",
		"WAYFINDER-STATUS.md": "# Status\n",
	} {
		if err := os.WriteFile(filepath.Join(repoDir, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := gittest.Command(t, repoDir, "add", "--force", ".gitignore", "WAYFINDER-STATUS.md").Run(); err != nil {
		t.Fatalf("stage initial files: %v", err)
	}
	if err := gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Status CHARTER\n"), 0o644); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if _, err := g.CommitPhaseStart("CHARTER"); err != nil {
		t.Fatalf("CommitPhaseStart() error = %v", err)
	}

	names, err := gittest.Command(t, repoDir, "show", "--name-only", "--format=", "HEAD").Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	if !strings.Contains(string(names), "WAYFINDER-STATUS.md") {
		t.Errorf("tracked marker missing from lifecycle commit:\n%s", names)
	}
}
