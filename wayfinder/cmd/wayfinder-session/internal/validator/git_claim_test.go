package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestValidateGitCommitStatus(t *testing.T) {
	tests := []struct {
		name        string
		phaseName   string
		files       []string // Files to create
		gitCommit   bool     // Whether to commit the files
		wantErr     bool
		errContains string
	}{
		{
			name:      "BUILD - all deliverables committed",
			phaseName: "BUILD",
			files:     []string{"BUILD-implementation.md", "main.go"},
			gitCommit: true,
			wantErr:   false,
		},
		{
			name:        "BUILD - current deliverable allowed but code untracked",
			phaseName:   "BUILD",
			files:       []string{"BUILD-implementation.md", "main.go"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "main.go",
		},
		{
			name:      "RETRO - deliverables committed",
			phaseName: "RETRO",
			files:     []string{"BUILD-implementation.md", "RETRO-retrospective.md"},
			gitCommit: true,
			wantErr:   false,
		},
		{
			name:      "RETRO - current deliverable remains reachable for scoped commit",
			phaseName: "RETRO",
			files:     []string{"RETRO-retrospective.md"},
			gitCommit: false,
			wantErr:   false,
		},
		{
			name:      "RETRO - all phase deliverables committed",
			phaseName: "RETRO",
			files:     []string{"CHARTER-intake.md", "PROBLEM-analysis.md", "BUILD-implementation.md", "RETRO-retrospective.md"},
			gitCommit: true,
			wantErr:   false,
		},
		{
			name:        "RETRO - earlier deliverable untracked",
			phaseName:   "RETRO",
			files:       []string{"BUILD-implementation.md", "RETRO-retrospective.md"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "BUILD-implementation.md",
		},
		{
			name:      "PROBLEM - planning phase (no git validation)",
			phaseName: "PROBLEM",
			files:     []string{"PROBLEM-validation.md"},
			gitCommit: false, // Untracked but PROBLEM is not validated
			wantErr:   false,
		},
		{
			name:      "BUILD - code files committed",
			phaseName: "BUILD",
			files:     []string{"BUILD-implementation.md", "server.py", "client.js"},
			gitCommit: true,
			wantErr:   false,
		},
		{
			name:        "BUILD - code files untracked (VIOLATION)",
			phaseName:   "BUILD",
			files:       []string{"BUILD-implementation.md", "server.py"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "server.py",
		},
		{
			name:        "BUILD - nested code file untracked (VIOLATION)",
			phaseName:   "BUILD",
			files:       []string{"BUILD-implementation.md", "src/foo.go"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "src/foo.go",
		},
		{
			name:      "BUILD - current deliverable and wayfinder internal files allowed",
			phaseName: "BUILD",
			files:     []string{"BUILD-implementation.md", ".wayfinder/session.json"},
			gitCommit: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory as git repo
			tmpDir := t.TempDir()

			// Initialize git repo (the sandbox also supplies the commit identity)
			if err := gittest.Command(t, tmpDir, "init", tmpDir).Run(); err != nil {
				t.Fatalf("failed to init git repo: %v", err)
			}

			// Create files
			for _, fileName := range tt.files {
				filePath := filepath.Join(tmpDir, fileName)

				// Create directory if needed (e.g., .wayfinder/)
				dir := filepath.Dir(filePath)
				if dir != tmpDir {
					os.MkdirAll(dir, 0755)
				}

				// Create file with some content
				content := "Test content for " + fileName
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create file %s: %v", fileName, err)
				}
			}

			// Commit files if requested
			if tt.gitCommit {
				gittest.Command(t, tmpDir, "add", ".").Run()
				gittest.Command(t, tmpDir, "commit", "-m", "test commit").Run()
			}

			// Run validation
			err := validateGitCommitStatus(tmpDir, tt.phaseName)

			// Check result
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected substring %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateGitCommitStatus_NonGitRepo(t *testing.T) {
	// Create temp directory WITHOUT git init
	tmpDir := t.TempDir()

	// Create untracked file
	filePath := filepath.Join(tmpDir, "BUILD-implementation.md")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Should not error - not a git repo
	err := validateGitCommitStatus(tmpDir, "BUILD")
	if err != nil {
		t.Errorf("expected no error for non-git repo, got: %v", err)
	}
}

func TestValidateGitCommitStatus_PartiallyCommitted(t *testing.T) {
	// Test the exact scenario from Instance 2: some files committed, some not
	tmpDir := t.TempDir()

	// Initialize git repo (the sandbox also supplies the commit identity)
	gittest.Command(t, tmpDir, "init", tmpDir).Run()

	// Create and commit WAYFINDER-STATUS.md
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	os.WriteFile(statusPath, []byte("status"), 0644)
	gittest.Command(t, tmpDir, "add", "WAYFINDER-STATUS.md").Run()
	gittest.Command(t, tmpDir, "commit", "-m", "commit status").Run()

	// Create phase deliverables but DON'T commit
	phaseDocs := []string{
		"CHARTER-intake.md",
		"PROBLEM-analysis.md",
		"RESEARCH-existing.md",
		"BUILD-implementation.md",
		"RETRO-retrospective.md",
	}
	for _, doc := range phaseDocs {
		filePath := filepath.Join(tmpDir, doc)
		os.WriteFile(filePath, []byte("content"), 0644)
	}

	// This simulates Instance 2: CLI committed WAYFINDER-STATUS.md
	// but left ~76 phase deliverables uncommitted

	// Validation should fail for the uncommitted earlier-phase deliverables;
	// RETRO-retrospective.md itself remains reachable for the scoped commit.
	err := validateGitCommitStatus(tmpDir, "RETRO")
	if err == nil {
		t.Fatalf("expected error for partially committed files, got nil")
		return
	}
	if !strings.Contains(err.Error(), "BUILD-implementation.md") {
		t.Errorf("error should mention BUILD-implementation.md, got: %v", err)
	}
}

func TestValidateGitCommitStatus_ModifiedTrackedSource(t *testing.T) {
	tmpDir := t.TempDir()

	exec.Command("git", "init", tmpDir).Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

	sourcePath := filepath.Join(tmpDir, "src", "foo.go")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("package src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmpDir, "add", "src/foo.go").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "initial source").Run()

	if err := os.WriteFile(sourcePath, []byte("package src\n\nconst changed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := validateGitCommitStatus(tmpDir, "BUILD")
	if err == nil || !strings.Contains(err.Error(), "src/foo.go") {
		t.Fatalf("expected modified tracked source violation for src/foo.go, got %v", err)
	}
}

func TestValidateGitCommitStatus_RenamedTrackedSource(t *testing.T) {
	tmpDir := t.TempDir()

	exec.Command("git", "init", tmpDir).Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

	sourcePath := filepath.Join(tmpDir, "src", "foo.go")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("package src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmpDir, "add", "src/foo.go").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "initial source").Run()
	if output, err := exec.Command("git", "-C", tmpDir, "mv", "src/foo.go", "README.txt").CombinedOutput(); err != nil {
		t.Fatalf("git mv failed: %v: %s", err, output)
	}

	err := validateGitCommitStatus(tmpDir, "BUILD")
	if err == nil || !strings.Contains(err.Error(), "src/foo.go") {
		t.Fatalf("expected renamed tracked source violation for src/foo.go, got %v", err)
	}
}

func TestGetUncommittedFilesInProjectDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo (the sandbox also supplies the commit identity)
	gittest.Command(t, tmpDir, "init", tmpDir).Run()

	// Create committed file
	committedPath := filepath.Join(tmpDir, "committed.txt")
	os.WriteFile(committedPath, []byte("committed"), 0644)
	gittest.Command(t, tmpDir, "add", "committed.txt").Run()
	gittest.Command(t, tmpDir, "commit", "-m", "initial").Run()

	// Create untracked files
	untrackedPath := filepath.Join(tmpDir, "untracked.txt")
	os.WriteFile(untrackedPath, []byte("untracked"), 0644)

	// Create .wayfinder/ internal file (should be filtered)
	wayfinderDir := filepath.Join(tmpDir, ".wayfinder")
	os.Mkdir(wayfinderDir, 0755)
	wayfinderFile := filepath.Join(wayfinderDir, "session.json")
	os.WriteFile(wayfinderFile, []byte("internal"), 0644)

	// Get untracked files
	untracked, err := getUncommittedFilesInProjectDir(tmpDir)
	if err != nil {
		t.Fatalf("getUncommittedFilesInProjectDir failed: %v", err)
	}

	// Should only include untracked.txt, not .wayfinder/session.json
	if len(untracked) != 1 {
		t.Errorf("expected 1 untracked file, got %d: %v", len(untracked), untracked)
	}

	if len(untracked) > 0 && untracked[0] != "untracked.txt" {
		t.Errorf("expected untracked.txt, got: %s", untracked[0])
	}
}

func TestIsFileUncommitted(t *testing.T) {
	tests := []struct {
		name           string
		fileName       string
		untrackedFiles []string
		want           bool
	}{
		{
			name:           "exact match",
			fileName:       "BUILD-implementation.md",
			untrackedFiles: []string{"BUILD-implementation.md", "main.go"},
			want:           true,
		},
		{
			name:           "path suffix match",
			fileName:       "BUILD-implementation.md",
			untrackedFiles: []string{"docs/BUILD-implementation.md"},
			want:           true,
		},
		{
			name:           "no match",
			fileName:       "BUILD-implementation.md",
			untrackedFiles: []string{"RETRO-retrospective.md"},
			want:           false,
		},
		{
			name:           "empty list",
			fileName:       "BUILD-implementation.md",
			untrackedFiles: []string{},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFileUncommitted(tt.fileName, tt.untrackedFiles)
			if got != tt.want {
				t.Errorf("isFileUncommitted() = %v, want %v", got, tt.want)
			}
		})
	}
}
