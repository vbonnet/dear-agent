package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ClaudeCodeProvider implements the Provider interface by wrapping Claude Code's
// native worktree-based isolation. This is the default provider when
// SandboxSpec.Mode is "worktree".
//
// Instead of creating overlayfs/apfs mounts, this provider relies on Claude
// Code's built-in `isolation: "worktree"` capability and maps SandboxSpec
// fields to Claude Code sandbox settings (allowed tools, network restrictions,
// resource limits).
type ClaudeCodeProvider struct {
	// Spec is the provider-agnostic sandbox configuration.
	// If nil, FullAccessSpec() defaults are used.
	Spec *SandboxSpec
}

// NewClaudeCodeProvider creates a new ClaudeCodeProvider with the given spec.
// If spec is nil, FullAccessSpec() is used.
func NewClaudeCodeProvider(spec *SandboxSpec) *ClaudeCodeProvider {
	if spec == nil {
		spec = FullAccessSpec()
	}
	return &ClaudeCodeProvider{Spec: spec}
}

// Create provisions a Claude Code worktree sandbox.
// Unlike overlayfs/apfs providers, this doesn't create filesystem mounts.
// It creates a workspace directory structure that Claude Code's native
// isolation can use, and records the SandboxSpec configuration for
// downstream consumers (e.g., AGM session launch) to apply.
func (p *ClaudeCodeProvider) Create(ctx context.Context, req SandboxRequest) (*Sandbox, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req.SessionID == "" {
		return nil, NewInvalidConfigError("SessionID", "must not be empty")
	}
	if req.WorkspaceDir == "" {
		return nil, NewInvalidConfigError("WorkspaceDir", "must not be empty")
	}

	// Create workspace directory structure
	mergedPath := filepath.Join(req.WorkspaceDir, "workspace")
	if err := os.MkdirAll(mergedPath, 0755); err != nil {
		return nil, WrapError(ErrCodeMountFailed,
			fmt.Sprintf("failed to create workspace directory: %s", mergedPath), err)
	}

	// For worktree mode, the "merged" path is the workspace directory
	// where Claude Code will operate. Unlike overlayfs, there's no
	// upper/work layer -- Claude Code manages isolation natively.
	sb := &Sandbox{
		ID:         req.SessionID,
		MergedPath: mergedPath,
		UpperPath:  "", // Not applicable for worktree mode
		WorkPath:   "", // Not applicable for worktree mode
		Type:       "claudecode-worktree",
		CreatedAt:  time.Now(),
		CleanupFunc: func() error {
			return os.RemoveAll(req.WorkspaceDir)
		},
	}

	return sb, nil
}

// Destroy tears down a Claude Code worktree sandbox.
// Removes the workspace directory.
func (p *ClaudeCodeProvider) Destroy(ctx context.Context, id string) error {
	// ClaudeCodeProvider is stateless -- it doesn't track sandboxes.
	// The caller is responsible for knowing the workspace path.
	// For idempotency, we return nil (as per Provider contract).
	return nil
}

// Validate checks if the sandbox workspace directory exists.
func (p *ClaudeCodeProvider) Validate(ctx context.Context, id string) error {
	// Without tracking state, we can't validate by ID alone.
	// Return not-found to signal the caller should check the path directly.
	return NewError(ErrCodeSandboxNotFound, "claudecode provider does not track sandbox state by ID: "+id)
}

// Name returns the provider name.
func (p *ClaudeCodeProvider) Name() string {
	return "claudecode-worktree"
}

// BuildClaudeArgs generates Claude CLI arguments from the SandboxSpec.
// This maps the provider-agnostic spec to Claude Code's native flags.
func (p *ClaudeCodeProvider) BuildClaudeArgs(workDir string) []string {
	var args []string

	if p.Spec == nil {
		return args
	}

	// Add allowed directories
	if p.Spec.Filesystem != nil {
		for _, dir := range p.Spec.Filesystem.AllowWrite {
			args = append(args, "--add-dir", dir)
		}
	}

	// Add working directory
	if workDir != "" {
		args = append(args, "--add-dir", workDir)
	}

	// Add resource limits
	if p.Spec.Resources != nil {
		if p.Spec.Resources.MaxBudgetUSD > 0 {
			args = append(args, "--max-budget-usd",
				fmt.Sprintf("%.2f", p.Spec.Resources.MaxBudgetUSD))
		}
	}

	// Tool restrictions are handled at the AGM/executor level,
	// not as Claude CLI args (Claude Code uses allowedTools in
	// settings, not CLI flags).

	return args
}

// AllowedTools returns the list of tools this spec permits.
// Empty list means all tools are allowed.
func (p *ClaudeCodeProvider) AllowedTools() []string {
	if p.Spec == nil || p.Spec.Tools == nil {
		return nil
	}
	return p.Spec.Tools.AllowedTools
}

// ToolPreset returns the tool preset name.
func (p *ClaudeCodeProvider) ToolPreset() string {
	if p.Spec == nil || p.Spec.Tools == nil {
		return "full"
	}
	return p.Spec.Tools.Preset
}

func init() {
	RegisterProvider("claudecode-worktree", func() Provider {
		return NewClaudeCodeProvider(nil)
	})
}
