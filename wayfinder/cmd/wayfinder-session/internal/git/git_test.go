package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user (required for commits)
	configUser := exec.Command("git", "config", "user.name", "Test User")
	configUser.Dir = tmpDir
	if err := configUser.Run(); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}

	configEmail := exec.Command("git", "config", "user.email", "test@example.com")
	configEmail.Dir = tmpDir
	if err := configEmail.Run(); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}

	return tmpDir
}

func TestNew(t *testing.T) {
	g := New("/tmp/test")
	if g.projectDir != "/tmp/test" {
		t.Errorf("New() projectDir = %q, want %q", g.projectDir, "/tmp/test")
	}
}

func TestIsGitRepo(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() string
		expected bool
	}{
		{
			name: "valid git repo",
			setup: func() string {
				return setupGitRepo(t)
			},
			expected: true,
		},
		{
			name: "non-git directory",
			setup: func() string {
				return t.TempDir()
			},
			expected: false,
		},
		{
			name: "subdirectory within git repo",
			setup: func() string {
				// Create git repo
				repoDir := setupGitRepo(t)
				// Create subdirectory (wayfinder project location)
				subDir := filepath.Join(repoDir, "wf", "my-project")
				if err := os.MkdirAll(subDir, 0755); err != nil {
					t.Fatalf("failed to create subdir: %v", err)
				}
				return subDir
			},
			expected: true, // Should detect git repo from subdirectory!
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup()
			g := New(dir)

			if got := g.IsGitRepo(); got != tt.expected {
				t.Errorf("IsGitRepo() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCommitPhaseCompletion(t *testing.T) {
	// Setup git repo
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit (git requires at least one commit)
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Project\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	addCmd := exec.Command("git", "add", "README.md")
	addCmd.Dir = repoDir
	if err := addCmd.Run(); err != nil {
		t.Fatalf("git add README failed: %v", err)
	}
	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = repoDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Create wayfinder files
	statusPath := filepath.Join(repoDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to write STATUS: %v", err)
	}

	historyPath := filepath.Join(repoDir, "WAYFINDER-HISTORY.md")
	if err := os.WriteFile(historyPath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("failed to write HISTORY: %v", err)
	}

	// Commit phase completion
	err := g.CommitPhaseCompletion("D1", "success", "Completed discovery phase")
	if err != nil {
		t.Fatalf("CommitPhaseCompletion() error = %v", err)
	}

	// Verify commit was created
	logCmd := exec.Command("git", "log", "--format=%s", "-n", "1")
	logCmd.Dir = repoDir
	output, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	subject := strings.TrimSpace(string(output))
	expectedSubject := "wayfinder: complete D1 (success)"
	if subject != expectedSubject {
		t.Errorf("commit subject = %q, want %q", subject, expectedSubject)
	}

	// Verify commit message body
	msgCmd := exec.Command("git", "log", "--format=%B", "-n", "1")
	msgCmd.Dir = repoDir
	msgOutput, err := msgCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	commitMsg := string(msgOutput)
	if !strings.Contains(commitMsg, "Completed discovery phase") {
		t.Errorf("commit message missing context: %q", commitMsg)
	}
	if !strings.Contains(commitMsg, "Wayfinder-Phase: D1") {
		t.Errorf("commit message missing phase metadata: %q", commitMsg)
	}
	if !strings.Contains(commitMsg, "Wayfinder-Outcome: success") {
		t.Errorf("commit message missing outcome metadata: %q", commitMsg)
	}
}

func TestCommitPhaseCompletion_NonGitRepo(t *testing.T) {
	// Non-git directory
	tmpDir := t.TempDir()
	g := New(tmpDir)

	err := g.CommitPhaseCompletion("D1", "success", "")
	if err == nil {
		t.Error("CommitPhaseCompletion() on non-git repo should return error")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error should mention git repository, got: %v", err)
	}
}

func TestCommitPhaseCompletion_NothingToCommit(t *testing.T) {
	// Setup git repo
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	addCmd := exec.Command("git", "add", "README.md")
	addCmd.Dir = repoDir
	addCmd.Run()
	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = repoDir
	commitCmd.Run()

	// Create and commit wayfinder files
	statusPath := filepath.Join(repoDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to write STATUS: %v", err)
	}
	addCmd2 := exec.Command("git", "add", "WAYFINDER-STATUS.md")
	addCmd2.Dir = repoDir
	addCmd2.Run()
	commitCmd2 := exec.Command("git", "commit", "-m", "Add wayfinder files")
	commitCmd2.Dir = repoDir
	commitCmd2.Run()

	// Try to commit again without changes (should not error)
	err := g.CommitPhaseCompletion("D1", "success", "")
	if err != nil {
		t.Errorf("CommitPhaseCompletion() with nothing to commit should not error, got: %v", err)
	}
}

func TestFormatCommitMessage(t *testing.T) {
	g := New("/tmp/test")

	tests := []struct {
		name     string
		phase    string
		outcome  string
		context  string
		contains []string
	}{
		{
			name:    "with context",
			phase:   "D1",
			outcome: "success",
			context: "Completed user interviews",
			contains: []string{
				"wayfinder: complete D1 (success)",
				"Completed user interviews",
				"Wayfinder-Phase: D1",
				"Wayfinder-Outcome: success",
			},
		},
		{
			name:    "without context",
			phase:   "S5",
			outcome: "partial",
			context: "",
			contains: []string{
				"wayfinder: complete S5 (partial)",
				"Wayfinder-Phase: S5",
				"Wayfinder-Outcome: partial",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := g.formatCommitMessage(tt.phase, tt.outcome, tt.context)

			for _, expected := range tt.contains {
				if !strings.Contains(msg, expected) {
					t.Errorf("formatCommitMessage() missing %q in:\n%s", expected, msg)
				}
			}
		})
	}
}

func TestGetCommitHash(t *testing.T) {
	// Setup git repo with a commit
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	addCmd := exec.Command("git", "add", "README.md")
	addCmd.Dir = repoDir
	addCmd.Run()
	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = repoDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Get commit hash
	hash, err := g.GetCommitHash()
	if err != nil {
		t.Fatalf("GetCommitHash() error = %v", err)
	}

	// Verify hash format (40 hex characters)
	if len(hash) != 40 {
		t.Errorf("GetCommitHash() returned hash length = %d, want 40", len(hash))
	}

	// Verify hash is valid hex
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("GetCommitHash() returned invalid hex character: %c", c)
		}
	}
}

func TestIsSourceCodeFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		// Source code files (should return true)
		{"Go file", "main.go", true},
		{"Python file", "script.py", true},
		{"JavaScript file", "app.js", true},
		{"TypeScript file", "component.ts", true},
		{"JSX file", "App.jsx", true},
		{"TSX file", "Component.tsx", true},
		{"C file", "main.c", true},
		{"C++ file", "main.cpp", true},
		{"Java file", "Main.java", true},
		{"Ruby file", "script.rb", true},
		{"Rust file", "main.rs", true},
		{"PHP file", "index.php", true},

		// Non-code files (should return false)
		{"Markdown file", "README.md", false},
		{"YAML file", "config.yaml", false},
		{"YAML file (.yml)", "config.yml", false},
		{"JSON file", "package.json", false},
		{"Text file", "notes.txt", false},
		{"Shell script", "setup.sh", false},
		{"Bash script", "install.bash", false},
		{"No extension", "Makefile", false},
		{"Hidden file", ".gitignore", false},

		// Path with directory (should check extension only)
		{"Go file in subdirectory", "internal/validator/validator.go", true},
		{"MD file in subdirectory", "docs/guide.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSourceCodeFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isSourceCodeFile(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestIsInProjectDir(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		projectDir string
		want       bool
	}{
		{
			name:       "File directly in project dir",
			filePath:   "/tmp/test/src/wf/phase-boundary-self-check/research.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       true,
		},
		{
			name:       "File in subdirectory of project dir",
			filePath:   "/tmp/test/src/wf/phase-boundary-self-check/subdir/test.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       true,
		},
		{
			name:       "File outside project dir (sibling)",
			filePath:   "/tmp/test/src/wf/other-project/file.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       false,
		},
		{
			name:       "File outside project dir (parent)",
			filePath:   "/tmp/test/src/engram/main.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       false,
		},
		{
			name:       "File in similarly named dir (false match test)",
			filePath:   "/tmp/test/src/wf/phase-boundary-self-check-2/file.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       false,
		},
		{
			name:       "Relative path in project dir",
			filePath:   "research.go",
			projectDir: ".",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInProjectDir(tt.filePath, tt.projectDir)
			if got != tt.want {
				t.Errorf("isInProjectDir(%q, %q) = %v, want %v",
					tt.filePath, tt.projectDir, got, tt.want)
			}
		})
	}
}

func TestGetUncommittedFilesInProjectDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantFiles []string
		wantErr   bool
	}{
		{
			name: "No git repo (graceful handling)",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Clean project directory",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()
				return repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Uncommitted deliverable files in project directory",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Create uncommitted deliverable files
				os.WriteFile(filepath.Join(repoDir, "W0-charter.md"), []byte("# Charter\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "D1-problem.md"), []byte("# Problem\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Status\n"), 0644)

				return repoDir
			},
			wantFiles: []string{"D1-problem.md", "W0-charter.md", "WAYFINDER-STATUS.md"},
			wantErr:   false,
		},
		{
			name: "Uncommitted files with .wayfinder/ directory (should ignore .wayfinder/)",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Create .wayfinder directory with files (should be ignored)
				wayfinderDir := filepath.Join(repoDir, ".wayfinder")
				os.MkdirAll(wayfinderDir, 0755)
				os.WriteFile(filepath.Join(wayfinderDir, "archive.json"), []byte("{}"), 0644)

				// Create uncommitted deliverable
				os.WriteFile(filepath.Join(repoDir, "S8-implementation.md"), []byte("# Implementation\n"), 0644)

				return repoDir
			},
			wantFiles: []string{"S8-implementation.md"},
			wantErr:   false,
		},
		{
			name: "Only .wayfinder/ files uncommitted (should return empty)",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Create .wayfinder directory with files (should all be ignored)
				wayfinderDir := filepath.Join(repoDir, ".wayfinder")
				os.MkdirAll(wayfinderDir, 0755)
				os.WriteFile(filepath.Join(wayfinderDir, "metadata.json"), []byte("{}"), 0644)

				return repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Modified and untracked files",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Create and commit initial files
				os.WriteFile(filepath.Join(repoDir, "W0-charter.md"), []byte("# Charter v1\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Modify committed file
				os.WriteFile(filepath.Join(repoDir, "W0-charter.md"), []byte("# Charter v2\n"), 0644)

				// Add untracked file
				os.WriteFile(filepath.Join(repoDir, "D1-problem.md"), []byte("# Problem\n"), 0644)

				return repoDir
			},
			wantFiles: []string{"D1-problem.md", "W0-charter.md"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := tt.setup(t)
			g := New(projectDir)

			gotFiles, err := g.GetUncommittedFilesInProjectDir()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUncommittedFilesInProjectDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(gotFiles) != len(tt.wantFiles) {
					t.Errorf("GetUncommittedFilesInProjectDir() returned %d files, want %d\nGot: %v\nWant: %v",
						len(gotFiles), len(tt.wantFiles), gotFiles, tt.wantFiles)
				}

				// Check each expected file is present
				for _, want := range tt.wantFiles {
					found := false
					for _, got := range gotFiles {
						if got == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("GetUncommittedFilesInProjectDir() missing expected file %q\nGot: %v\nWant: %v",
							want, gotFiles, tt.wantFiles)
					}
				}
			}
		})
	}
}

