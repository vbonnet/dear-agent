//go:build linux

package gvisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	if got := p.Name(); got != "gvisor" {
		t.Errorf("Name() = %q, want %q", got, "gvisor")
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

func TestProvider_CheckRunscInstalled_Missing(t *testing.T) {
	if _, err := exec.LookPath("runsc"); err == nil {
		t.Skip("runsc is installed; cannot test the missing-binary path")
	}
	p := NewProvider()
	err := p.checkRunscInstalled()
	if err == nil {
		t.Fatal("checkRunscInstalled() should fail when runsc is not on PATH")
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) {
		t.Fatalf("expected *sandbox.Error, got %T", err)
	}
	if sbErr.Code != sandbox.ErrCodeUnsupportedPlatform {
		t.Errorf("expected ErrCodeUnsupportedPlatform, got %v", sbErr.Code)
	}
	if !strings.Contains(sbErr.Message, "runsc") {
		t.Errorf("error message should mention runsc, got %q", sbErr.Message)
	}
}

func TestProvider_Create_FailsWithoutRunsc(t *testing.T) {
	if _, err := exec.LookPath("runsc"); err == nil {
		t.Skip("runsc is installed; cannot test the missing-binary path")
	}

	p := NewProvider()
	tmp := t.TempDir()
	lower := filepath.Join(tmp, "lower")
	if err := os.MkdirAll(lower, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	req := sandbox.SandboxRequest{
		SessionID:    "no-runsc",
		LowerDirs:    []string{lower},
		WorkspaceDir: filepath.Join(tmp, "ws"),
	}
	sb, err := p.Create(context.Background(), req)
	if err == nil {
		t.Fatal("Create() should fail when runsc is not installed")
	}
	if sb != nil {
		t.Errorf("Create() should return nil sandbox on failure, got %+v", sb)
	}
}

func TestGVisorRejectsMatchedNonGitLowerDir(t *testing.T) {
	base := t.TempDir()
	gitRepo := filepath.Join(base, "git-repo")
	requestedRepo := filepath.Join(base, "requested-non-git")
	mergedDir := filepath.Join(base, "merged")
	runGVisorGit(t, "", "init", "-q", "-b", "main", gitRepo)
	if err := os.MkdirAll(requestedRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mergedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitRepo, "project.txt"), []byte("wrong repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(requestedRepo, "project.txt"), []byte("requested repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	orderedLowerDirs := sandbox.PrioritizeLowerDir([]string{gitRepo, requestedRepo}, requestedRepo)
	_, err := p.createPrivateWorktree(orderedLowerDirs, "non-git-authority", mergedDir, requestedRepo)
	if err == nil {
		t.Fatal("createPrivateWorktree() error = nil, want isolation failure")
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) || sbErr.Code != sandbox.ErrCodeMountFailed {
		t.Fatalf("error = %v, want structured %v error", err, sandbox.ErrCodeMountFailed)
	}
	if !strings.Contains(err.Error(), "refusing host-symlink fallback") {
		t.Fatalf("error = %v, want host-symlink refusal", err)
	}
	if _, statErr := os.Lstat(filepath.Join(mergedDir, "project.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed isolation exposed a host repository: stat error = %v", statErr)
	}
}

func TestGVisorPreservesGitWorktreeCreationFailure(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	activeWorktree := filepath.Join(base, "active")
	mergedDir := filepath.Join(base, "merged")
	runGVisorGit(t, "", "init", "-q", "-b", "main", repo)
	runGVisorGit(t, repo, "config", "user.name", "gVisor Test")
	runGVisorGit(t, repo, "config", "user.email", "gvisor@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGVisorGit(t, repo, "add", "README.md")
	runGVisorGit(t, repo, "commit", "-q", "-m", "initial")
	runGVisorGit(t, repo, "worktree", "add", "-q", "-b", "agm/worktree-add-failure", activeWorktree)
	if err := os.MkdirAll(mergedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewProvider().createPrivateWorktree([]string{repo}, "worktree-add-failure", mergedDir, repo)
	if err == nil {
		t.Fatal("createPrivateWorktree() error = nil, want Git worktree failure")
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) || sbErr.Code != sandbox.ErrCodeMountFailed {
		t.Fatalf("error = %v, want structured %v error", err, sandbox.ErrCodeMountFailed)
	}
	if !strings.Contains(err.Error(), "git worktree add failed") || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("error = %v, want preserved Git worktree failure", err)
	}
}

func TestProvider_Destroy_Idempotent(t *testing.T) {
	p := NewProvider()
	if err := p.Destroy(context.Background(), "does-not-exist"); err != nil {
		t.Errorf("Destroy() should be idempotent for unknown id, got %v", err)
	}
}

func TestProvider_DestroyPreservesLockedWorktreeForRetry(t *testing.T) {
	repo, mergedDir, upperDir, workDir := newLockedGVisorWorktree(t)
	p := NewProvider()
	const id = "locked-destroy"
	cleanupState := sandbox.NewWorktreeCleanup(true)
	p.sandboxes[id] = &sandbox.Sandbox{
		ID:         id,
		MergedPath: mergedDir,
		UpperPath:  upperDir,
		WorkPath:   workDir,
		CleanupFunc: func() error {
			return cleanupState.Run(
				func() error { return p.removeWorktree(repo, mergedDir) },
				func() error { return p.cleanup(upperDir, workDir, mergedDir) },
			)
		},
	}

	err := p.Destroy(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "git worktree remove failed") {
		t.Fatalf("Destroy(locked) = %v, want Git removal refusal", err)
	}
	for _, path := range []string{mergedDir, upperDir, workDir} {
		if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
			t.Fatalf("failed destroy removed retryable provider path %s: %v", path, statErr)
		}
	}
	if err := p.Validate(context.Background(), id); err != nil {
		t.Fatalf("failed destroy removed provider registry state: %v", err)
	}
	out := runGVisorGit(t, repo, "worktree", "list", "--porcelain")
	if !strings.Contains(out, "locked provider-active-owner") {
		t.Fatalf("failed destroy changed the exact Git lock reason: %s", out)
	}

	runGVisorGit(t, repo, "worktree", "unlock", mergedDir)
	if err := p.Destroy(context.Background(), id); err != nil {
		t.Fatalf("destroy retry after unlock: %v", err)
	}
	for _, path := range []string{mergedDir, upperDir, workDir} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("successful destroy retained %s: %v", path, statErr)
		}
	}
	if err := p.Validate(context.Background(), id); err == nil {
		t.Fatal("successful destroy retained provider registry state")
	}
}

func newLockedGVisorWorktree(t *testing.T) (repo, mergedDir, upperDir, workDir string) {
	t.Helper()
	base := t.TempDir()
	repo = filepath.Join(base, "repo")
	workspace := filepath.Join(base, "sandbox")
	mergedDir = filepath.Join(workspace, "merged")
	upperDir = filepath.Join(workspace, "upper")
	workDir = filepath.Join(workspace, "work")
	runGVisorGit(t, "", "init", "-q", "-b", "main", repo)
	runGVisorGit(t, repo, "config", "user.name", "gVisor Test")
	runGVisorGit(t, repo, "config", "user.email", "gvisor@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("locked provider cleanup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGVisorGit(t, repo, "add", "README.md")
	runGVisorGit(t, repo, "commit", "-q", "-m", "initial")
	runGVisorGit(t, repo, "worktree", "add", "-q", "-b", "locked-destroy", mergedDir)
	if err := os.MkdirAll(upperDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGVisorGit(t, repo, "worktree", "lock", "--reason", "provider-active-owner", mergedDir)
	return repo, mergedDir, upperDir, workDir
}

func runGVisorGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	commandArgs := slices.Clone(args)
	if dir != "" {
		commandArgs = append([]string{"-C", dir}, commandArgs...)
	}
	cmd := exec.Command("git", commandArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(commandArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}

func TestProvider_Validate_NotFound(t *testing.T) {
	p := NewProvider()
	err := p.Validate(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("Validate() should fail for unknown sandbox id")
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) {
		t.Fatalf("expected *sandbox.Error, got %T", err)
	}
	if sbErr.Code != sandbox.ErrCodeSandboxNotFound {
		t.Errorf("expected ErrCodeSandboxNotFound, got %v", sbErr.Code)
	}
}
