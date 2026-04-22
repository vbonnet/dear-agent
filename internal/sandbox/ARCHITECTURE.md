# Sandbox Architecture

## System Overview

The sandbox subsystem provides isolated, copy-on-write filesystem environments for AGM sessions. It abstracts platform-specific isolation technologies (OverlayFS, APFS) behind a unified Provider interface.

```
┌─────────────────────────────────────────────────────────────┐
│                    AGM Session Manager                       │
├─────────────────────────────────────────────────────────────┤
│  cmd/agm/new.go          cmd/agm/kill.go                    │
│    │                        │                                │
│    ├─ provisionSandbox()   ├─ cleanupSessionSandbox()      │
│    │                        │                                │
│    v                        v                                │
├─────────────────────────────────────────────────────────────┤
│              internal/sandbox (Provider Interface)           │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Factory    │  │   Registry   │  │    Types     │     │
│  │              │  │              │  │              │     │
│  │ DetectPlatform() RegisterProvider() SandboxRequest │     │
│  │ NewProvider()│  │              │  │   Sandbox    │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│                   Platform Providers                         │
│                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │  OverlayFS  │  │    APFS     │  │ ClaudeCode  │        │
│  │  (Linux)    │  │   (macOS)   │  │ (Worktree)  │        │
│  │             │  │             │  │             │        │
│  │ Create()    │  │ Create()    │  │ Create()    │        │
│  │ Destroy()   │  │ Destroy()   │  │ Destroy()   │        │
│  │ Validate()  │  │ Validate()  │  │ Validate()  │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
│       │                 │                  │                │
│       v                 v                  v                │
│                                                              │
│  ┌─────────────┐                                            │
│  │    Mock     │  (Testing only)                            │
│  └─────────────┘                                            │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│              Provider-Agnostic Spec Layer                    │
│                                                              │
│  ┌──────────────┐  ┌──────────────────────────────────┐    │
│  │ SandboxSpec  │  │ Presets: ReadOnlySpec()           │    │
│  │              │  │          FullAccessSpec()          │    │
│  │ .Mode        │  │          CodeOnlySpec()           │    │
│  │ .Filesystem  │  └──────────────────────────────────┘    │
│  │ .Network     │                                           │
│  │ .Resources   │  ClaudeCodeProvider.BuildClaudeArgs()     │
│  │ .Tools       │  maps SandboxSpec -> Claude CLI flags     │
│  └──────────────┘                                           │
├─────────────────────────────────────────────────────────────┤
│                  Operating System Layer                      │
│                                                              │
│   Linux Kernel      macOS (Darwin)      Claude Code CLI    │
│   OverlayFS         APFS Filesystem     Native Worktrees   │
└─────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

### Factory (`factory.go`)

**Purpose**: Platform detection and provider instantiation

**Key Functions**:
- `DetectPlatform()`: Inspects kernel version, filesystem capabilities
- `NewProvider()`: Returns best provider for current platform
- `NewProviderForPlatform(name string)`: Creates specific provider

**Platform Detection Logic**:
```go
if runtime.GOOS == "linux" {
    version := parseKernelVersion()
    if isKernelVersionAtLeast(version, 5, 11) {
        return "overlayfs" // Native rootless support
    }
}
if runtime.GOOS == "darwin" {
    return "apfs" // APFS reflink provider
}
return "mock" // Testing fallback
```

### Registry (`providers.go`)

**Purpose**: Decouples provider implementations from factory (avoids import cycles)

**Pattern**: Providers self-register via `init()` functions

**Implementation**:
```go
var (
    providerRegistry = make(map[string]func() Provider)
    registryMu       sync.RWMutex
)

func RegisterProvider(name string, factory func() Provider) {
    registryMu.Lock()
    defer registryMu.Unlock()
    providerRegistry[name] = factory
}
```

**Benefits**:
- No import cycles (factory doesn't import provider packages)
- Extensible (new providers just call RegisterProvider)
- Thread-safe (RWMutex for concurrent access)

### Provider Interface (`provider.go`)

**Purpose**: Unified abstraction for all isolation technologies

**Contract**:
- `Create()`: Provisions isolated filesystem environment
- `Destroy()`: Tears down sandbox, cleans up resources (idempotent)
- `Validate()`: Health check for existing sandbox
- `Name()`: Returns provider identifier

**Design Principles**:
1. **Idempotency**: Destroy can be called multiple times safely
2. **Context-Aware**: All operations accept `context.Context` for cancellation
3. **Structured Errors**: Return `*Error` with codes for programmatic handling
4. **Stateless**: Providers don't maintain internal state

## Provider Implementations

### OverlayFS Provider (`overlayfs/provider.go`)

**Linux-specific** (kernel 5.11+, rootless mounting)

**Directory Structure**:
```
workspaceDir/
└── sessionID/
    ├── lowerdir/  (symlinks to read-only repos)
    ├── upperdir/  (session modifications)
    ├── workdir/   (OverlayFS internal state)
    └── merged/    (unified view - where agent operates)
