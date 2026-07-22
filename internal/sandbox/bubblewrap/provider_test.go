package bubblewrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// These tests cover the cross-platform, pure-logic surface of the Bubblewrap
// provider. The real bwrap enforcement path is Linux-only (see
// integration_test.go, which t.Skip()s on non-Linux), so the
// security-relevant input gate validateRequest — which decides whether a
// sandbox is even attempted and which host paths get exposed — is the part
// that must be protected by tests that run everywhere.

func TestProvider_Name(t *testing.T) {
	assert.Equal(t, "bubblewrap", NewProvider().Name())
}

// errCode extracts the structured sandbox error code, failing the test if
// err is not a *sandbox.Error.
func errCode(t *testing.T, err error) sandbox.ErrorCode {
	t.Helper()
	var sErr *sandbox.Error
	require.True(t, errors.As(err, &sErr), "expected *sandbox.Error, got %T (%v)", err, err)
	return sErr.Code
}

func TestProvider_validateRequest(t *testing.T) {
	// An existing directory to satisfy the LowerDirs stat check on the
	// happy path and the "workspace empty" case.
	existing := t.TempDir()

	tests := []struct {
		name     string
		req      sandbox.SandboxRequest
		wantErr  bool
		wantCode sandbox.ErrorCode
	}{
		{
			name: "empty session id",
			req: sandbox.SandboxRequest{
				LowerDirs:    []string{existing},
				WorkspaceDir: "/tmp/ws",
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "no lower dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "s1",
				WorkspaceDir: "/tmp/ws",
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "lower dir does not exist",
			req: sandbox.SandboxRequest{
				SessionID:    "s1",
				LowerDirs:    []string{"/definitely/not/a/real/path/xyzzy"},
				WorkspaceDir: "/tmp/ws",
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeRepoNotFound,
		},
		{
			name: "empty workspace dir",
			req: sandbox.SandboxRequest{
				SessionID: "s1",
				LowerDirs: []string{existing},
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "valid request",
			req: sandbox.SandboxRequest{
				SessionID:    "s1",
				LowerDirs:    []string{existing},
				WorkspaceDir: "/tmp/ws",
			},
			wantErr: false,
		},
	}

	p := NewProvider()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.validateRequest(tt.req)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, errCode(t, err))
		})
	}
}

func TestProvider_validateRequest_multipleLowerDirs_oneMissing(t *testing.T) {
	// Every LowerDir must exist; a single missing entry must be rejected so
	// a sandbox is never created with a silently-dropped mount.
	p := NewProvider()
	err := p.validateRequest(sandbox.SandboxRequest{
		SessionID:    "s1",
		LowerDirs:    []string{t.TempDir(), "/definitely/not/real/abc"},
		WorkspaceDir: "/tmp/ws",
	})
	require.Error(t, err)
	assert.Equal(t, sandbox.ErrCodeRepoNotFound, errCode(t, err))
}

func TestProvider_checkBubblewrapInstalled(t *testing.T) {
	err := NewProvider().checkBubblewrapInstalled()
	if _, lookErr := exec.LookPath("bwrap"); lookErr != nil {
		// bwrap absent (the usual case on darwin): must report the
		// structured unsupported-platform error, not a nil/opaque one.
		require.Error(t, err)
		assert.Equal(t, sandbox.ErrCodeUnsupportedPlatform, errCode(t, err))
	} else {
		assert.NoError(t, err)
	}
}

