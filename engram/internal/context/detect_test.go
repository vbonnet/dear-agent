package context

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestDetectContext_GitRepo verifies git repo name used when available
func TestDetectContext_GitRepo(t *testing.T) {
	// Create a temp directory with git repo
	tmpDir := testutil.SetupTempDir(t)

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Skip("git not available, skipping test")
	}

	// Set git remote with SSH URL
	cmd = exec.Command("git", "remote", "add", "origin", "git@github.com:user/my-awesome-project.git")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add git remote: %v", err)
	}

	// Test detection
	got, err := DetectContext()
	if err != nil {
		t.Fatalf("DetectContext() error = %v", err)
	}

	want := "my awesome project patterns"
	if got != want {
		t.Errorf("DetectContext() = %q, want %q", got, want)
	}
}

// TestDetectContext_GitHTTPS verifies git HTTPS URL parsing
func TestDetectContext_GitHTTPS(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Skip("git not available, skipping test")
	}

	cmd = exec.Command("git", "remote", "add", "origin", "https://github.com/user/another-repo.git")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add git remote: %v", err)
	}

	got, err := DetectContext()
	if err != nil {
		t.Fatalf("DetectContext() error = %v", err)
	}

	want := "another repo patterns"
	if got != want {
		t.Errorf("DetectContext() = %q, want %q", got, want)
	}
}

// TestDetectContext_DirectoryFallback verifies directory name fallback
func TestDetectContext_DirectoryFallback(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	// Create a subdirectory with a specific name
	testDir := filepath.Join(tmpDir, "test-project-name")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(testDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// No git repo, should use directory name
	got, err := DetectContext()
	if err != nil {
		t.Fatalf("DetectContext() error = %v", err)
	}

	want := "test project name patterns"
	if got != want {
		t.Errorf("DetectContext() = %q, want %q", got, want)
	}
}

// TestDetectContext_FinalFallback verifies final fallback when no context available
func TestDetectContext_FinalFallback(t *testing.T) {
	// This is hard to test since we'd need to be in "/" directory
	// Instead, we verify that DetectContext always returns non-empty result
	got, err := DetectContext()
	if err != nil {
		t.Fatalf("DetectContext() error = %v", err)
	}

	if got == "" {
		t.Error("DetectContext() returned empty string, want non-empty")
	}
}

// TestCleanName verifies name sanitization
func TestCleanName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name with dashes",
			input: "simple-name",
			want:  "simple name",
		},
		{
			name:  "name with underscores",
			input: "name_with_underscores",
			want:  "name with underscores",
		},
		{
			name:  "name with multiple dashes",
			input: "name-with-many---dashes",
			want:  "name with many dashes",
		},
		{
			name:  "mixed separators",
			input: "mixed_name-with-separators",
			want:  "mixed name with separators",
		},
		{
			name:  "CamelCase name",
			input: "CamelCaseName",
			want:  "camelcasename",
		},
		{
			name:  "name with special characters",
			input: "name@with#special$chars",
			want:  "namewithspecialchars",
		},
		{
			name:  "name with spaces already",
			input: "name with spaces",
			want:  "name with spaces",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "name with numbers",
			input: "project-123-test",
			want:  "project 123 test",
		},
		{
			name:  "name with multiple spaces",
			input: "name    with    spaces",
			want:  "name with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanName(tt.input)
			if got != tt.want {
				t.Errorf("cleanName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestGetGitRepoName_NoGitRepo verifies behavior when not in a git repo
func TestGetGitRepoName_NoGitRepo(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Should return error or empty string when not in a git repo
	name, err := getGitRepoName()
	if err == nil && name != "" {
		t.Errorf("getGitRepoName() = %q, want empty or error in non-git directory", name)
	}
}

// TestGetGitRepoName_WithGitSuffix verifies .git suffix is removed
func TestGetGitRepoName_WithGitSuffix(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Skip("git not available, skipping test")
	}

	cmd = exec.Command("git", "remote", "add", "origin", "https://github.com/user/repo.git")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add git remote: %v", err)
	}

	name, err := getGitRepoName()
	if err != nil {
		t.Fatalf("getGitRepoName() error = %v", err)
	}

	if name != "repo" {
		t.Errorf("getGitRepoName() = %q, want %q", name, "repo")
	}
}