```

**Mount Command**:
```bash
mount -t overlay overlay \
  -o lowerdir=/repo1:/repo2,\
     upperdir=/tmp/sandbox/session/upperdir,\
     workdir=/tmp/sandbox/session/workdir,\
     xino=auto \
  /tmp/sandbox/session/merged
```

**Critical Mount Option**:
- `xino=auto`: Enables inotify event propagation
  - Without this, file watchers (LSP, hot reload) don't receive events
  - Must be set for proper developer experience

**Multi-Repository Merging**:
- Colon-separated `lowerdir` values: `/repo1:/repo2:/repo3`
- Repositories overlay in order (left = highest priority)
- Modifications always go to `upperdir` (isolated)

### APFS Provider (`apfs/provider.go`)

**macOS-specific** (Darwin build tag)

**Current Implementation** (MVP):
```
workspaceDir/
└── sessionID/
    ├── lower_clones/
    │   ├── repo1/  (directory copy)
    │   └── repo2/  (directory copy)
    └── merged -> lower_clones/repo1  (symlink)
```

**Limitations**:
- Uses recursive copy (slow for large repos)
- No true union mount (macOS lacks equivalent of OverlayFS)

**Future Implementation**:
```go
// Use APFS reflinks for instant zero-copy cloning
syscall.Clonefile(src, dst, syscall.CLONE_NOFOLLOW)
```

### ClaudeCode Provider (`claudecode_provider.go`)

**Purpose**: Delegate isolation to Claude Code's native worktree capability

Instead of creating OverlayFS/APFS mounts, this provider relies on Claude
Code's built-in `isolation: "worktree"` mode. It maps the provider-agnostic
`SandboxSpec` fields to Claude CLI arguments (`--add-dir`, `--max-budget-usd`).

**Key Methods**:
- `BuildClaudeArgs(workDir)`: Generates CLI flags from SandboxSpec
- `AllowedTools()`: Returns tool restrictions (empty = all allowed)
- `ToolPreset()`: Returns preset name ("read-only", "code-only", "full")

**Registration**: Self-registers as `"claudecode-worktree"` via `init()`

**When Used**: Default provider for sub-agent execution through the Engram
executor plugin, where each sub-agent gets its own Claude Code worktree.

### SandboxSpec (`spec.go`)

**Purpose**: Provider-agnostic sandbox configuration

`SandboxSpec` is a declarative configuration that other components (executor,
wayfinder, AGM) compose to request isolation. It separates *what* isolation
is needed from *how* it is implemented (provider selection).

**Presets**:
- `ReadOnlySpec()`: Research/review -- only Read, Grep, Glob, WebSearch, WebFetch
- `FullAccessSpec()`: Trusted agents -- all tools allowed
- `CodeOnlySpec()`: Code editing -- Read, Write, Edit, Bash, Grep, Glob (no network)

**Fields**: Mode, Filesystem (AllowWrite/DenyRead), Network (AllowedDomains),
Resources (TimeoutSeconds, MaxBudgetUSD), Tools (AllowedTools, Preset)

### Mock Provider (`mock_provider.go`)

**Purpose**: Testing without platform dependencies

**Behavior**:
- Creates in-memory sandbox metadata
- No actual filesystem operations
- Configurable delays, error injection
- Used in unit tests and CI environments

**Example**:
```go
mock := sandbox.NewMockProvider()
mock.InjectError("create", errors.New("mount failed"))
_, err := mock.Create(ctx, req) // Returns injected error
```

## Data Flow

### Sandbox Creation Flow

```
1. AGM Session Creation
   ├─ agm session new --sandbox
   │
2. Provision Sandbox
   ├─ shouldEnableSandbox() → Check flags/config
   ├─ sandbox.NewProvider() → Auto-detect platform
   │   ├─ DetectPlatform()
   │   └─ NewProviderForPlatform("overlayfs")
   │
3. Create Sandbox Environment
   ├─ provider.Create(ctx, SandboxRequest{...})
   │   ├─ Validate request parameters
   │   ├─ Create directory structure
   │   ├─ Mount filesystem (OverlayFS) or clone (APFS)
   │   ├─ Write secrets to upperdir/.env
   │   └─ Return Sandbox metadata
   │
4. Update Session Context
   ├─ Update workDir = sandbox.MergedPath
   ├─ Store sandbox metadata in manifest
   └─ Start tmux session in sandbox environment
```

### Sandbox Cleanup Flow

```
1. AGM Session Kill
   ├─ agm session kill my-session
   │
2. Terminate Tmux Session
   ├─ killTmuxSession()
   │
3. Cleanup Sandbox
   ├─ cleanupSessionSandbox()
   │   ├─ Check if sandbox enabled in manifest
   │   ├─ Respect --keep-sandbox flag
   │   ├─ Get provider from manifest
   │   └─ provider.Destroy(ctx, sandboxID)
   │       ├─ Unmount filesystem (OverlayFS)
   │       ├─ Remove directory structure
   │       └─ Idempotent (safe if already destroyed)
   │
