package bubblewrap_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/bubblewrap"
)

// skipUnlessBwrapFunctional skips t when bubblewrap is unavailable or when
// the host does not support user namespaces (required for --unshare-all).
// Some CI environments (e.g. GitHub Actions Ubuntu runners) install bwrap but
// block unprivileged user namespace creation, causing sandbox tests to fail.
func skipUnlessBwrapFunctional(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap only available on Linux")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap (bwrap) not installed")
	}
	// Probe: attempt a minimal bwrap execution that exercises user namespaces.
	probe := exec.Command("bwrap",
		"--unshare-all", "--share-net",
		"--ro-bind", "/", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"/bin/true",
	)
	if err := probe.Run(); err != nil {
		t.Skipf("bubblewrap user namespaces not supported in this environment: %v", err)
	}
}

func commitBubblewrapFixtureRepo(t *testing.T, dir string) {
	t.Helper()
	commands := [][]string{
		{"init", "-q", "-b", "main", dir},
		{"-C", dir, "add", "-A"},
		{"-C", dir, "-c", "user.name=AGM Test", "-c", "user.email=agm-test@example.invalid", "commit", "-q", "--allow-empty", "-m", "fixture"},
	}
	for _, args := range commands {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
}

// TestBubblewrap_E2E tests end-to-end Bubblewrap lifecycle
func TestBubblewrap_E2E(t *testing.T) {
	skipUnlessBwrapFunctional(t)

	provider := bubblewrap.NewProvider()
	ctx := context.Background()

	// Create test repository
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	testFiles := map[string]string{
		"README.md":           "# Test Repository",
		"src/main.go":         "package main\n\nfunc main() {}",
		"src/lib/helper.go":   "package lib\n\nfunc Helper() {}",
		"config/settings.yml": "debug: true",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(lowerDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}
	commitBubblewrapFixtureRepo(t, lowerDir)

	req := sandbox.SandboxRequest{
		SessionID:    "bwrap-e2e-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"API_KEY": "test-key-123",
		},
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Bubblewrap sandbox creation must succeed")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify sandbox structure
	t.Run("sandbox_structure", func(t *testing.T) {
		assert.DirExists(t, sb.MergedPath)
		assert.DirExists(t, sb.UpperPath)
		assert.DirExists(t, sb.WorkPath)
		assert.Equal(t, "bubblewrap", sb.Type)
	})

	// Verify secrets written to upperdir/.env
	t.Run("secrets_injection", func(t *testing.T) {
		envFile := filepath.Join(sb.UpperPath, ".env")
		content, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "API_KEY=test-key-123")
	})

	// Validate sandbox
	err = provider.Validate(ctx, sb.ID)
	assert.NoError(t, err)

	// Destroy sandbox
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)

	// Verify cleanup
	_, err = os.Stat(sb.MergedPath)
	assert.True(t, os.IsNotExist(err), "Merged path should be removed after destroy")
}

