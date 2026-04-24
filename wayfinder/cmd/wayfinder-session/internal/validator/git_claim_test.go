package validator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
			name:        "BUILD - deliverable untracked (VIOLATION - oss-55e)",
			phaseName:   "BUILD",
			files:       []string{"BUILD-implementation.md", "main.go"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "deliverable files exist but are not committed to git",
		},
		{
			name:      "RETRO - deliverables committed",
			phaseName: "RETRO",
			files:     []string{"BUILD-implementation.md", "RETRO-retrospective.md"},
			gitCommit: true,
			wantErr:   false,
		},
		{
			name:        "RETRO - retro doc untracked (VIOLATION - Instance 1 from today)",
			phaseName:   "RETRO",
			files:       []string{"RETRO-retrospective.md"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "RETRO-retrospective.md",
		},
		{
			name:      "RETRO - all phase deliverables committed",
			phaseName: "RETRO",
			files:     []string{"CHARTER-intake.md", "PROBLEM-analysis.md", "BUILD-implementation.md", "RETRO-retrospective.md"},
			gitCommit: true,
			wantErr:   false,
		},
		{
			name:        "RETRO - retrospective untracked (VIOLATION - Instance 2 from today)",
			phaseName:   "RETRO",
			files:       []string{"RETRO-retrospective.md"},
			gitCommit:   false,
			wantErr:     true,
			errContains: "RETRO-retrospective.md",
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
			name:        "BUILD - wayfinder internal files ignored",
			phaseName:   "BUILD",
			files:       []string{"BUILD-implementation.md", ".wayfinder/session.json"},
			gitCommit:   false, // .wayfinder/session.json is untracked but should be ignored
			wantErr:     true,  // Only BUILD-implementation.md causes error
			errContains: "BUILD-implementation.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory as git repo
			tmpDir, err := os.MkdirTemp("", "git-claim-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Initialize git repo
			if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
				t.Fatalf("failed to init git repo: %v", err)
			}

			// Configure git user for commits
			exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
			exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

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
				exec.Command("git", "-C", tmpDir, "add", ".").Run()
				exec.Command("git", "-C", tmpDir, "commit", "-m", "test commit").Run()
			}

			// Run validation
			err = validateGitCommitStatus(tmpDir, tt.phaseName)

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
	tmpDir, err := os.MkdirTemp("", "non-git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create untracked file
	filePath := filepath.Join(tmpDir, "BUILD-implementation.md")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Should not error - not a git repo
	err = validateGitCommitStatus(tmpDir, "BUILD")
	if err != nil {
		t.Errorf("expected no error for non-git repo, got: %v", err)
	}
}

func TestValidateGitCommitStatus_PartiallyCommitted(t *testing.T) {
	// Test the exact scenario from Instance 2: some files committed, some not
	tmpDir, err := os.MkdirTemp("", "partial-commit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	exec.Command("git", "init", tmpDir).Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

	// Create and commit WAYFINDER-STATUS.md
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	os.WriteFile(statusPath, []byte("status"), 0644)
	exec.Command("git", "-C", tmpDir, "add", "WAYFINDER-STATUS.md").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "commit status").Run()

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

	// Validation should FAIL for RETRO
	err = validateGitCommitStatus(tmpDir, "RETRO")
	if err == nil {
		t.Fatalf("expected error for partially committed files, got nil")
	}
	if !strings.Contains(err.Error(), "RETRO-retrospective.md") {
		t.Errorf("error should mention RETRO-retrospective.md, got: %v", err)
	}
}

func TestGetUntrackedFilesInProjectDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "untracked-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	exec.Command("git", "init", tmpDir).Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

	// Create committed file
	committedPath := filepath.Join(tmpDir, "committed.txt")
	os.WriteFile(committedPath, []byte("committed"), 0644)
	exec.Command("git", "-C", tmpDir, "add", "committed.txt").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run()

	// Create untracked files
	untrackedPath := filepath.Join(tmpDir, "untracked.txt")
	os.WriteFile(untrackedPath, []byte("untracked"), 0644)

	// Create .wayfinder/ internal file (should be filtered)
	wayfinderDir := filepath.Join(tmpDir, ".wayfinder")
	os.Mkdir(wayfinderDir, 0755)
	wayfinderFile := filepath.Join(wayfinderDir, "session.json")
	os.WriteFile(wayfinderFile, []byte("internal"), 0644)

	// Get untracked files
	untracked, err := getUntrackedFilesInProjectDir(tmpDir)
	if err != nil {
		t.Fatalf("getUntrackedFilesInProjectDir failed: %v", err)
	}

	// Should only include untracked.txt, not .wayfinder/session.json
	if len(untracked) != 1 {
		t.Errorf("expected 1 untracked file, got %d: %v", len(untracked), untracked)
	}

	if len(untracked) > 0 && untracked[0] != "untracked.txt" {
		t.Errorf("expected untracked.txt, got: %s", untracked[0])
	}
}

func TestIsFileUntracked(t *testing.T) {
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
			got := isFileUntracked(tt.fileName, tt.untrackedFiles)
			if got != tt.want {
				t.Errorf("isFileUntracked() = %v, want %v", got, tt.want)
			}
		})
	}
}