4. Verify Cleanup
   ├─ Sandbox directory removed
   ├─ Lowerdir repos unchanged
   └─ Secrets removed (not in lowerdir)
```

## Thread Safety

### Provider Registry

- **RWMutex**: Protects concurrent registration and lookup
- **Init Safety**: Providers register in `init()` before `main()`
- **Read-Heavy**: Multiple goroutines can read registry concurrently

### Provider Instances

- **Stateless**: No shared mutable state between operations
- **Concurrent Creates**: Multiple sandboxes can be created in parallel
- **Context Cancellation**: Respects `ctx.Done()` for graceful shutdown

## Error Handling

### Structured Errors

```go
type Error struct {
    Code       ErrorCode
    Message    string
    Underlying error
}
```

**Error Codes**:
- `ErrCodeInvalidRequest`: Bad input (400-class)
- `ErrCodeUnsupportedPlatform`: Provider unavailable (501)
- `ErrCodeMountFailed`: Filesystem operation failed (500)
- `ErrCodeCleanupFailed`: Destroy failed (partial cleanup possible)

### Error Propagation

```go
// Provider returns structured error
if err := provider.Create(ctx, req); err != nil {
    var sbErr *sandbox.Error
    if errors.As(err, &sbErr) {
        switch sbErr.Code {
        case sandbox.ErrCodeInvalidRequest:
            // User error - show clear message
        case sandbox.ErrCodeMountFailed:
            // System error - suggest troubleshooting
        }
    }
}
```

## Performance Characteristics

### OverlayFS (Linux)

- **Create**: < 100ms (mount syscall)
- **Read**: Zero overhead (kernel-level union)
- **Write**: Copy-on-write (first modification of file)
- **Destroy**: < 50ms (unmount + directory removal)

### APFS (macOS, Current)

- **Create**: 2-5s (recursive copy)
- **Read**: Native filesystem performance
- **Write**: Native filesystem performance
- **Destroy**: < 100ms (directory removal)

### APFS (macOS, Future with Reflinks)

- **Create**: < 200ms (instant cloning)
- **Read**: Zero overhead (shared extents)
- **Write**: Copy-on-write (APFS handles)
- **Destroy**: < 100ms (reference counting)

## Scalability

### Concurrent Sandboxes

- **Linux**: 50+ sandboxes tested successfully
- **Bottleneck**: Inode limits (`fs.inotify.max_user_watches`)
- **Solution**: Monitor inode usage, warn at 80%

### Resource Usage

- **Disk Space**: Only modified files consume space (CoW)
- **Memory**: Minimal (metadata only, no page cache duplication)
- **Inodes**: Each file in upperdir consumes one inode

## Testing Strategy

### Unit Tests

- **Mock Provider**: Test logic without filesystem dependencies
- **Contract Tests**: Verify all providers implement interface correctly
- **Error Injection**: Test error handling paths

### Integration Tests

- **Platform-Gated**: `//go:build linux` and `//go:build darwin`
- **Real Filesystem**: Actual mount/unmount operations
- **Isolation Verification**: Destructive operations don't affect host

### Benchmarks

- **BenchmarkSandboxCreation**: Measure creation overhead
- **BenchmarkConcurrentSandboxes**: Parallel creation stress test
- **BenchmarkSandboxValidation**: Health check performance

## Security Model

### Principle: Defense in Depth

1. **Input Validation**: All paths validated against traversal attacks
2. **Rootless**: No sudo required (kernel 5.11+ on Linux)
3. **Isolation**: Upperdir fully separated from lowerdir
4. **Secrets**: Strict permissions (0600), automatic cleanup
5. **Idempotency**: Safe to call Destroy multiple times

### Attack Surface

**Minimized**:
- No network exposure
- No privileged operations
- No external dependencies (native kernel features)

**Risks**:
- Kernel vulnerabilities in OverlayFS (mitigated by kernel version check)
- Path traversal (mitigated by input validation)
- Inode exhaustion (mitigated by monitoring)

## Future Enhancements

### Phase 2 (Scale & Hardening)

- [ ] Quota enforcement (disk space limits)
- [ ] Inotify limit detection and warnings
- [ ] Automatic orphan sandbox pruning
- [ ] Resource usage metrics

### Phase 3 (Advanced Features)

- [ ] Nested sandbox support
- [ ] Snapshot/restore functionality
- [ ] Network isolation (optional)
- [ ] GPU passthrough (for ML workloads)

## References

- **OverlayFS Kernel Docs**: https://www.kernel.org/doc/html/latest/filesystems/overlayfs.html
- **APFS Reference**: https://developer.apple.com/documentation/foundation/filemanager
- **Provider Registry Pattern**: ADR-001
- **Platform Detection**: ADR-002
