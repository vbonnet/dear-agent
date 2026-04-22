# Sandbox Provider Specification

## Overview

The sandbox package provides isolated filesystem environments for AGM sessions using platform-native technologies (OverlayFS on Linux, APFS on macOS). This enables agents to operate in secure, copy-on-write sandboxes that prevent host corruption.

## Goals

1. **Zero-Copy Isolation**: Enable agents to operate in isolated filesystems without duplicating repository data
2. **Host Protection**: Prevent destructive operations (`rm -rf *`) from affecting host filesystem
3. **Multi-Repository Support**: Merge multiple repositories into a single workspace view
4. **Secrets Management**: Inject credentials securely into sandbox environment
5. **Cross-Platform**: Support Linux (OverlayFS) and macOS (APFS) with consistent interface

## Architecture

### Provider Interface

```go
type Provider interface {
    // Create provisions a new sandbox environment
    Create(ctx context.Context, req SandboxRequest) (*Sandbox, error)

    // Destroy tears down a sandbox and cleans up resources
    Destroy(ctx context.Context, sandboxID string) error

    // Validate checks if a sandbox is healthy
    Validate(ctx context.Context, sandboxID string) error

    // Name returns the provider's identifier
    Name() string
}
```

### Platform Detection

The factory auto-detects the best provider for the current platform:

- **Linux 5.11+**: Native rootless OverlayFS (optimal)
- **Linux < 5.11**: FUSE-based OverlayFS fallback (future)
- **macOS**: APFS reflink cloning
- **Other**: Mock provider (testing only)

### Provider Registry Pattern

Providers self-register via `init()` functions to avoid import cycles:

```go
func init() {
    sandbox.RegisterProvider("overlayfs", func() sandbox.Provider {
        return NewProvider()
    })
}
```

## Usage

### Creating a Sandbox

```go
provider, err := sandbox.NewProvider() // Auto-detect platform
if err != nil {
    return err
}

sb, err := provider.Create(ctx, sandbox.SandboxRequest{
    SessionID:    "session-abc123",
    LowerDirs:    []string{"/path/to/repo1", "/path/to/repo2"},
    WorkspaceDir: "/tmp/sandboxes",
    Secrets: map[string]string{
        "ANTHROPIC_API_KEY": "sk-ant-...",
        "GITHUB_TOKEN":      "${GITHUB_TOKEN}", // Env expansion
    },
})

// Agent operates in sb.MergedPath
// All modifications go to sb.UpperPath (isolated)
```

### Cleanup

```go
err := provider.Destroy(ctx, sb.ID)
// Sandbox removed, host repos unchanged
```

## Platform-Specific Behavior

### Linux OverlayFS

**Mount Options**:
- `xino=auto`: Enables inotify propagation (critical for file watchers)
- `lowerdir`: Colon-separated read-only repository paths
- `upperdir`: Session-specific modifications
- `workdir`: OverlayFS internal state
- `merged`: Unified view (where agent operates)

**Permissions**: Rootless mounting (kernel 5.11+, no sudo required)

**Performance**: < 100ms creation time, zero disk I/O overhead

### macOS APFS

**Strategy**: Directory cloning + symlink merging (no native union mounts)

**Current**: Recursive copy (MVP implementation)

**Future**: `syscall.Clonefile()` for true APFS reflinks (zero-copy CoW)

**Performance**: ~2-5s for medium repositories (reflinks will be instant)

## Secrets Management

Secrets written to `upperdir/.env` with:
- **Permissions**: 0600 (owner read/write only)
- **Format**: `KEY=value` (one per line)
- **Expansion**: `${VAR}` syntax supported
- **Isolation**: Never written to lowerdir, cleaned up on destroy

## Error Handling

Structured errors with codes:

```go
type ErrorCode int

const (
    ErrCodeUnknown              ErrorCode = iota
    ErrCodeInvalidRequest       // Invalid input parameters
    ErrCodeUnsupportedPlatform  // Provider not available
    ErrCodeMountFailed          // Filesystem operation failed
    ErrCodeCleanupFailed        // Destroy operation failed
)
```

## Testing

