//go:build darwin

package apfs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	if p.Name() != "apfs-reflink" {
		t.Errorf("expected name 'apfs-reflink', got '%s'", p.Name())
	}
}

func TestProvider_Create(t *testing.T) {
	// Create temporary directories for test
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Create lower directory with test file
	if err := os.MkdirAll(lowerDir, 0755); err != nil {
		t.Fatalf("failed to create lower dir: %v", err)
	}
	testFile := filepath.Join(lowerDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create provider
	p := NewProvider()

	// Create sandbox request
	req := sandbox.SandboxRequest{
		SessionID:    "test-session",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"TEST_KEY": "test_value",
		},
		Timeout: 10 * time.Second,
	}

	// Create sandbox
	ctx := context.Background()
	sb, err := p.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Verify sandbox metadata
	if sb.ID != "test-session" {
		t.Errorf("expected ID 'test-session', got '%s'", sb.ID)
	}
	if sb.Type != "apfs-reflink" {
		t.Errorf("expected type 'apfs-reflink', got '%s'", sb.Type)
	}

	// Verify merged directory exists
	if _, err := os.Stat(sb.MergedPath); os.IsNotExist(err) {
		t.Errorf("merged directory does not exist: %s", sb.MergedPath)
	}

	// Verify merged is a symlink
	target, err := os.Readlink(sb.MergedPath)
	if err != nil {
		t.Errorf("merged path is not a symlink: %v", err)
	}
	if target != sb.UpperPath {
		t.Errorf("merged symlink points to wrong target: got %s, want %s", target, sb.UpperPath)
	}

	// Verify cloned file exists
	clonedFile := filepath.Join(sb.UpperPath, "repo0", "test.txt")
	if _, err := os.Stat(clonedFile); os.IsNotExist(err) {
		t.Errorf("cloned file does not exist: %s", clonedFile)
	}

	// Verify secrets file exists
	secretsFile := filepath.Join(sb.UpperPath, ".env")
	if _, err := os.Stat(secretsFile); os.IsNotExist(err) {
		t.Errorf("secrets file does not exist: %s", secretsFile)
	}

	// Verify secrets content
	content, err := os.ReadFile(secretsFile)
	if err != nil {
		t.Fatalf("failed to read secrets file: %v", err)
	}
	if !contains(string(content), "TEST_KEY=test_value") {
		t.Errorf("secrets file does not contain expected content: %s", content)
	}

	// Clean up
	if err := p.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("Destroy() failed: %v", err)
	}
}

