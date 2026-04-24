package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveBranch(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		sessName  string
		want      string
	}{
		{
			name:      "with session ID",
			sessionID: "abc-123",
			sessName:  "my-session",
			want:      "agm/abc-123",
		},
		{
			name:      "without session ID",
			sessionID: "",
			sessName:  "my-session",
			want:      "my-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBranch(tt.sessionID, tt.sessName)
			if got != tt.want {
				t.Errorf("resolveBranch(%q, %q) = %q, want %q", tt.sessionID, tt.sessName, got, tt.want)
			}
		})
	}
}

func TestDetectMainBranch(t *testing.T) {
	// Create a temp git repo with a "main" branch
	tmpDir := t.TempDir()
	runGit(t, tmpDir, "init", "--initial-branch=main")
	runGit(t, tmpDir, "config", "user.email", "test@test.com")
	runGit(t, tmpDir, "config", "user.name", "Test")

	// Create an initial commit so main exists
	dummyFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(dummyFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "initial")

	got := detectMainBranch(tmpDir)
	if got != "main" {
		t.Errorf("detectMainBranch() = %q, want %q", got, "main")
	}
}

func TestBranchExists(t *testing.T) {
	tmpDir := t.TempDir()
	runGit(t, tmpDir, "init", "--initial-branch=main")
	runGit(t, tmpDir, "config", "user.email", "test@test.com")
	runGit(t, tmpDir, "config", "user.name", "Test")

	dummyFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(dummyFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "initial")

	if !branchExists(tmpDir, "main") {
		t.Error("branchExists(main) should be true")
	}
	if branchExists(tmpDir, "nonexistent") {
		t.Error("branchExists(nonexistent) should be false")
	}
}

func TestGetCommitsOnBranch(t *testing.T) {
	tmpDir := t.TempDir()
	runGit(t, tmpDir, "init", "--initial-branch=main")
	runGit(t, tmpDir, "config", "user.email", "test@test.com")
	runGit(t, tmpDir, "config", "user.name", "Test")

	// Initial commit on main
	dummyFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(dummyFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "initial commit")

	// Create a feature branch with commits
	runGit(t, tmpDir, "checkout", "-b", "agm/test-session")
	featureFile := filepath.Join(tmpDir, "feature.go")
	if err := os.WriteFile(featureFile, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "feat: add feature")

	secondFile := filepath.Join(tmpDir, "second.go")
	if err := os.WriteFile(secondFile, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "feat: add second file")

	// Check commits on branch
	commits, err := getCommitsOnBranch(tmpDir, "agm/test-session")
	if err != nil {
		t.Fatalf("getCommitsOnBranch() error = %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].Subject != "feat: add second file" {
		t.Errorf("first commit subject = %q, want %q", commits[0].Subject, "feat: add second file")
	}
	if commits[1].Subject != "feat: add feature" {
		t.Errorf("second commit subject = %q, want %q", commits[1].Subject, "feat: add feature")
	}
}

func TestGetCommitsOnBranch_NoCommits(t *testing.T) {
	tmpDir := t.TempDir()
	runGit(t, tmpDir, "init", "--initial-branch=main")
	runGit(t, tmpDir, "config", "user.email", "test@test.com")
	runGit(t, tmpDir, "config", "user.name", "Test")

	dummyFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(dummyFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "initial commit")

	// Create branch at same point as main (no new commits)
	runGit(t, tmpDir, "checkout", "-b", "agm/empty-session")

	commits, err := getCommitsOnBranch(tmpDir, "agm/empty-session")
	if err != nil {
		t.Fatalf("getCommitsOnBranch() error = %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2024-01-01T00:00:00+00:00",
		"GIT_COMMITTER_DATE=2024-01-01T00:00:00+00:00",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
