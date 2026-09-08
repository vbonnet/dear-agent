# Sandbox Provider Specification

## Executable EARS Requirements

**SNDBR-01** When a sandbox is created, the provider shall isolate writes from the source workspace and preserve declared read-only inputs.

**SNDBR-02** When sandbox cleanup runs, the provider shall remove only resources owned by that sandbox instance.

**SNDBR-03** When Git worktree removal succeeds but subsequent sandbox directory cleanup fails, the system shall resume a retry at directory cleanup without repeating the completed Git removal phase.

**SNDBR-04** When a sandbox request names a working directory inside a configured lower directory, the provider shall return the corresponding isolated directory while preserving its repository-relative path.

**SNDBR-05** If a requested working directory is outside every configured lower directory, the provider shall fail before materializing a workspace rather than launch from an unrelated repository.

**SNDBR-06** If an explicitly selected sandbox provider cannot materialize a provider-owned isolated workspace, the system shall reject it as unavailable before creating any sandbox directory.

**SNDBR-07** When a process monitor starts, the system shall publish running state, cancellation, and completion tracking atomically before the monitor loop begins.

**SNDBR-08** When a process monitor is stopped, the system shall cancel the active loop, wait for it to exit, and clear active lifecycle state before returning.

**SNDBR-09** When a process monitor parent context is canceled, the system shall clear active lifecycle state so the monitor can be started again.

**SNDBR-10** When a process monitor starts after a prior run, the system shall reset the process-count baseline before sampling descendant counts.

**SNDBR-11** When a process monitor emits an alert, the system shall invoke the external alert callback without blocking the monitor loop on callback-owned lifecycle actions.

**SNDBR-12** While a process monitor alert callback is still running, the system shall suppress duplicate alert callback launches and allow a later callback after the in-flight callback completes.

**SNDBR-13** While a process monitor alert callback from a prior monitor run is still running, the system shall keep restart attempts on the prior lifecycle until that callback drains.

**SNDBR-14** When a process monitor loop exits without an active alert callback, the system shall publish stopped lifecycle state before unblocking stop callers so an immediate restart creates a fresh lifecycle.

**SNDBR-15** Where sandbox orphan cleanup runs on a platform other than Linux or Darwin, the system shall report an unsupported-platform failure before mount discovery, unmount, retry, or directory removal.

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`
- Feature: `agm/test/bdd/features/sandbox_provider_guardrails.feature`

<!-- Last audited at: 2026-07-20 -->

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

- **Linux with `bwrap`**: Bubblewrap
- **Linux without `bwrap`, kernel 5.11+**: Native rootless OverlayFS
- **Older Linux**: Unsupported `fuse-overlayfs` recommendation
- **macOS**: APFS directory cloning
- **Other**: Unsupported fallback recommendation

Automatic selection never substitutes the test-only mock provider. Unsupported
recommendations fail closed so callers cannot mistake an empty directory for an
isolated workspace.

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
    WorkingDir:   "/path/to/repo2/agm",
    WorkspaceDir: "/tmp/sandboxes",
    Secrets: map[string]string{
        "ANTHROPIC_API_KEY": "sk-ant-...",
        "GITHUB_TOKEN":      "${GITHUB_TOKEN}", // Env expansion
    },
})

// Harness operates in sb.WorkingDir; sb.MergedPath remains the workspace root.
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
  provider: "auto"        # auto, bubblewrap, overlayfs, gvisor, apfs, mock
  repos: []               # Additional repositories to merge
  secrets: {}             # Secrets to inject
```

**Note**: As of the better-sandboxing feature, sandboxing is enabled by default.
The retired `claudecode-worktree` name is deliberately unavailable: AGM cannot
delegate workspace creation to a harness after it has already chosen the
sandbox working directory. Harness-specific tool permissions remain a harness
launch concern and are not represented as filesystem isolation.

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