func TestProvider_CreateMapsRequestedWorkingDirectoryIntoMatchingClone(t *testing.T) {
	root := t.TempDir()
	otherRepo := filepath.Join(root, "other")
	targetRepo := filepath.Join(root, "dear-agent")
	requestedDir := filepath.Join(targetRepo, ".agents", "skills")
	if err := os.MkdirAll(otherRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(requestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(requestedDir, "SKILL.md")
	if err := os.WriteFile(marker, []byte("sandbox discovery marker"), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := NewProvider()
	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "mapped-workdir",
		LowerDirs:    []string{otherRepo, targetRepo},
		WorkingDir:   requestedDir,
		WorkspaceDir: filepath.Join(root, "workspace"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		if destroyErr := provider.Destroy(context.Background(), sb.ID); destroyErr != nil {
			t.Errorf("Destroy() error = %v", destroyErr)
		}
	})

	wantWorkingDir := filepath.Join(sb.MergedPath, "repo1", ".agents", "skills")
	if sb.WorkingDir != wantWorkingDir {
		t.Fatalf("WorkingDir = %q, want %q", sb.WorkingDir, wantWorkingDir)
	}
	if _, err := os.Stat(filepath.Join(sb.WorkingDir, "SKILL.md")); err != nil {
		t.Fatalf("mapped repository instructions are not visible from WorkingDir: %v", err)
	}
}

func TestProvider_CreateDetachesLinkedWorktreeGitMetadata(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	linked := filepath.Join(root, "linked")
	runAPFSGit(t, root, "init", "-b", "main", primary)
	runAPFSGit(t, primary, "config", "user.name", "APFS test")
	runAPFSGit(t, primary, "config", "user.email", "apfs-test@example.invalid")
	tracked := filepath.Join(primary, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("host content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAPFSGit(t, primary, "add", "tracked.txt")
	runAPFSGit(t, primary, "commit", "-m", "initial")
	runAPFSGit(t, primary, "worktree", "add", "-b", "feature", linked)
	runAPFSGit(t, primary, "config", "extensions.worktreeConfig", "true")
	runAPFSGit(t, linked, "config", "--worktree", "sandbox.test-marker", "linked")
	hostHead := strings.TrimSpace(runAPFSGit(t, linked, "rev-parse", "HEAD"))

	provider := NewProvider()
	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "linked-worktree-metadata",
		LowerDirs:    []string{linked},
		WorkingDir:   linked,
		WorkspaceDir: filepath.Join(root, "workspace"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		if destroyErr := provider.Destroy(context.Background(), sb.ID); destroyErr != nil {
			t.Errorf("Destroy() error = %v", destroyErr)
		}
	})

	cloneRoot := filepath.Join(sb.MergedPath, "repo0")
	gitInfo, err := os.Stat(filepath.Join(cloneRoot, ".git"))
	if err != nil {
		t.Fatalf("stat detached .git: %v", err)
	}
	if !gitInfo.IsDir() {
		t.Fatalf("sandbox .git mode = %s, want independent directory", gitInfo.Mode())
	}
	commonDir := strings.TrimSpace(runAPFSGit(t, cloneRoot, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	commonInfo, err := os.Stat(commonDir)
	if err != nil {
		t.Fatalf("stat sandbox common Git directory: %v", err)
	}
	if !os.SameFile(commonInfo, gitInfo) {
		t.Fatalf("sandbox common Git directory = %q, want %q", commonDir, filepath.Join(cloneRoot, ".git"))
	}
	configuredWorktree := strings.TrimSpace(runAPFSGit(t, cloneRoot, "config", "--get", "core.worktree"))
	configuredInfo, err := os.Stat(configuredWorktree)
	if err != nil {
		t.Fatalf("stat configured sandbox worktree: %v", err)
	}
	cloneInfo, err := os.Stat(cloneRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(configuredInfo, cloneInfo) {
		t.Fatalf("sandbox core.worktree = %q, want %q", configuredWorktree, cloneRoot)
	}
	if got := strings.TrimSpace(runAPFSGit(t, cloneRoot, "config", "--worktree", "--get", "sandbox.test-marker")); got != "linked" {
		t.Fatalf("sandbox worktree-specific config marker = %q, want linked", got)
	}

	if err := os.WriteFile(filepath.Join(cloneRoot, "tracked.txt"), []byte("sandbox content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAPFSGit(t, cloneRoot, "add", "tracked.txt")
	runAPFSGit(t, cloneRoot, "commit", "-m", "sandbox-only")
	if got := strings.TrimSpace(runAPFSGit(t, linked, "rev-parse", "HEAD")); got != hostHead {
		t.Fatalf("host linked-worktree HEAD changed to %s, want %s", got, hostHead)
	}
	if got := strings.TrimSpace(runAPFSGit(t, linked, "status", "--porcelain")); got != "" {
		t.Fatalf("host linked-worktree index changed through sandbox Git metadata: %q", got)
	}
}

func TestProvider_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Create lower directory
	if err := os.MkdirAll(lowerDir, 0755); err != nil {
		t.Fatalf("failed to create lower dir: %v", err)
	}

	p := NewProvider()

	req := sandbox.SandboxRequest{
		SessionID:    "test-validate",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	ctx := context.Background()
	sb, err := p.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Validate should succeed
	if err := p.Validate(ctx, sb.ID); err != nil {
		t.Errorf("Validate() failed for existing sandbox: %v", err)
	}

	// Validate non-existent sandbox should fail
	err = p.Validate(ctx, "non-existent")
	if err == nil {
		t.Error("Validate() should fail for non-existent sandbox")
	}

	// Clean up
	if err := p.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("Destroy() failed: %v", err)
	}

	// Validate after destroy should fail
	err = p.Validate(ctx, sb.ID)
	if err == nil {
		t.Error("Validate() should fail after Destroy()")
	}
}

func TestProvider_Destroy_Idempotent(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	// Destroy non-existent sandbox should not error (idempotent)
	if err := p.Destroy(ctx, "non-existent"); err != nil {
		t.Errorf("Destroy() should be idempotent for non-existent sandbox: %v", err)
	}
}

func TestProvider_ValidateRequest(t *testing.T) {
	p := NewProvider()

	tests := []struct {
		name    string
		req     sandbox.SandboxRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/tmp"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: false,
		},
		{
			name: "missing session ID",
			req: sandbox.SandboxRequest{
				LowerDirs:    []string{"/tmp"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing lower dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing workspace dir",
			req: sandbox.SandboxRequest{
				SessionID: "test",
				LowerDirs: []string{"/tmp"},
			},
			wantErr: true,
		},
		{
			name: "non-existent lower dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/this/does/not/exist"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.validateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloneDirectory_APFS(t *testing.T) {
	// This test verifies that cloneDirectory successfully uses cp -c on APFS
	p := NewProvider()
	tmpDir := t.TempDir()

	// Create source directory with test files
	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	// Create test file in source
	testFile := filepath.Join(srcDir, "test.txt")
	testContent := []byte("test content for cloning")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Clone to destination
	dstDir := filepath.Join(tmpDir, "destination")
	if err := p.cloneDirectory(context.Background(), srcDir, dstDir); err != nil {
		t.Fatalf("cloneDirectory() failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		t.Errorf("destination directory does not exist: %s", dstDir)
	}

	// Verify cloned file exists and has correct content
	clonedFile := filepath.Join(dstDir, "test.txt")
	content, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}
	if string(content) != string(testContent) {
		t.Errorf("cloned file content mismatch: got %s, want %s", content, testContent)
	}
}

func TestCloneDirectory_NonAPFS(t *testing.T) {
	// This test verifies fallback to recursive copy on non-APFS filesystems
	// On macOS tmpDir is typically APFS, so this test mainly validates the fallback logic exists
	p := NewProvider()
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "source-fallback")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	// Create nested structure to test recursive copy
	nestedDir := filepath.Join(srcDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	nestedFile := filepath.Join(nestedDir, "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("nested content"), 0644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	// Clone (will use cp -c on APFS, but tests recursive copy logic)
	dstDir := filepath.Join(tmpDir, "destination-fallback")
	if err := p.cloneDirectory(context.Background(), srcDir, dstDir); err != nil {
		t.Fatalf("cloneDirectory() failed: %v", err)
	}

	// Verify nested structure was copied
	clonedNestedFile := filepath.Join(dstDir, "nested", "nested.txt")
	if _, err := os.Stat(clonedNestedFile); os.IsNotExist(err) {
		t.Errorf("nested file was not cloned: %s", clonedNestedFile)
	}
}

// TestCloneDirectory_RejectsNestedDst is the ce-fmxv defense-in-depth
// regression test: the leaked home-dir clones had dst
// (~/.agm/sandboxes/<id>/upper/repo0) nested inside src ($HOME) — an
// unbounded clone, since every byte cloneDirectory writes becomes new input
// for its own walk. cloneDirectory must refuse this shape outright rather
// than relying solely on resolveSandboxLowerDirs upstream.
func TestCloneDirectory_RejectsNestedDst(t *testing.T) {
	p := NewProvider()
	tmpDir := t.TempDir()

	src := tmpDir // dst will be created underneath src
	dst := filepath.Join(tmpDir, "sandboxes", "id", "upper", "repo0")

	err := p.cloneDirectory(context.Background(), src, dst)
	if err == nil {
		t.Fatal("cloneDirectory() = nil error, want a rejection (dst is nested inside src)")
	}
	if !strings.Contains(err.Error(), "nested inside the source") {
		t.Errorf("cloneDirectory() error = %v, want a nested-dst rejection message", err)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("cloneDirectory() should not have created dst %s after rejecting it", dst)
	}
}

// TestCloneDirectory_AllowsSiblingDst is the negative case for the nested-dst
// guard: a dst that lives alongside (not inside) src must still clone
// normally.
func TestCloneDirectory_AllowsSiblingDst(t *testing.T) {
	p := NewProvider()
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	dst := filepath.Join(tmpDir, "destination")

	if err := p.cloneDirectory(context.Background(), src, dst); err != nil {
		t.Fatalf("cloneDirectory() failed for sibling dst: %v", err)
	}
}

// TestCloneDirectory_ContextCanceled verifies a canceled/expired context
// aborts the clone quickly instead of letting `cp -c -R` run unbounded — the
// direct mechanism behind the 180s SIGKILL that used to leak partial
// home-dir clones with no cleanup.
func TestCloneDirectory_ContextCanceled(t *testing.T) {
	p := NewProvider()
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	dst := filepath.Join(tmpDir, "destination")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before the clone even starts

	err := p.cloneDirectory(ctx, src, dst)
	if err == nil {
		t.Fatal("cloneDirectory() = nil error, want an error for an already-canceled context")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("cloneDirectory() should clean up dst %s after a canceled clone", dst)
	}
}

func TestIsClonefileError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "cloning not supported error",
			err:      os.ErrInvalid,
			expected: false,
		},
		{
			name:     "operation not supported",
			err:      &os.PathError{Op: "cp", Path: "/test", Err: os.ErrInvalid},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isClonefileError(tt.err)
			if result != tt.expected {
				t.Errorf("isClonefileError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func runAPFSGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