### Contract Tests

All providers must pass contract tests defined in `provider_test.go`:
- Sandbox creation and destruction
- Validation of healthy sandboxes
- Idempotent cleanup
- Error handling for invalid inputs

### Isolation Tests

Destructive isolation verified via:
- `TestDestructiveIsolation`: 100+ iterations of `rm -rf *`
- `TestWhiteoutMechanism`: Character device (0,0) validation
- `TestCopyUpOnWrite`: Modification isolation

### Cross-Platform Tests

Platform-specific tests gated by build tags:
- `//go:build linux`: OverlayFS tests
- `//go:build darwin`: APFS tests

## Configuration

Sandbox settings in AGM config:

```yaml
sandbox:
  enabled: true           # Sandbox-by-default (changed from opt-in)
  provider: "auto"        # auto, overlayfs, apfs, claudecode-worktree, mock
  repos: []               # Additional repositories to merge
  secrets: {}             # Secrets to inject
```

**Note**: As of the better-sandboxing feature, sandboxing is enabled by default.
The `claudecode-worktree` provider is the default for sub-agent execution,
delegating isolation to Claude Code's native worktree support.

## SandboxSpec (Provider-Agnostic Configuration)

`SandboxSpec` decouples sandbox *requirements* from provider *implementation*.
Components like the executor, wayfinder, and AGM compose a `SandboxSpec` to
declare what isolation they need, without knowing which provider will fulfill it.

```go
spec := &SandboxSpec{
    Mode: "worktree",
    Resources: &ResourceSpec{MaxBudgetUSD: 5.0},
    Tools:     &ToolSpec{Preset: "code-only"},
}
provider := NewClaudeCodeProvider(spec)
args := provider.BuildClaudeArgs("/path/to/workdir")
// args: ["--add-dir", "/path/to/workdir", "--max-budget-usd", "5.00"]
```

### Presets

| Preset | Tools | Use Case |
|--------|-------|----------|
| `ReadOnlySpec()` | Read, Grep, Glob, WebSearch, WebFetch | Research, review |
| `CodeOnlySpec()` | Read, Write, Edit, Bash, Grep, Glob | Implementation |
| `FullAccessSpec()` | All | Trusted agents |

### ClaudeCode Provider

The `ClaudeCodeProvider` maps `SandboxSpec` fields to Claude CLI arguments:
- `Filesystem.AllowWrite` -> `--add-dir` flags
- `Resources.MaxBudgetUSD` -> `--max-budget-usd` flag
- `Tools.AllowedTools` -> applied at AGM/executor level (not CLI flags)

## Integration with AGM

### Session Creation

```bash
agm session new my-session --sandbox
# Creates sandbox, starts session in merged directory
```

### Session Cleanup

```bash
agm session kill my-session
# Destroys sandbox automatically (unless --keep-sandbox)
```

## Performance Characteristics

| Operation | OverlayFS | APFS (reflinks) | Mock |
|-----------|-----------|-----------------|------|
| Create    | < 100ms   | < 200ms         | < 1ms |
| Destroy   | < 50ms    | < 100ms         | < 1ms |
| Validate  | < 10ms    | < 10ms          | < 1ms |

## Limitations

### Current

- macOS uses directory copy (slow for large repos)
- No quota enforcement
- No nested sandbox support
- Single workspace directory per sandbox

### Future Enhancements

- APFS reflink implementation (zero-copy)
- Quota limits (disk space, inode count)
- Resource usage metrics
- Automatic pruning of orphan sandboxes

## Security Considerations

1. **Rootless**: No sudo required (kernel 5.11+ on Linux)
2. **Isolation**: Upper directory fully isolated from lowerdir
3. **Secrets**: Strict permissions (0600), cleaned up on destroy
4. **Validation**: Input validation prevents path traversal
5. **Idempotent**: Destroy is safe to call multiple times

## References

- ADR-001: Provider Registry Pattern
- ADR-002: Platform Detection Strategy
- ADR-003: Secrets Injection Design
- `/docs/platform-support.md`: Detailed platform documentation
- `/docs/sandbox-architecture.md`: Architecture deep dive
