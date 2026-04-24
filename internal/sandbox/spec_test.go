package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadOnlySpec(t *testing.T) {
	spec := ReadOnlySpec()

	require.NotNil(t, spec)
	assert.Equal(t, "worktree", spec.Mode)
	require.NotNil(t, spec.Tools)
	assert.Equal(t, "read-only", spec.Tools.Preset)
	assert.Contains(t, spec.Tools.AllowedTools, "Read")
	assert.Contains(t, spec.Tools.AllowedTools, "Grep")
	assert.Contains(t, spec.Tools.AllowedTools, "Glob")
	assert.Contains(t, spec.Tools.AllowedTools, "WebSearch")
	assert.Contains(t, spec.Tools.AllowedTools, "WebFetch")
	// Must NOT contain write tools
	assert.NotContains(t, spec.Tools.AllowedTools, "Write")
	assert.NotContains(t, spec.Tools.AllowedTools, "Edit")
	assert.NotContains(t, spec.Tools.AllowedTools, "Bash")
	assert.Len(t, spec.Tools.AllowedTools, 5)

	// Other specs should be nil (not configured)
	assert.Nil(t, spec.Filesystem)
	assert.Nil(t, spec.Network)
	assert.Nil(t, spec.Resources)
}

func TestFullAccessSpec(t *testing.T) {
	spec := FullAccessSpec()

	require.NotNil(t, spec)
	assert.Equal(t, "worktree", spec.Mode)
	require.NotNil(t, spec.Tools)
	assert.Equal(t, "full", spec.Tools.Preset)
	// Full access = empty allowed list means all tools permitted
	assert.Empty(t, spec.Tools.AllowedTools)

	assert.Nil(t, spec.Filesystem)
	assert.Nil(t, spec.Network)
	assert.Nil(t, spec.Resources)
}

func TestCodeOnlySpec(t *testing.T) {
	spec := CodeOnlySpec()

	require.NotNil(t, spec)
	assert.Equal(t, "worktree", spec.Mode)
	require.NotNil(t, spec.Tools)
	assert.Equal(t, "code-only", spec.Tools.Preset)
	assert.Contains(t, spec.Tools.AllowedTools, "Read")
	assert.Contains(t, spec.Tools.AllowedTools, "Write")
	assert.Contains(t, spec.Tools.AllowedTools, "Edit")
	assert.Contains(t, spec.Tools.AllowedTools, "Bash")
	assert.Contains(t, spec.Tools.AllowedTools, "Grep")
	assert.Contains(t, spec.Tools.AllowedTools, "Glob")
	// Must NOT contain web tools
	assert.NotContains(t, spec.Tools.AllowedTools, "WebSearch")
	assert.NotContains(t, spec.Tools.AllowedTools, "WebFetch")
	assert.Len(t, spec.Tools.AllowedTools, 6)

	assert.Nil(t, spec.Filesystem)
	assert.Nil(t, spec.Network)
	assert.Nil(t, spec.Resources)
}

func TestSandboxSpec_ManualConstruction(t *testing.T) {
	spec := &SandboxSpec{
		Mode: "overlayfs",
		Filesystem: &FilesystemSpec{
			AllowWrite: []string{"/tmp/output"},
			DenyRead:   []string{"/etc/secrets"},
		},
		Network: &NetworkSpec{
			AllowedDomains: []string{"api.example.com", "cdn.example.com"},
		},
		Resources: &ResourceSpec{
			TimeoutSeconds: 300,
			MaxBudgetUSD:   5.0,
		},
		Tools: &ToolSpec{
			Preset:       "custom",
			AllowedTools: []string{"Read", "Write"},
		},
	}

	assert.Equal(t, "overlayfs", spec.Mode)
	require.NotNil(t, spec.Filesystem)
	assert.Equal(t, []string{"/tmp/output"}, spec.Filesystem.AllowWrite)
	assert.Equal(t, []string{"/etc/secrets"}, spec.Filesystem.DenyRead)
	require.NotNil(t, spec.Network)
	assert.Len(t, spec.Network.AllowedDomains, 2)
	require.NotNil(t, spec.Resources)
	assert.Equal(t, 300, spec.Resources.TimeoutSeconds)
	assert.Equal(t, 5.0, spec.Resources.MaxBudgetUSD)
	require.NotNil(t, spec.Tools)
	assert.Equal(t, "custom", spec.Tools.Preset)
	assert.Len(t, spec.Tools.AllowedTools, 2)
}

func TestSandboxSpec_NoneMode(t *testing.T) {
	spec := &SandboxSpec{
		Mode: "none",
	}

	assert.Equal(t, "none", spec.Mode)
	assert.Nil(t, spec.Filesystem)
	assert.Nil(t, spec.Network)
	assert.Nil(t, spec.Resources)
	assert.Nil(t, spec.Tools)
}
