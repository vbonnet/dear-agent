package vcs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRepo_InitializesNewRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "memories")

	repo, err := EnsureRepo(repoDir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	if !repo.IsRepo() {
		t.Error("expected directory to be a git repo")
	}

	// Verify .gitignore exists
	gitignore := filepath.Join(repoDir, ".gitignore")
	if _, err := os.Stat(gitignore); err != nil {
		t.Errorf("expected .gitignore to exist: %v", err)
	}

	// Verify initial commit
	entries, err := repo.Log("", 1)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(entries))
	}
	if entries[0].Message != "vcs: initialize memory repository" {
		t.Errorf("unexpected commit message: %s", entries[0].Message)
	}
}

func TestEnsureRepo_Idempotent(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "memories")

	repo1, err := EnsureRepo(repoDir, "", "", "main")
	if err != nil {
		t.Fatalf("first EnsureRepo: %v", err)
	}

	hash1, _ := repo1.HeadHash()

	repo2, err := EnsureRepo(repoDir, "", "", "main")
	if err != nil {
		t.Fatalf("second EnsureRepo: %v", err)
	}

	hash2, _ := repo2.HeadHash()

	if hash1 != hash2 {
		t.Errorf("expected same hash, got %s vs %s", hash1, hash2)
	}
}

func TestEnsureRepo_WithRemote(t *testing.T) {
	// Create a bare repo to act as remote
	bareDir := t.TempDir()
	bareRepo := &Repo{dir: bareDir}
	if err := bareRepo.run("git", "init", "--bare"); err != nil {
		t.Fatalf("init bare repo: %v", err)
	}

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "memories")

	_, err := EnsureRepo(repoDir, "origin", bareDir, "main")
	if err != nil {
		t.Fatalf("EnsureRepo with remote: %v", err)
	}
}