func TestBubblewrapRejectsMatchedNonGitLowerDir(t *testing.T) {
	base := t.TempDir()
	gitRepo := filepath.Join(base, "git-repo")
	requestedRepo := filepath.Join(base, "requested-non-git")
	mergedDir := filepath.Join(base, "merged")
	runBubblewrapGit(t, "", "init", "-q", "-b", "main", gitRepo)
	require.NoError(t, os.MkdirAll(requestedRepo, 0o755))
	require.NoError(t, os.MkdirAll(mergedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitRepo, "project.txt"), []byte("wrong repo\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(requestedRepo, "project.txt"), []byte("requested repo\n"), 0o600))

	p := NewProvider()
	orderedLowerDirs := sandbox.PrioritizeLowerDir([]string{gitRepo, requestedRepo}, requestedRepo)
	_, err := p.createPrivateWorktree(orderedLowerDirs, "non-git-authority", mergedDir, requestedRepo)
	require.Error(t, err)
	assert.Equal(t, sandbox.ErrCodeMountFailed, errCode(t, err))
	assert.Contains(t, err.Error(), "refusing host-symlink fallback")
	_, statErr := os.Lstat(filepath.Join(mergedDir, "project.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "failed isolation must not expose either host repository")
}

func TestBubblewrapPreservesGitWorktreeCreationFailure(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	activeWorktree := filepath.Join(base, "active")
	mergedDir := filepath.Join(base, "merged")
	runBubblewrapGit(t, "", "init", "-q", "-b", "main", repo)
	runBubblewrapGit(t, repo, "config", "user.name", "Bubblewrap Test")
	runBubblewrapGit(t, repo, "config", "user.email", "bubblewrap@example.invalid")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600))
	runBubblewrapGit(t, repo, "add", "README.md")
	runBubblewrapGit(t, repo, "commit", "-q", "-m", "initial")
	runBubblewrapGit(t, repo, "worktree", "add", "-q", "-b", "agm/worktree-add-failure", activeWorktree)
	require.NoError(t, os.MkdirAll(mergedDir, 0o755))

	_, err := NewProvider().createPrivateWorktree([]string{repo}, "worktree-add-failure", mergedDir, repo)
	require.Error(t, err)
	assert.Equal(t, sandbox.ErrCodeMountFailed, errCode(t, err))
	assert.Contains(t, err.Error(), "git worktree add failed")
	assert.Contains(t, err.Error(), "delete failed")
}

func TestProvider_DestroyPreservesLockedWorktreeForRetry(t *testing.T) {
	repo, mergedDir, upperDir, workDir := newLockedBubblewrapWorktree(t)
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git worktree remove failed")
	for _, path := range []string{mergedDir, upperDir, workDir} {
		require.DirExists(t, path, "failed destroy removed retryable provider path")
	}
	require.NoError(t, p.Validate(context.Background(), id), "failed destroy removed provider registry state")
	out := runBubblewrapGit(t, repo, "worktree", "list", "--porcelain")
	assert.Contains(t, out, "locked provider-active-owner", "failed destroy changed the exact Git lock reason")

	runBubblewrapGit(t, repo, "worktree", "unlock", mergedDir)
	require.NoError(t, p.Destroy(context.Background(), id), "destroy retry after unlock")
	for _, path := range []string{mergedDir, upperDir, workDir} {
		_, statErr := os.Stat(path)
		assert.True(t, os.IsNotExist(statErr), "successful destroy retained %s: %v", path, statErr)
	}
	require.Error(t, p.Validate(context.Background(), id), "successful destroy retained provider registry state")
}

func newLockedBubblewrapWorktree(t *testing.T) (repo, mergedDir, upperDir, workDir string) {
	t.Helper()
	base := t.TempDir()
	repo = filepath.Join(base, "repo")
	workspace := filepath.Join(base, "sandbox")
	mergedDir = filepath.Join(workspace, "merged")
	upperDir = filepath.Join(workspace, "upper")
	workDir = filepath.Join(workspace, "work")
	runBubblewrapGit(t, "", "init", "-q", "-b", "main", repo)
	runBubblewrapGit(t, repo, "config", "user.name", "Bubblewrap Test")
	runBubblewrapGit(t, repo, "config", "user.email", "bubblewrap@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("locked provider cleanup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runBubblewrapGit(t, repo, "add", "README.md")
	runBubblewrapGit(t, repo, "commit", "-q", "-m", "initial")
	runBubblewrapGit(t, repo, "worktree", "add", "-q", "-b", "locked-destroy", mergedDir)
	require.NoError(t, os.MkdirAll(upperDir, 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	runBubblewrapGit(t, repo, "worktree", "lock", "--reason", "provider-active-owner", mergedDir)
	return repo, mergedDir, upperDir, workDir
}

func runBubblewrapGit(t *testing.T, dir string, args ...string) string {
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

// ---- buildBwrapArgs unit tests (cross-platform, no exec) ----

func TestBuildBwrapArgs_SecurityInvariants(t *testing.T) {
	// These invariants must hold on any OS; they are the security contract
	// for the bubblewrap sandbox layer.
	args := buildBwrapArgs(
		[]string{"/repo"},
		"/upper",
		false, // network isolated by default
		nil,   // no extra ro paths
	)

	argSet := toArgSet(args)
	t.Run("unshare_all_present", func(t *testing.T) {
		assert.Contains(t, args, "--unshare-all",
			"--unshare-all must be present to isolate all kernel namespaces")
	})
	t.Run("die_with_parent_present", func(t *testing.T) {
		assert.Contains(t, args, "--die-with-parent",
			"--die-with-parent must be present so orphaned sandboxes are killed")
	})
	t.Run("lower_dirs_are_readonly", func(t *testing.T) {
		// The repo is mounted read-only; --bind (writable) must not appear
		// for lower dirs.
		idx := indexArg(args, "--ro-bind")
		found := false
		for _, i := range idx {
			if i+1 < len(args) && args[i+1] == "/repo" {
				found = true
			}
		}
		assert.True(t, found, "lower dir /repo must be mounted --ro-bind")
		// Verify /repo does NOT appear after a --bind flag.
		bindIdx := indexArg(args, "--bind")
		for _, i := range bindIdx {
			if i+1 < len(args) {
				assert.NotEqual(t, "/repo", args[i+1],
					"lower dir /repo must never be mounted --bind (writable)")
			}
		}
	})
	t.Run("upper_dir_is_writable", func(t *testing.T) {
		// upper dir is bind-mounted writable so the sandbox can write output.
		bindIdx := indexArg(args, "--bind")
		found := false
		for _, i := range bindIdx {
			if i+1 < len(args) && args[i+1] == "/upper" {
				found = true
			}
		}
		assert.True(t, found, "upper dir must be --bind (writable)")
	})
	t.Run("proc_and_dev_present", func(t *testing.T) {
		assert.True(t, argSet["--proc"], "--proc /proc must be present")
		assert.True(t, argSet["--dev"], "--dev /dev must be present")
	})
}

func TestBuildBwrapArgs_NetworkIsolated_NoShareNet(t *testing.T) {
	args := buildBwrapArgs([]string{"/repo"}, "/upper", false, nil)
	assert.NotContains(t, args, "--share-net",
		"shareNetwork=false must omit --share-net (network namespace isolated)")
}

func TestBuildBwrapArgs_NetworkShared_HasShareNet(t *testing.T) {
	args := buildBwrapArgs([]string{"/repo"}, "/upper", true, nil)
	assert.Contains(t, args, "--share-net",
		"shareNetwork=true must include --share-net")
}

func TestBuildBwrapArgs_MultipleLowerDirs_AllReadOnly(t *testing.T) {
	dirs := []string{"/repo-a", "/repo-b", "/repo-c"}
	args := buildBwrapArgs(dirs, "/upper", false, nil)
	for i, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			// Every lower dir must appear as --ro-bind <dir> /sandbox/lowerN.
			expectedDest := fmt.Sprintf("/sandbox/lower%d", i)
			idx := indexArg(args, "--ro-bind")
			found := false
			for _, j := range idx {
				if j+1 < len(args) && args[j+1] == dir {
					require.True(t, j+2 < len(args), "missing destination after --ro-bind %s", dir)
					assert.Equal(t, expectedDest, args[j+2])
					found = true
				}
			}
			assert.True(t, found, "--ro-bind for %s not found", dir)
		})
	}
}

func TestBuildBwrapArgs_ExtraROPaths_IncludedReadOnly(t *testing.T) {
	extra := []string{"/lib64", "/sbin"}
	args := buildBwrapArgs([]string{"/repo"}, "/upper", false, extra)
	for _, p := range extra {
		t.Run(p, func(t *testing.T) {
			idx := indexArg(args, "--ro-bind")
			found := false
			for _, i := range idx {
				if i+1 < len(args) && args[i+1] == p {
					found = true
				}
			}
			assert.True(t, found, "%s must appear as --ro-bind", p)
		})
	}
}

// toArgSet converts an arg slice to a set of flags (values omitted) for
// quick membership tests.
func toArgSet(args []string) map[string]bool {
	s := make(map[string]bool, len(args))
	for _, a := range args {
		s[a] = true
	}
	return s
}

// indexArg returns all indices where flag appears in args.
func indexArg(args []string, flag string) []int {
	var out []int
	for i, a := range args {
		if a == flag {
			out = append(out, i)
		}
	}
	return out
}
