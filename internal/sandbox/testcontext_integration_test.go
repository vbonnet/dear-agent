package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// TestSandboxWithinTestContextHome verifies that ClaudeCodeProvider creates
// workspace directories under TestContext's HOME (i.e. under /tmp/agm-test-{id}/home).
func TestSandboxWithinTestContextHome(t *testing.T) {
	t.Parallel()

	// Simulate a TestContext HOME by using a temp directory that mirrors
	// the /tmp/agm-test-{id}/home structure.
	baseDir := t.TempDir()
	homeDir := filepath.Join(baseDir, "home")
	require.NoError(t, os.MkdirAll(homeDir, 0700))

	// Create a sandbox workspace under the test HOME, as AGM would.
	sandboxBaseDir := filepath.Join(homeDir, ".agm", "sandboxes", "test-session-1")
	require.NoError(t, os.MkdirAll(sandboxBaseDir, 0755))

	provider := sandbox.NewClaudeCodeProvider(nil)
	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "test-session-1",
		WorkspaceDir: sandboxBaseDir,
	})
	require.NoError(t, err)
	require.NotNil(t, sb)

	// The merged path should be under the test HOME
	assert.True(t, strings.HasPrefix(sb.MergedPath, homeDir),
		"MergedPath %s should be under test HOME %s", sb.MergedPath, homeDir)

	// Verify workspace directory was created
	_, err = os.Stat(sb.MergedPath)
	assert.NoError(t, err, "sandbox workspace should exist")

	// Verify the expected path structure
	assert.Equal(t, filepath.Join(sandboxBaseDir, "workspace"), sb.MergedPath)
}

// TestClaudeCodeProviderInTestEnvironment verifies that creating a
// ClaudeCodeProvider and provisioning a sandbox works when the environment
// variables point to a test-isolated HOME.
func TestClaudeCodeProviderInTestEnvironment(t *testing.T) {
	t.Parallel()

	// Set up a test environment that mimics TestContext.
	testHome := t.TempDir()
	agmSandboxDir := filepath.Join(testHome, ".agm", "sandboxes", "env-test")
	require.NoError(t, os.MkdirAll(agmSandboxDir, 0755))

	// Create provider with FullAccessSpec (the default).
	provider := sandbox.NewClaudeCodeProvider(sandbox.FullAccessSpec())
	assert.Equal(t, "claudecode-worktree", provider.Name())

	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "env-test",
		WorkspaceDir: agmSandboxDir,
	})
	require.NoError(t, err)
	require.NotNil(t, sb)

	// Verify the sandbox lives under the test HOME.
	assert.True(t, strings.HasPrefix(sb.MergedPath, testHome),
		"sandbox should be under test HOME")
	assert.Equal(t, "claudecode-worktree", sb.Type)
	assert.False(t, sb.CreatedAt.IsZero())

	// Verify workspace dir exists.
	info, err := os.Stat(sb.MergedPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestClaudeCodeProviderWithReadOnlySpec verifies that a read-only spec
// still provisions the workspace directory correctly.
func TestClaudeCodeProviderWithReadOnlySpec(t *testing.T) {
	t.Parallel()

	testHome := t.TempDir()
	workDir := filepath.Join(testHome, ".agm", "sandboxes", "readonly-test")
	require.NoError(t, os.MkdirAll(workDir, 0755))

	provider := sandbox.NewClaudeCodeProvider(sandbox.ReadOnlySpec())
	assert.Equal(t, "read-only", provider.ToolPreset())

	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "readonly-test",
		WorkspaceDir: workDir,
	})
	require.NoError(t, err)
	require.NotNil(t, sb)

	// Verify allowed tools match read-only spec.
	tools := provider.AllowedTools()
	assert.Contains(t, tools, "Read")
	assert.Contains(t, tools, "Grep")
	assert.NotContains(t, tools, "Write")
	assert.NotContains(t, tools, "Bash")
}

// TestSandboxCleanupWithinTestEnvironment verifies that sandbox Destroy()
// (via CleanupFunc) correctly removes paths under /tmp/agm-test-{id}/.
func TestSandboxCleanupWithinTestEnvironment(t *testing.T) {
	t.Parallel()

	// Create a directory structure mirroring TestContext.
	baseDir := t.TempDir()
	homeDir := filepath.Join(baseDir, "home")
	sandboxDir := filepath.Join(homeDir, ".agm", "sandboxes", "cleanup-test")
	require.NoError(t, os.MkdirAll(sandboxDir, 0755))

	provider := sandbox.NewClaudeCodeProvider(nil)
	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "cleanup-test",
		WorkspaceDir: sandboxDir,
	})
	require.NoError(t, err)

	// Verify workspace exists.
	_, err = os.Stat(sb.MergedPath)
	require.NoError(t, err, "workspace should exist before cleanup")

	// Create some files in the workspace to test recursive removal.
	testFile := filepath.Join(sb.MergedPath, "work-in-progress.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("wip"), 0600))

	// Run cleanup.
	require.NotNil(t, sb.CleanupFunc)
	err = sb.CleanupFunc()
	assert.NoError(t, err, "cleanup should succeed")

	// Verify the entire workspace directory tree is gone.
	_, err = os.Stat(sandboxDir)
	assert.True(t, os.IsNotExist(err), "sandbox dir should be removed after cleanup")

	// The parent .agm/sandboxes/ should still exist (RemoveAll only removes sandboxDir).
	parentDir := filepath.Dir(sandboxDir)
	_, err = os.Stat(parentDir)
	assert.NoError(t, err, "parent directory should still exist")
}

