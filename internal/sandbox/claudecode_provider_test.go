package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClaudeCodeProvider_NilSpec(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	require.NotNil(t, p)
	require.NotNil(t, p.Spec)
	assert.Equal(t, "worktree", p.Spec.Mode)
	assert.Equal(t, "full", p.Spec.Tools.Preset)
}

func TestNewClaudeCodeProvider_WithSpec(t *testing.T) {
	spec := ReadOnlySpec()
	p := NewClaudeCodeProvider(spec)
	require.NotNil(t, p)
	assert.Equal(t, spec, p.Spec)
	assert.Equal(t, "read-only", p.Spec.Tools.Preset)
}

func TestClaudeCodeProvider_Name(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	assert.Equal(t, "claudecode-worktree", p.Name())
}

func TestClaudeCodeProvider_Create_Success(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	workDir := t.TempDir()

	sb, err := p.Create(context.Background(), SandboxRequest{
		SessionID:    "test-session",
		WorkspaceDir: workDir,
	})

	require.NoError(t, err)
	require.NotNil(t, sb)
	assert.Equal(t, "test-session", sb.ID)
	assert.Equal(t, "claudecode-worktree", sb.Type)
	assert.Equal(t, filepath.Join(workDir, "workspace"), sb.MergedPath)
	assert.Empty(t, sb.UpperPath)
	assert.Empty(t, sb.WorkPath)
	assert.False(t, sb.CreatedAt.IsZero())

	// Verify workspace directory was created
	_, err = os.Stat(sb.MergedPath)
	assert.NoError(t, err)

	// Verify cleanup function works
	require.NotNil(t, sb.CleanupFunc)
	err = sb.CleanupFunc()
	assert.NoError(t, err)
	_, err = os.Stat(workDir)
	assert.True(t, os.IsNotExist(err))
}

func TestClaudeCodeProvider_Create_EmptySessionID(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	_, err := p.Create(context.Background(), SandboxRequest{
		WorkspaceDir: "/tmp/test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SessionID")
}

func TestClaudeCodeProvider_Create_EmptyWorkspaceDir(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	_, err := p.Create(context.Background(), SandboxRequest{
		SessionID: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WorkspaceDir")
}

func TestClaudeCodeProvider_Create_CancelledContext(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Create(ctx, SandboxRequest{
		SessionID:    "test",
		WorkspaceDir: t.TempDir(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestClaudeCodeProvider_Destroy(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	// Destroy is stateless/idempotent - should always return nil
	err := p.Destroy(context.Background(), "any-id")
	assert.NoError(t, err)
}

func TestClaudeCodeProvider_Validate(t *testing.T) {
	p := NewClaudeCodeProvider(nil)
	err := p.Validate(context.Background(), "any-id")
	require.Error(t, err)
	var sbErr *Error
	require.ErrorAs(t, err, &sbErr)
	assert.Equal(t, ErrCodeSandboxNotFound, sbErr.Code)
}

func TestClaudeCodeProvider_BuildClaudeArgs_NilSpec(t *testing.T) {
	p := &ClaudeCodeProvider{Spec: nil}
	args := p.BuildClaudeArgs("/work")
	assert.Empty(t, args)
}

func TestClaudeCodeProvider_BuildClaudeArgs_WithWorkDir(t *testing.T) {
	p := NewClaudeCodeProvider(FullAccessSpec())
	args := p.BuildClaudeArgs("/my/workdir")
	assert.Contains(t, args, "--add-dir")
	assert.Contains(t, args, "/my/workdir")
}

func TestClaudeCodeProvider_BuildClaudeArgs_EmptyWorkDir(t *testing.T) {
	p := NewClaudeCodeProvider(FullAccessSpec())
	args := p.BuildClaudeArgs("")
	assert.Empty(t, args)
}

func TestClaudeCodeProvider_BuildClaudeArgs_WithFilesystem(t *testing.T) {
	spec := &SandboxSpec{
		Mode: "worktree",
		Filesystem: &FilesystemSpec{
			AllowWrite: []string{"/tmp/output", "/var/data"},
		},
	}
	p := NewClaudeCodeProvider(spec)
	args := p.BuildClaudeArgs("/work")

	// Should have --add-dir for each AllowWrite + the workDir
	addDirCount := 0
	for _, a := range args {
		if a == "--add-dir" {
			addDirCount++
		}
	}
	assert.Equal(t, 3, addDirCount) // 2 AllowWrite + 1 workDir
	assert.Contains(t, args, "/tmp/output")
	assert.Contains(t, args, "/var/data")
	assert.Contains(t, args, "/work")
}

func TestClaudeCodeProvider_BuildClaudeArgs_WithBudget(t *testing.T) {
	spec := &SandboxSpec{
		Mode: "worktree",
		Resources: &ResourceSpec{
			MaxBudgetUSD: 5.0,
		},
	}
	p := NewClaudeCodeProvider(spec)
	args := p.BuildClaudeArgs("")

	assert.Contains(t, args, "--max-budget-usd")
	assert.Contains(t, args, "5.00")
}

func TestClaudeCodeProvider_BuildClaudeArgs_ZeroBudget(t *testing.T) {
	spec := &SandboxSpec{
		Mode: "worktree",
		Resources: &ResourceSpec{
			MaxBudgetUSD: 0,
		},
	}
	p := NewClaudeCodeProvider(spec)
	args := p.BuildClaudeArgs("")
	assert.NotContains(t, args, "--max-budget-usd")
}

func TestClaudeCodeProvider_AllowedTools_NilSpec(t *testing.T) {
	p := &ClaudeCodeProvider{Spec: nil}
	assert.Nil(t, p.AllowedTools())
}

func TestClaudeCodeProvider_AllowedTools_NilToolSpec(t *testing.T) {
	p := NewClaudeCodeProvider(&SandboxSpec{Mode: "worktree"})
	assert.Nil(t, p.AllowedTools())
}

func TestClaudeCodeProvider_AllowedTools_ReadOnly(t *testing.T) {
	p := NewClaudeCodeProvider(ReadOnlySpec())
	tools := p.AllowedTools()
	assert.Len(t, tools, 5)
	assert.Contains(t, tools, "Read")
	assert.NotContains(t, tools, "Write")
}

func TestClaudeCodeProvider_AllowedTools_Full(t *testing.T) {
	p := NewClaudeCodeProvider(FullAccessSpec())
	tools := p.AllowedTools()
	assert.Empty(t, tools) // empty means all allowed
}

func TestClaudeCodeProvider_ToolPreset_NilSpec(t *testing.T) {
	p := &ClaudeCodeProvider{Spec: nil}
	assert.Equal(t, "full", p.ToolPreset())
}

func TestClaudeCodeProvider_ToolPreset_NilToolSpec(t *testing.T) {
	p := NewClaudeCodeProvider(&SandboxSpec{Mode: "worktree"})
	assert.Equal(t, "full", p.ToolPreset())
}

func TestClaudeCodeProvider_ToolPreset_ReadOnly(t *testing.T) {
	p := NewClaudeCodeProvider(ReadOnlySpec())
	assert.Equal(t, "read-only", p.ToolPreset())
}

func TestClaudeCodeProvider_ToolPreset_CodeOnly(t *testing.T) {
	p := NewClaudeCodeProvider(CodeOnlySpec())
	assert.Equal(t, "code-only", p.ToolPreset())
}

func TestClaudeCodeProvider_RegistryInit(t *testing.T) {
	// Verify the provider registered itself
	p, err := NewProviderForPlatform("claudecode-worktree")
	require.NoError(t, err)
	assert.Equal(t, "claudecode-worktree", p.Name())
}