func TestGetModifiedSourceFiles(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) (repoDir, projectDir string)
		wantFiles  []string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "No git repo (graceful handling)",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				return tmpDir, tmpDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "No modified files",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()
				return repoDir, repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Modified source file outside project dir",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Create project dir subdirectory
				projectDir := filepath.Join(repoDir, "wf", "my-project")
				os.MkdirAll(projectDir, 0755)

				// Modify .go file outside project dir
				goFile := filepath.Join(repoDir, "main.go")
				os.WriteFile(goFile, []byte("package main\n"), 0644)

				return repoDir, projectDir
			},
			wantFiles: []string{"main.go"},
			wantErr:   false,
		},
		{
			name: "Modified source file inside project dir (should be ignored)",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Create project dir
				projectDir := filepath.Join(repoDir, "wf", "my-project")
				os.MkdirAll(projectDir, 0755)

				// Modify .go file inside project dir (should be ignored)
				goFile := filepath.Join(projectDir, "research.go")
				os.WriteFile(goFile, []byte("package main\n"), 0644)

				return repoDir, projectDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Modified non-code file (should be ignored)",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Modify markdown file (not source code)
				mdFile := filepath.Join(repoDir, "PLAN.md")
				os.WriteFile(mdFile, []byte("# Plan\n"), 0644)

				return repoDir, repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Multiple source files modified",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				exec.Command("git", "-C", repoDir, "add", ".").Run()
				exec.Command("git", "-C", repoDir, "commit", "-m", "Initial").Run()

				// Modify multiple source files
				os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "script.py"), []byte("print('hello')\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "app.js"), []byte("console.log('hi')\n"), 0644)

				return repoDir, repoDir
			},
			wantFiles: []string{"app.js", "main.go", "script.py"}, // Alphabetical order from git status
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir, projectDir := tt.setup(t)
			g := New(projectDir)

			gotFiles, err := g.GetModifiedSourceFiles(projectDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetModifiedSourceFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("GetModifiedSourceFiles() error = %v, want error containing %q", err, tt.wantErrMsg)
			}

			if !tt.wantErr {
				// Sort both slices for comparison (git status order may vary)
				gotStr := strings.Join(gotFiles, ",")
				wantStr := strings.Join(tt.wantFiles, ",")

				if len(gotFiles) != len(tt.wantFiles) {
					t.Errorf("GetModifiedSourceFiles() returned %d files, want %d\nGot: %v\nWant: %v",
						len(gotFiles), len(tt.wantFiles), gotFiles, tt.wantFiles)
				}

				// Check each expected file is present
				for _, want := range tt.wantFiles {
					found := false
					for _, got := range gotFiles {
						if got == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("GetModifiedSourceFiles() missing expected file %q\nGot: %v\nWant: %v",
							want, gotStr, wantStr)
					}
				}
			}

			// Cleanup
			_ = repoDir
		})
	}
}