// TestAuthForwardingAndSandbox verifies that when credential directories
// are symlinked (as ForwardAuth does), the sandbox workspace can coexist
// with them under the same HOME.
func TestAuthForwardingAndSandbox(t *testing.T) {
	t.Parallel()

	// Simulate the structure that TestContext + ForwardAuth creates.
	baseDir := t.TempDir()
	homeDir := filepath.Join(baseDir, "home")
	require.NoError(t, os.MkdirAll(homeDir, 0700))

	// Create a fake host home with credential dirs.
	fakeHostHome := t.TempDir()
	claudeDir := filepath.Join(fakeHostHome, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0700))
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0600))

	// Symlink .claude/ into the test HOME (mimicking ForwardAuth inherit mode).
	dst := filepath.Join(homeDir, ".claude")
	require.NoError(t, os.Symlink(claudeDir, dst))

	// Now provision a sandbox under the same test HOME.
	sandboxDir := filepath.Join(homeDir, ".agm", "sandboxes", "auth-test")
	require.NoError(t, os.MkdirAll(sandboxDir, 0755))

	provider := sandbox.NewClaudeCodeProvider(sandbox.FullAccessSpec())
	sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
		SessionID:    "auth-test",
		WorkspaceDir: sandboxDir,
	})
	require.NoError(t, err)
	require.NotNil(t, sb)

	// Verify the symlinked credential dir is still accessible from the test HOME.
	link, err := os.Readlink(filepath.Join(homeDir, ".claude"))
	require.NoError(t, err)
	assert.Equal(t, claudeDir, link)

	// Verify the settings file is readable via the symlink.
	content, err := os.ReadFile(filepath.Join(homeDir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Equal(t, "{}", string(content))

	// Verify sandbox workspace is separate from credential paths.
	assert.True(t, strings.HasPrefix(sb.MergedPath, sandboxDir))

	// Cleanup sandbox should not affect credential symlinks.
	require.NotNil(t, sb.CleanupFunc)
	err = sb.CleanupFunc()
	assert.NoError(t, err)

	// Credential symlink should still exist after sandbox cleanup.
	_, err = os.Lstat(filepath.Join(homeDir, ".claude"))
	assert.NoError(t, err, "credential symlink should survive sandbox cleanup")
}

// TestMultipleSandboxesUnderTestHome verifies that multiple sandboxes can
// coexist under the same test HOME directory.
func TestMultipleSandboxesUnderTestHome(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	sandboxBaseDir := filepath.Join(homeDir, ".agm", "sandboxes")

	provider := sandbox.NewClaudeCodeProvider(nil)

	var sandboxes []*sandbox.Sandbox
	sessionIDs := []string{"session-a", "session-b", "session-c"}

	for _, id := range sessionIDs {
		workDir := filepath.Join(sandboxBaseDir, id)
		require.NoError(t, os.MkdirAll(workDir, 0755))

		sb, err := provider.Create(context.Background(), sandbox.SandboxRequest{
			SessionID:    id,
			WorkspaceDir: workDir,
		})
		require.NoError(t, err)
		sandboxes = append(sandboxes, sb)
	}

	// Verify all sandboxes have distinct merged paths.
	paths := make(map[string]bool)
	for _, sb := range sandboxes {
		assert.False(t, paths[sb.MergedPath], "merged paths should be unique")
		paths[sb.MergedPath] = true

		_, err := os.Stat(sb.MergedPath)
		assert.NoError(t, err)
	}

	// Cleanup one sandbox should not affect others.
	err := sandboxes[0].CleanupFunc()
	assert.NoError(t, err)

	_, err = os.Stat(sandboxes[0].MergedPath)
	assert.True(t, os.IsNotExist(err), "cleaned sandbox should be gone")

	// Other sandboxes should still exist.
	for _, sb := range sandboxes[1:] {
		_, err := os.Stat(sb.MergedPath)
		assert.NoError(t, err, "other sandboxes should survive")
	}
}

// TestSandboxBuildClaudeArgsInTestEnv verifies that BuildClaudeArgs
// produces correct arguments when paths are under a test HOME.
func TestSandboxBuildClaudeArgsInTestEnv(t *testing.T) {
	t.Parallel()

	testHome := t.TempDir()
	workDir := filepath.Join(testHome, ".agm", "sandboxes", "args-test", "workspace")

	spec := &sandbox.SandboxSpec{
		Mode: "worktree",
		Filesystem: &sandbox.FilesystemSpec{
			AllowWrite: []string{filepath.Join(testHome, "extra-dir")},
		},
		Resources: &sandbox.ResourceSpec{
			MaxBudgetUSD: 2.50,
		},
		Tools: &sandbox.ToolSpec{
			Preset:       "code-only",
			AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
		},
	}

	provider := sandbox.NewClaudeCodeProvider(spec)
	args := provider.BuildClaudeArgs(workDir)

	// Should include --add-dir for the extra dir and workDir.
	assert.Contains(t, args, "--add-dir")
	assert.Contains(t, args, filepath.Join(testHome, "extra-dir"))
	assert.Contains(t, args, workDir)

	// Should include budget flag.
	assert.Contains(t, args, "--max-budget-usd")
	assert.Contains(t, args, "2.50")

	// ToolPreset should be correct.
	assert.Equal(t, "code-only", provider.ToolPreset())
	assert.Len(t, provider.AllowedTools(), 6)
}
