package sandbox

// SandboxSpec is a provider-agnostic configuration for sandbox isolation.
// Other components (executor, wayfinder, AGM) compose this to request isolation.
type SandboxSpec struct {
	// Isolation mode: "worktree" (Claude Code native), "overlayfs", "apfs", "none"
	Mode string

	// Filesystem isolation
	Filesystem *FilesystemSpec

	// Network isolation (maps to Claude Code sandbox.network)
	Network *NetworkSpec

	// Resource limits
	Resources *ResourceSpec

	// Tool restrictions for sub-agents
	Tools *ToolSpec
}

// FilesystemSpec configures filesystem-level isolation.
type FilesystemSpec struct {
	// Directories the agent can write to (beyond its worktree)
	AllowWrite []string
	// Directories the agent cannot read
	DenyRead []string
}

// NetworkSpec configures network-level isolation.
type NetworkSpec struct {
	// Allowed domains (empty = all allowed)
	AllowedDomains []string
}

// ResourceSpec configures resource limits for sandbox execution.
type ResourceSpec struct {
	// Maximum execution time
	TimeoutSeconds int
	// Maximum token budget (passed to --max-budget-usd)
	MaxBudgetUSD float64
	// Process limits for fork bomb detection and resource control.
	// Nil means use DefaultProcessLimits().
	ProcessLimits *ProcessLimits
}

// ToolSpec configures which tools are available within the sandbox.
type ToolSpec struct {
	// Allowed tools (empty = all allowed)
	AllowedTools []string
	// Tool preset: "read-only", "full", "code-only"
	Preset string
}

// ReadOnlySpec returns a SandboxSpec configured for read-only access.
// Useful for research, analysis, and review tasks.
func ReadOnlySpec() *SandboxSpec {
	return &SandboxSpec{
		Mode: "worktree",
		Tools: &ToolSpec{
			Preset:       "read-only",
			AllowedTools: []string{"Read", "Grep", "Glob", "WebSearch", "WebFetch"},
		},
	}
}

// FullAccessSpec returns a SandboxSpec with full tool access.
// Suitable for trusted agents that need unrestricted capabilities.
func FullAccessSpec() *SandboxSpec {
	return &SandboxSpec{
		Mode: "worktree",
		Tools: &ToolSpec{
			Preset: "full",
		},
	}
}

// CodeOnlySpec returns a SandboxSpec limited to code editing tools.
// No network, no web search -- just file I/O and shell.
func CodeOnlySpec() *SandboxSpec {
	return &SandboxSpec{
		Mode: "worktree",
		Tools: &ToolSpec{
			Preset:       "code-only",
			AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
		},
	}
}
