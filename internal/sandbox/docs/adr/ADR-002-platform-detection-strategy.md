# ADR-002: Platform Detection Strategy

**Status**: Accepted
**Date**: 2026-03-20
**Authors**: Claude Sonnet 4.5
**Context**: Phase 1 - Core Implementation

## Context

The sandbox system must automatically select the best isolation technology for the current platform without requiring user configuration. Different platforms have different capabilities:

- **Linux 5.11+**: Native rootless OverlayFS
- **Linux < 5.11**: No rootless OverlayFS (requires FUSE fallback)
- **macOS**: APFS with reflink cloning
- **Other**: No native isolation (testing only)

## Decision

Implement **automatic platform detection** in `DetectPlatform()` that inspects kernel version, filesystem capabilities, and OS type to determine the recommended provider.

### Detection Algorithm

```go
func DetectPlatform() (*PlatformInfo, error) {
    info := &PlatformInfo{OS: runtime.GOOS}

    switch runtime.GOOS {
    case "linux":
        // Parse kernel version from /proc/version
        version := parseKernelVersion()
        info.KernelVersion = version

        // Check if OverlayFS supported (kernel 5.11+)
        if isKernelVersionAtLeast(version, 5, 11) {
            info.HasOverlayFS = true
            info.Recommended = "overlayfs"
        } else {
            info.Recommended = "fuse-overlayfs" // Fallback
        }

    case "darwin":
        // macOS always has APFS (10.13+)
        info.HasAPFS = true
        info.Recommended = "apfs"

    default:
        info.Recommended = "mock" // Testing only
    }

    return info, nil
}
```

### Kernel Version Parsing

**Source**: `/proc/version`

**Format**: `Linux version X.Y.Z...`

**Parsing Logic**:
```go
func parseKernelVersion() string {
    data, _ := os.ReadFile("/proc/version")
    // Extract version after "Linux version "
    // Handle variants: "5.11.0-ubuntu", "6.6.123+", etc.
    return cleanVersion // "X.Y.Z"
}

func isKernelVersionAtLeast(version string, major, minor int) bool {
    parts := strings.Split(version, ".")
    if len(parts) < 2 {
        return false
    }

    vmajor, _ := strconv.Atoi(parts[0])
    vminor, _ := strconv.Atoi(parts[1])

    if vmajor > major {
        return true
    }
    if vmajor == major && vminor >= minor {
        return true
    }
    return false
}
```

## Consequences

### Positive

1. **Zero Configuration**: Users don't specify provider manually
2. **Optimal Selection**: Always chooses best available technology
3. **Forward Compatible**: New kernel versions automatically use OverlayFS
4. **Graceful Degradation**: Falls back to less optimal providers when needed
5. **Testable**: Detection logic isolated, easy to unit test

### Negative

1. **Runtime Detection**: Capability checks happen at runtime (not compile-time)
2. **Kernel Version Parsing**: Fragile if `/proc/version` format changes
3. **APFS Assumption**: Assumes all macOS >= 10.13 has APFS (mostly true)
4. **No Capability Testing**: Doesn't verify mount permissions

### Mitigations

- **Robust Parsing**: Handle multiple kernel version formats
- **Fallback Chain**: If preferred provider fails, try next best
- **Validation**: Test actual provider creation, not just detection
- **User Override**: Allow `--sandbox-provider` flag to force specific provider

## Alternatives Considered

### 1. Compile-Time Detection (Build Tags)

```go
//go:build linux

func NewProvider() Provider {
    return NewOverlayFSProvider()
}
```

**Rejected**: Can't distinguish kernel versions at compile-time

### 2. Configuration File

```yaml
sandbox:
  provider: overlayfs
```

**Rejected**: Requires manual configuration, error-prone

### 3. Try-and-Fallback

```go
providers := []string{"overlayfs", "fuse-overlayfs", "mock"}
for _, name := range providers {
    if p, err := NewProviderForPlatform(name); err == nil {
        return p
    }
}
```

**Rejected**: Expensive (attempts mount operations), slow startup

## Platform-Specific Considerations

### Linux

**Kernel 5.11+ Detection Rationale**:
- Rootless OverlayFS support added in Linux 5.11
- Before 5.11: Requires sudo or user namespaces
- After 5.11: Works out-of-the-box for unprivileged users

**Version Check Sources**:
- `/proc/version`: Most reliable
- `uname -r`: Alternative fallback
- `/proc/sys/kernel/osrelease`: Backup option

### macOS

**APFS Detection**:
- Introduced in macOS High Sierra (10.13, released 2017)
- All modern macs have APFS
- No reliable programmatic detection needed (assume present)

**Future**: Could check filesystem type via `statfs()` syscall

### Other Platforms

**Mock Provider**:
- Used for Windows, BSD, or unknown platforms
- Allows testing without real isolation
- Returns errors if used in production

## Testing Strategy

### Unit Tests

```go
func TestDetectPlatform(t *testing.T) {
    info, err := DetectPlatform()
    require.NoError(t, err)

    switch runtime.GOOS {
    case "linux":
        assert.NotEmpty(t, info.KernelVersion)
        // On CI (kernel 6.6.123+), should have OverlayFS
    case "darwin":
        assert.True(t, info.HasAPFS)
        assert.Equal(t, "apfs", info.Recommended)
    }
}
```

### Integration Tests

```go
func TestProviderCreation(t *testing.T) {
    info, _ := DetectPlatform()

    // Should successfully create recommended provider
    provider, err := NewProviderForPlatform(info.Recommended)
    require.NoError(t, err)
    assert.NotNil(t, provider)
}
```

## User Override

**Flag**:
```bash
agm session new --sandbox-provider=mock
```

**Config**:
```yaml
sandbox:
  provider: mock # Override auto-detection
```

**Use Cases**:
- Testing specific provider
- Forcing fallback for debugging
- Compatibility with older systems

## Future Enhancements

### Capability Testing

Instead of version checks, test actual capabilities:

```go
func hasOverlayFSSupport() bool {
    // Try to mount in temp directory
    // If successful, OverlayFS is supported
    // Unmount and return true
}
```

**Benefit**: More reliable than version parsing
**Cost**: Slower startup (requires mount operation)

### Dynamic Fallback

```go
func getBestProvider() Provider {
    candidates := []string{
        DetectPlatform().Recommended,
        "fuse-overlayfs",
        "mock",
    }

    for _, name := range candidates {
        if p, err := TryCreateProvider(name); err == nil {
            return p
        }
    }
}
```

## Related Decisions

- ADR-001: Provider Registry Pattern (used by detection)
- ADR-003: Secrets Injection Design (platform-agnostic)

## References

- **Linux Kernel Docs**: https://www.kernel.org/doc/html/latest/filesystems/overlayfs.html
- **OverlayFS Rootless**: https://lwn.net/Articles/842096/
- **APFS Overview**: https://developer.apple.com/documentation/foundation/filemanager
- **Implementation**: `internal/sandbox/factory.go`
