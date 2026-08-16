# Platform Support

## Overview

The AGM sandbox system provides platform-specific implementations for creating isolated filesystem environments. Each provider is optimized for its target platform's native capabilities.

The authoritative subsystem reference is
[`internal/sandbox/ARCHITECTURE.md`](../internal/sandbox/ARCHITECTURE.md); this
page covers per-platform setup and troubleshooting. Registered providers are
`bubblewrap`, `overlayfs` (also `overlayfs-native`), `gvisor`, and `apfs`, plus
the built-in `mock` provider for tests.

## Linux

### Requirements

**Option 1: Bubblewrap (preferred by automatic selection when installed)**
- `bwrap` binary available (`sudo apt-get install bubblewrap` on Debian/Ubuntu)
- Works regardless of kernel OverlayFS support

**Option 2: Native OverlayFS**
- Linux kernel 5.11+ for rootless operation
- No additional packages required

**Option 3: gVisor**
- Requires `runsc`; workspace materialization only — command wrapping inside
  the OCI bundle is the caller's responsibility

There is no registered `fuse-overlayfs` provider: on a kernel older than 5.11
with no `bwrap` installed, automatic selection returns an unsupported-provider
error rather than falling back.

### Performance

**Native OverlayFS Benefits:**
- Minimal overhead: Near-native filesystem performance
- Rootless operation: No sudo required
- Kernel-level copy-on-write: Instant file cloning
- inotify propagation: Full file watching support

### Testing

```bash
# Run all tests
cd .
go test ./internal/sandbox/... -v

# Run integration tests (requires root for mount operations)
go test ./internal/sandbox/... -tags=integration -v

# Run benchmarks
go test ./internal/sandbox/... -bench=. -benchmem
```

### Platform Detection

```go
info, err := sandbox.DetectPlatform()
// info.OS = "linux"
// info.KernelVersion = "6.6.123"
// info.HasOverlayFS = true (if kernel >= 5.11)
// info.Recommended = "bubblewrap" when bwrap is installed,
//                    otherwise "overlayfs" (kernel 5.11+)
```

### Troubleshooting

**Error: "kernel version too old"**
- Your kernel is < 5.11
- Solution: Install bubblewrap (`bwrap`) and use the "bubblewrap" provider
- Or: Upgrade to a newer kernel

**Error: "permission denied" during mount**
- Rootless OverlayFS requires kernel 5.11+
- Check kernel version: `uname -r`
- Verify user namespaces enabled: `cat /proc/sys/kernel/unprivileged_userns_clone`

**Error: "mount failed"**
- Check dmesg for kernel errors: `dmesg | tail -20`
- Verify workspace directory is writable
- Ensure lower directories exist and are readable

## macOS

### Requirements

**APFS Reflink Support**
- macOS 10.13+ (High Sierra or later)
- APFS filesystem (default on modern macOS)
- `clonefile` system call support

### Performance

| Operation | Time | Notes |
|-----------|------|-------|
| Sandbox Creation | 50-200ms | Fast for large repos (reflink cloning) |
| File Copy-on-Write | <1ms | Instant until modification |
| I/O Overhead | <2% | Native APFS performance |

**APFS Benefits:**
- Fast cloning: Entire repositories cloned instantly using reflinks
- Space efficient: Shared blocks until modification
- Copy-on-write: Automatic at filesystem level
- No union mounts: Simpler, more reliable architecture

**Limitations:**
- No union mount overlay: Files are actually cloned, not layered
- Modifications require actual copies (still fast with COW)
- Requires APFS filesystem (not HFS+)

### Testing

```bash
# Run all tests
cd .
go test ./internal/sandbox/... -v

# Run integration tests
go test ./internal/sandbox/... -tags=integration -v

# Run APFS-specific tests
go test ./internal/sandbox/apfs/... -v
```

### Platform Detection

```go
info, err := sandbox.DetectPlatform()
// info.OS = "darwin"
// info.HasAPFS = true
// info.Recommended = "apfs"
```

### Troubleshooting

**Error: "APFS not supported"**
- Verify filesystem type: `diskutil info / | grep "Type (Bundle)"`
- Should show "apfs"
- Solution: Reformat volume as APFS or use different volume

**Error: "clonefile failed"**
- Source or destination may be on different volumes
- APFS reflinks require same volume
- Check with: `df -h`

**Error: "operation not supported"**
- macOS version may be too old (< 10.13)
- Check version: `sw_vers`
- Solution: Upgrade macOS, or use the mock provider for development

## Windows

**Status:** Not yet implemented

Future provider options:
- WSL2 with Linux OverlayFS
- Windows Container Storage Interface (CSI)
- Filesystem filters (minifilter drivers)

## Cross-Platform Testing

### Test Suite

The test suite is designed to work across all platforms:

```bash
# Platform detection tests (work everywhere)
go test ./internal/sandbox/... -run TestPlatformDetection -v

# Provider availability tests (platform-specific)
go test ./internal/sandbox/... -run TestProviderAvailability -v

# Benchmarks (use mock provider if real provider unavailable)
go test ./internal/sandbox/... -bench=. -benchmem
```

### Mock Provider

For development and testing on unsupported platforms:

```go
provider := sandbox.NewMockProvider()
// Simulates provider behavior without actually creating sandboxes
```

## Performance Benchmarks

Run comprehensive benchmarks:

```bash
go test ./internal/sandbox/... -bench=. -benchmem -benchtime=10s

# Expected results (approximate):
# BenchmarkSandboxCreation-8           100    100-700ms/op
# BenchmarkConcurrentSandboxes-8       200     50-300ms/op
# BenchmarkSandboxValidation-8      100000      0.1-1ms/op
```

## Choosing a Provider

### Automatic Selection (Recommended)

```go
provider, err := sandbox.NewProvider()
// Automatically detects platform and selects best provider
```

### Manual Selection

```go
// Linux with bubblewrap
provider, err := sandbox.NewProviderForPlatform("bubblewrap")

// Linux with OverlayFS
provider, err := sandbox.NewProviderForPlatform("overlayfs")

// Linux with gVisor (requires runsc)
provider, err := sandbox.NewProviderForPlatform("gvisor")

// macOS with APFS
provider, err := sandbox.NewProviderForPlatform("apfs")

// Testing/development
provider := sandbox.NewMockProvider()
```

Any other name returns a "provider not available" error.

## Platform-Specific Behavior

### File Watching (inotify/fsevents)

**Linux OverlayFS:**
- Full inotify support with `xino=auto` mount option
- File changes propagate to host watchers
- LSP servers work correctly

**macOS APFS:**
- Native fsevents support
- File changes tracked by macOS kernel
- Full compatibility with file watchers

### Symlinks

**Both platforms:**
- Symlinks preserved in sandboxes
- Relative symlinks work correctly
- Absolute symlinks point to original targets

### Permissions

**Linux:**
- Preserves Unix permissions
- User/group ownership maintained
- ACLs supported

**macOS:**
- Preserves Unix permissions
- Extended attributes preserved
- ACLs supported through APFS

## Related Documentation

- [Sandbox architecture](../internal/sandbox/ARCHITECTURE.md)
- [Sandbox SPEC](../internal/sandbox/SPEC.md)
