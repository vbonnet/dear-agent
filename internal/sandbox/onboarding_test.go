package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateOnboardingContent(t *testing.T) {
	content, err := GenerateOnboardingContent(
		"test-session",
		"~/.agm/sandboxes/abc-123/merged",
		[]string{"~/src/ws/oss/repos/ai-tools", "~/src/ws/oss/repos/engram"},
	)
	if err != nil {
		t.Fatalf("GenerateOnboardingContent failed: %v", err)
	}

	// Verify key content is present
	checks := []string{
		"READ-ONLY",
		"git worktree add",
		"test-session",
		"~/src/ws/oss/repos/ai-tools",
		"~/src/ws/oss/repos/engram",
		"~/src/ws/oss/worktrees/",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("expected content to contain %q, got:\n%s", check, content)
		}
	}
}

func TestGenerateOnboardingContent_EmptyRepos(t *testing.T) {
	content, err := GenerateOnboardingContent("my-session", "/tmp/merged", []string{})
	if err != nil {
		t.Fatalf("GenerateOnboardingContent failed: %v", err)
	}
	if !strings.Contains(content, "my-session") {
		t.Error("expected session name in content")
	}
}

func TestClaudeProjectDir(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		workDir string
		want    string
	}{
		{
			name:    "simple path",
			workDir: "/home/user/project",
			want:    filepath.Join(homeDir, ".claude", "projects", "-home-user-project"),
		},
		{
			name:    "sandbox merged path",
			workDir: "/home/user/.agm/sandboxes/abc-123/merged",
			want:    filepath.Join(homeDir, ".claude", "projects", "-home-user-.agm-sandboxes-abc-123-merged"),
		},
		{
			name:    "deep nested path",
			workDir: "/home/user/src/ws/oss/repos/ai-tools",
			want:    filepath.Join(homeDir, ".claude", "projects", "-home-user-src-ws-oss-repos-ai-tools"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClaudeProjectDir(tt.workDir)
			if err != nil {
				t.Fatalf("ClaudeProjectDir(%q) error: %v", tt.workDir, err)
			}
			if got != tt.want {
				t.Errorf("ClaudeProjectDir(%q) = %q, want %q", tt.workDir, got, tt.want)
			}
		})
	}
}

func TestWriteOnboardingClaudeMd_WritesToProjectDir(t *testing.T) {
	// Set up a fake home directory so we control where ~/.claude/projects/ goes
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Use a fake merged path (simulating sandbox overlay)
	mergedDir := t.TempDir()
	content := "# Test CLAUDE.md\nThis is onboarding content."

	err := WriteOnboardingClaudeMd(mergedDir, content)
	if err != nil {
		t.Fatalf("WriteOnboardingClaudeMd failed: %v", err)
	}

	// Verify content was written to ~/.claude/projects/<encoded>/CLAUDE.md
	projectDir, err := ClaudeProjectDir(mergedDir)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(written) != content {
		t.Errorf("expected %q, got %q", content, string(written))
	}

	// Verify the repo's CLAUDE.md was NOT created/modified
	repoClaude := filepath.Join(mergedDir, "CLAUDE.md")
	if _, err := os.Stat(repoClaude); !os.IsNotExist(err) {
		t.Errorf("expected repo CLAUDE.md to NOT exist, but it does at %s", repoClaude)
	}
}

func TestWriteOnboardingClaudeMd_PreservesExistingProjectClaudeMd(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	mergedDir := t.TempDir()

	// Pre-create a project-level CLAUDE.md (simulating existing user instructions)
	projectDir, err := ClaudeProjectDir(mergedDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := "# Existing User Instructions\nDo not delete me."
	if err := os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	onboarding := "# Sandbox Environment\nNew onboarding content."
	err = WriteOnboardingClaudeMd(mergedDir, onboarding)
	if err != nil {
		t.Fatalf("WriteOnboardingClaudeMd failed: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	result := string(written)

	if !strings.Contains(result, "Sandbox Environment") {
		t.Error("expected onboarding content in result")
	}
	if !strings.Contains(result, "Existing User Instructions") {
		t.Error("expected existing content preserved in result")
	}
	// Onboarding should come first
	onboardIdx := strings.Index(result, "Sandbox Environment")
	origIdx := strings.Index(result, "Existing User Instructions")
	if onboardIdx > origIdx {
		t.Error("expected onboarding content before existing content")
	}
}

func TestWriteOnboardingClaudeMd_DoesNotModifyRepoCLAUDEMd(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	mergedDir := t.TempDir()

	// Pre-create a repo-level CLAUDE.md (simulating overlay lower dir)
	repoClaude := filepath.Join(mergedDir, "CLAUDE.md")
	original := "# Original Repo Instructions\nTracked by git."
	if err := os.WriteFile(repoClaude, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	onboarding := "# Sandbox Environment\nOnboarding content."
	err := WriteOnboardingClaudeMd(mergedDir, onboarding)
	if err != nil {
		t.Fatalf("WriteOnboardingClaudeMd failed: %v", err)
	}

	// Verify repo CLAUDE.md was NOT modified
	after, err := os.ReadFile(repoClaude)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Errorf("repo CLAUDE.md was modified: expected %q, got %q", original, string(after))
	}
}

func TestGenerateOnboardingContentFromFile(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "custom.md")
	tmplContent := "# Custom\nSession: {{.SessionName}}\nRepos: {{range .Repos}}{{.}} {{end}}"
	if err := os.WriteFile(tmplPath, []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	content, err := GenerateOnboardingContentFromFile(
		tmplPath, "custom-session", "/tmp/merged",
		[]string{"~/src/repo1"},
	)
	if err != nil {
		t.Fatalf("GenerateOnboardingContentFromFile failed: %v", err)
	}
	if !strings.Contains(content, "custom-session") {
		t.Error("expected session name in custom template output")
	}
}