// TestBubblewrap_MultiRepo tests with multiple lower directories
func TestBubblewrap_MultiRepo(t *testing.T) {
	skipUnlessBwrapFunctional(t)

	provider := bubblewrap.NewProvider()
	ctx := context.Background()

	// Create multiple repositories
	repo1 := t.TempDir()
	repo2 := t.TempDir()
	repo3 := t.TempDir()
	workspaceDir := t.TempDir()

	// Create files in each repo
	err := os.WriteFile(filepath.Join(repo1, "file1.txt"), []byte("repo1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(repo2, "file2.txt"), []byte("repo2"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(repo3, "file3.txt"), []byte("repo3"), 0644)
	require.NoError(t, err)
	commitBubblewrapFixtureRepo(t, repo1)
	commitBubblewrapFixtureRepo(t, repo2)
	commitBubblewrapFixtureRepo(t, repo3)

	req := sandbox.SandboxRequest{
		SessionID:    "bwrap-multi-repo-test",
		LowerDirs:    []string{repo1, repo2, repo3},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Multi-repo sandbox creation must succeed")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestBubblewrap_ValidationErrors tests validation error handling
func TestBubblewrap_ValidationErrors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap only available on Linux")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap (bwrap) not installed")
	}

	provider := bubblewrap.NewProvider()
	ctx := context.Background()

	tests := []struct {
		name      string
		req       sandbox.SandboxRequest
		wantError bool
		errorCode sandbox.ErrorCode
	}{
		{
			name: "empty_session_id",
			req: sandbox.SandboxRequest{
				SessionID:    "",
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: t.TempDir(),
			},
			wantError: true,
			errorCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "empty_lower_dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{},
				WorkspaceDir: t.TempDir(),
			},
			wantError: true,
			errorCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "nonexistent_lower_dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/nonexistent/path"},
				WorkspaceDir: t.TempDir(),
			},
			wantError: true,
			errorCode: sandbox.ErrCodeRepoNotFound,
		},
		{
			name: "empty_workspace_dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: "",
			},
			wantError: true,
			errorCode: sandbox.ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.Create(ctx, tt.req)
			if tt.wantError {
				require.Error(t, err)
				var sbErr *sandbox.Error
				if assert.ErrorAs(t, err, &sbErr) {
					assert.Equal(t, tt.errorCode, sbErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBubblewrap_IdempotentDestroy tests destroy idempotency
func TestBubblewrap_IdempotentDestroy(t *testing.T) {
	skipUnlessBwrapFunctional(t)

	provider := bubblewrap.NewProvider()
	ctx := context.Background()

	// Destroy non-existent sandbox should succeed
	err := provider.Destroy(ctx, "nonexistent-sandbox")
	assert.NoError(t, err, "Destroy should be idempotent")

	// Destroy same sandbox multiple times should succeed
	lowerDir := t.TempDir()
	commitBubblewrapFixtureRepo(t, lowerDir)
	req := sandbox.SandboxRequest{
		SessionID:    "bwrap-idempotent-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: t.TempDir(),
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, sb)

	// First destroy
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)

	// Second destroy
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err, "Second destroy should also succeed")
}

// TestBubblewrap_NetworkIsolation verifies that --share-net being absent from
// the bwrap arg list actually isolates the network namespace.
//
// This test skips on environments (GitHub Actions Ubuntu runners) that do not
// support unprivileged network namespace unsharing. It is the companion to the
// note in testBubblewrap() — the startup self-test cannot validate network
// isolation because of the RTM_NEWADDR restriction, so this integration test
// covers that gap on developer machines and capable CI runners.
func TestBubblewrap_NetworkIsolation(t *testing.T) {
	skipUnlessBwrapFunctional(t)

	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) not installed — cannot probe network routes inside sandbox")
	}

	// Additional probe: check whether the host allows unsharing the network
	// namespace without --share-net. GitHub Actions runners reject this.
	probe := exec.Command("bwrap",
		"--unshare-all",
		"--ro-bind", "/", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"/bin/true",
	)
	if err := probe.Run(); err != nil {
		t.Skipf("network namespace unsharing not supported in this environment: %v", err)
	}

	// Run `ip route` without --share-net. In an isolated network namespace the
	// routing table is empty (no default route); with --share-net it contains
	// the host routes.  Any non-empty output means host routes leaked in.
	lowerDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(lowerDir, ".placeholder"), []byte("x"), 0o600))

	args := buildNetworkIsolationBwrapArgs(lowerDir)
	out, err := exec.Command("bwrap", append(args, "/bin/ip", "route")...).Output()
	require.NoError(t, err, "bwrap ip route should not error in isolated sandbox")

	routeOutput := strings.TrimSpace(string(out))
	assert.Empty(t, routeOutput,
		"network-isolated sandbox must have empty routing table, got:\n%s", routeOutput)

	// Positive control: with --share-net the host routing table is visible.
	sharedBaseArgs := make([]string, len(args))
	copy(sharedBaseArgs, args)
	sharedBaseArgs = append(sharedBaseArgs, "--share-net")
	sharedOut, err := exec.Command("bwrap", append(sharedBaseArgs, "/bin/ip", "route")...).Output()
	require.NoError(t, err, "bwrap ip route with --share-net should not error")
	assert.NotEmpty(t, strings.TrimSpace(string(sharedOut)),
		"network-shared sandbox should see host routes")
}

// buildNetworkIsolationBwrapArgs returns a minimal bwrap argument list suitable
// for the network isolation test — no --share-net, bind /usr /bin /lib.
func buildNetworkIsolationBwrapArgs(lowerDir string) []string {
	args := []string{
		"--unshare-all",
		"--ro-bind", "/", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--bind", lowerDir, "/sandbox",
		"--new-session",
	}
	return args
}