func TestTrackChange_CommitsFile(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// Create a test .ai.md file
	testFile := filepath.Join(dir, "test-memory.ai.md")
	if err := os.WriteFile(testFile, []byte("---\ntype: pattern\n---\n# Test"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	hash, err := repo.TrackChange("test-memory.ai.md", "memory: add test memory")
	if err != nil {
		t.Fatalf("TrackChange: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty commit hash")
	}

	// Verify commit in log
	entries, err := repo.Log("test-memory.ai.md", 1)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 commit for file, got %d", len(entries))
	}
	if entries[0].Message != "memory: add test memory" {
		t.Errorf("unexpected message: %s", entries[0].Message)
	}
}

func TestTrackChange_IncludesCompanion(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// Create .ai.md and .why.md pair
	aiFile := filepath.Join(dir, "test.ai.md")
	whyFile := filepath.Join(dir, "test.why.md")
	if err := os.WriteFile(aiFile, []byte("---\ntype: pattern\n---\n# Test"), 0644); err != nil {
		t.Fatalf("write .ai.md: %v", err)
	}
	if err := os.WriteFile(whyFile, []byte("---\nrationale_for: test\n---\n# Why"), 0644); err != nil {
		t.Fatalf("write .why.md: %v", err)
	}

	_, err = repo.TrackChange("test.ai.md", "memory: add test pair")
	if err != nil {
		t.Fatalf("TrackChange: %v", err)
	}

	// Both files should be committed — verify by checking git status is clean
	status, err := repo.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if strings.TrimSpace(status) != "" {
		t.Errorf("expected clean status, got: %s", status)
	}
}

func TestTrackChange_DefaultMessage(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	testFile := filepath.Join(dir, "my-memory.ai.md")
	if err := os.WriteFile(testFile, []byte("---\ntype: pattern\n---\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = repo.TrackChange("my-memory.ai.md", "")
	if err != nil {
		t.Fatalf("TrackChange: %v", err)
	}

	entries, err := repo.Log("", 1)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if entries[0].Message != "memory: update my-memory.ai.md" {
		t.Errorf("unexpected default message: %s", entries[0].Message)
	}
}

func TestTrackDelete_CommitsDeletion(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// Create and commit a file first
	testFile := filepath.Join(dir, "to-delete.ai.md")
	if err := os.WriteFile(testFile, []byte("---\ntype: pattern\n---\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := repo.TrackChange("to-delete.ai.md", "add file"); err != nil {
		t.Fatalf("TrackChange: %v", err)
	}

	// Delete the file
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("remove: %v", err)
	}

	hash, err := repo.TrackDelete("to-delete.ai.md", "memory: remove old memory")
	if err != nil {
		t.Fatalf("TrackDelete: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty commit hash")
	}
}

func TestRestore_RevertsFile(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	testFile := filepath.Join(dir, "versioned.ai.md")

	// Version 1
	if err := os.WriteFile(testFile, []byte("version 1"), 0644); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	hash1, err := repo.TrackChange("versioned.ai.md", "v1")
	if err != nil {
		t.Fatalf("TrackChange v1: %v", err)
	}

	// Version 2
	if err := os.WriteFile(testFile, []byte("version 2"), 0644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	if _, err := repo.TrackChange("versioned.ai.md", "v2"); err != nil {
		t.Fatalf("TrackChange v2: %v", err)
	}

	// Restore to v1
	if err := repo.Restore("versioned.ai.md", hash1); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read after restore: %v", err)
	}
	if string(data) != "version 1" {
		t.Errorf("expected 'version 1', got %q", string(data))
	}
}

func TestLog_ReturnsHistory(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	testFile := filepath.Join(dir, "logged.ai.md")

	// Create 3 versions
	for i, msg := range []string{"first", "second", "third"} {
		content := []byte("version " + msg)
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		if _, err := repo.TrackChange("logged.ai.md", "memory: "+msg); err != nil {
			t.Fatalf("TrackChange %d: %v", i, err)
		}
	}

	entries, err := repo.Log("logged.ai.md", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Most recent first
	if entries[0].Message != "memory: third" {
		t.Errorf("expected 'memory: third', got %q", entries[0].Message)
	}
}

func TestLog_WithLimit(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	testFile := filepath.Join(dir, "limited.ai.md")
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(testFile, []byte("v"+string(rune('0'+i))), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := repo.TrackChange("limited.ai.md", "commit"); err != nil {
			t.Fatalf("TrackChange: %v", err)
		}
	}

	entries, err := repo.Log("limited.ai.md", 2)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(entries))
	}
}

func TestCompanionPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"test.ai.md", "test.why.md"},
		{"test.why.md", "test.ai.md"},
		{"path/to/memory.ai.md", "path/to/memory.why.md"},
		{"not-an-engram.md", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := companionPath(tt.input)
		if got != tt.want {
			t.Errorf("companionPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsTrackedFileType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"test.ai.md", true},
		{"test.why.md", true},
		{"test.md", false},
		{"test.go", false},
	}

	for _, tt := range tests {
		got := isTrackedFileType(tt.input)
		if got != tt.want {
			t.Errorf("isTrackedFileType(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestValidateMemoryPair(t *testing.T) {
	dir := t.TempDir()

	// Create .ai.md without .why.md
	aiFile := filepath.Join(dir, "orphan.ai.md")
	if err := os.WriteFile(aiFile, []byte("---\ntype: pattern\n---\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	errors := ValidateMemoryPair(dir, []string{"orphan.ai.md"})
	if len(errors) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(errors))
	}
	if !strings.Contains(errors[0].Message, "missing companion .why.md") {
		t.Errorf("unexpected error: %s", errors[0].Message)
	}

	// Create matching .why.md
	whyFile := filepath.Join(dir, "orphan.why.md")
	if err := os.WriteFile(whyFile, []byte("---\nrationale_for: orphan\n---\n"), 0644); err != nil {
		t.Fatalf("write .why.md: %v", err)
	}

	errors = ValidateMemoryPair(dir, []string{"orphan.ai.md"})
	if len(errors) != 0 {
		t.Errorf("expected no errors after creating .why.md, got %d", len(errors))
	}
}

func TestValidateEngramFrontmatter(t *testing.T) {
	dir := t.TempDir()

	// File without frontmatter
	bad := filepath.Join(dir, "bad.ai.md")
	if err := os.WriteFile(bad, []byte("# No frontmatter"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	errors := ValidateEngramFrontmatter(dir, []string{"bad.ai.md"})
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	// File with valid frontmatter
	good := filepath.Join(dir, "good.ai.md")
	if err := os.WriteFile(good, []byte("---\ntype: pattern\n---\n# Content"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	errors = ValidateEngramFrontmatter(dir, []string{"good.ai.md"})
	if len(errors) != 0 {
		t.Errorf("expected no errors for valid file, got %d", len(errors))
	}
}

func TestInstallPreCommitHook(t *testing.T) {
	dir := t.TempDir()
	repo, err := EnsureRepo(dir, "", "", "main")
	if err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}
	_ = repo

	if err := InstallPreCommitHook(dir); err != nil {
		t.Fatalf("InstallPreCommitHook: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook file missing: %v", err)
	}

	// Check executable permission
	if info.Mode()&0111 == 0 {
		t.Error("hook should be executable")
	}
}

func TestParsePushStrategy(t *testing.T) {
	tests := []struct {
		input string
		want  PushStrategy
	}{
		{"immediate", PushImmediate},
		{"async", PushAsync},
		{"batched", PushBatched},
		{"manual", PushManual},
		{"unknown", PushAsync}, // default
		{"", PushAsync},        // default
	}

	for _, tt := range tests {
		got := ParsePushStrategy(tt.input)
		if got != tt.want {
			t.Errorf("ParsePushStrategy(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMemoryVCS_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	m, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Operations should be no-ops
	hash, err := m.TrackChange("anything", "msg")
	if err != nil {
		t.Errorf("TrackChange should not error when disabled: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty hash when disabled, got %s", hash)
	}

	status, err := m.Status()
	if err != nil {
		t.Errorf("Status: %v", err)
	}
	if status != "VCS disabled" {
		t.Errorf("unexpected status: %s", status)
	}
}

func TestMemoryVCS_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Enabled:      true,
		RepoPath:     dir,
		PushStrategy: "manual", // no remote, manual push
		Branch:       "main",
		Validation: ValidationConfig{
			RequireWhyFile: true,
		},
	}

	m, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	// Create and track a memory pair
	testFile := filepath.Join(dir, "e2e-test.ai.md")
	whyFile := filepath.Join(dir, "e2e-test.why.md")
	if err := os.WriteFile(testFile, []byte("---\ntype: pattern\n---\n# E2E"), 0644); err != nil {
		t.Fatalf("write .ai.md: %v", err)
	}
	if err := os.WriteFile(whyFile, []byte("---\nrationale_for: e2e-test\n---\n# Why"), 0644); err != nil {
		t.Fatalf("write .why.md: %v", err)
	}

	hash, err := m.TrackChange(testFile, "memory: e2e test")
	if err != nil {
		t.Fatalf("TrackChange: %v", err)
	}
	if hash == "" {
		t.Error("expected commit hash")
	}

	// Check log
	entries, err := m.Log("e2e-test.ai.md", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Modify and track again (companion already exists)
	if err := os.WriteFile(testFile, []byte("---\ntype: pattern\n---\n# E2E v2"), 0644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	hash2, err := m.TrackChange(testFile, "memory: update e2e")
	if err != nil {
		t.Fatalf("TrackChange v2: %v", err)
	}
	if hash2 == hash {
		t.Error("expected different hash for new commit")
	}

	// Restore to first version
	if err := m.Restore("e2e-test.ai.md", hash); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read after restore: %v", err)
	}
	if !strings.Contains(string(data), "# E2E") || strings.Contains(string(data), "v2") {
		t.Errorf("restore failed, got: %s", string(data))
	}
}

func TestMemoryVCS_Push_ToLocalBare(t *testing.T) {
	// Create a bare repo as "remote"
	bareDir := t.TempDir()
	bareRepo := &Repo{dir: bareDir}
	if err := bareRepo.run("git", "init", "--bare"); err != nil {
		t.Fatalf("init bare: %v", err)
	}

	dir := t.TempDir()
	cfg := &Config{
		Enabled:      true,
		RepoPath:     dir,
		PushStrategy: "manual",
		RemoteURL:    bareDir,
		RemoteName:   "origin",
		Branch:       "main",
	}

	m, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	// Create a file and commit
	testFile := filepath.Join(dir, "push-test.ai.md")
	if err := os.WriteFile(testFile, []byte("---\ntype: pattern\n---\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := m.TrackChange(testFile, "add for push test"); err != nil {
		t.Fatalf("TrackChange: %v", err)
	}

	// Push
	if err := m.Push(); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify remote received the commit by cloning
	cloneDir := t.TempDir()
	cloneRepo := &Repo{dir: cloneDir}
	if err := cloneRepo.run("git", "clone", bareDir, cloneDir); err != nil {
		t.Fatalf("clone: %v", err)
	}

	clonedFile := filepath.Join(cloneDir, "push-test.ai.md")
	if _, err := os.Stat(clonedFile); err != nil {
		t.Errorf("expected file in clone: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("expected enabled by default")
	}
	if cfg.PushStrategy != "async" {
		t.Errorf("expected async push, got %s", cfg.PushStrategy)
	}
	if cfg.Branch != "main" {
		t.Errorf("expected main branch, got %s", cfg.Branch)
	}
	if !cfg.Validation.RequireWhyFile {
		t.Error("expected RequireWhyFile true by default")
	}
}
