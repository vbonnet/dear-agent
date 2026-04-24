# AGM Sandbox Migration Guide

**Version:** AGM with Sandbox Support (Phase 4)
**Date:** March 2026
**Applies to:** Users upgrading from AGM without sandbox support

---

## Overview

### What's New in This Version

AGM now includes **optional sandbox isolation** for agent sessions, providing:

- **Filesystem isolation**: Agents operate in isolated copy-on-write sandboxes
- **Host protection**: Destructive operations (e.g., `rm -rf *`) cannot affect your host filesystem
- **Multi-repository support**: Merge multiple repositories into a single sandbox workspace
- **Secure secrets injection**: Inject credentials into sandbox environment with strict permissions
- **Zero-copy performance**: OverlayFS (Linux) or APFS cloning (macOS) for instant sandbox creation

### Why Upgrade

**Enhanced Safety:**
- Protect your host filesystem from accidental destructive operations
- Test risky commands without fear of data loss
- Experiment with confidence in isolated environments

**Improved Workflows:**
- Multi-repository workspaces (merge multiple repos into one view)
- Secure credential management (secrets injected with 0600 permissions)
- Clean separation between sandbox and host state

**Performance:**
- OverlayFS: <100ms sandbox creation, zero disk I/O overhead
- APFS: ~2-5s for medium repos (instant with future reflink support)

### Backward Compatibility Guarantee

**This upgrade is 100% backward compatible:**

- ✅ Sandboxing is **opt-in** via `--sandbox` flag
- ✅ Existing sessions work unchanged (no migration required)
- ✅ All existing commands function identically without `--sandbox`
- ✅ No breaking changes to manifest format or configuration
- ✅ Existing workflows continue as before

**Default behavior:** Sessions run on host filesystem (same as before)

---

## Migration Steps

### Step 1: Check System Requirements

**Linux users:**

```bash
# Check kernel version (need 5.11+ for rootless OverlayFS)
uname -r

# Verify OverlayFS support
cat /proc/filesystems | grep overlay

# If not found, load the module
sudo modprobe overlay
```

**macOS users:**

```bash
# Check macOS version
sw_vers

# Verify APFS filesystem
df -T .
```

**Minimum requirements:**
- Linux: Kernel 5.11+ (for rootless operation) or 5.0+ with sudo
- macOS: APFS filesystem (standard on modern macOS)
- Filesystem: ext4, xfs, or btrfs (Linux) / APFS (macOS)

### Step 2: Install/Upgrade AGM

```bash
# Upgrade to latest version with sandbox support
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest

# Verify installation
agm --version
```

### Step 3: Verify Sandbox Support

```bash
# Check if sandbox provider is available
agm doctor --validate

# Expected output includes:
# ✓ Sandbox provider: overlayfs (Linux)
# or
# ✓ Sandbox provider: apfs (macOS)
```

### Step 4: Test Sandbox Creation (Optional)

Create a test session to verify sandbox functionality:

```bash
# Create sandbox-enabled session
agm new test-sandbox --sandbox --test

# Verify sandbox isolation
agm resume test-sandbox
# Try: ls / (should see merged sandbox view)
# Try: touch /sandbox-test-file (modifications isolated)

# Cleanup test session
agm kill test-sandbox
agm delete test-sandbox
```

### Step 5: Update Configuration (Optional)

Enable sandbox by default for new sessions (optional):

```yaml
# ~/.config/agm/config.yaml
sandbox:
  enabled: false          # Keep false to maintain current behavior
  provider: "auto"        # auto-detect best provider
  repos: []               # Additional repos to merge (optional)
  secrets: {}             # Secrets to inject (optional)
```

**Note:** Leave `enabled: false` to maintain existing behavior. Use `--sandbox` flag when needed.

---

## Breaking Changes

**None.** This is a fully backward-compatible release.

- No changes to existing session manifests
- No changes to command syntax (except new `--sandbox` flag)
- No changes to default behavior
- No data migration required

---

## Rollback Procedures

If you encounter issues with the sandbox feature, rollback is simple.

### Option 1: Don't Use Sandbox (Recommended)

Simply don't use the `--sandbox` flag. Your existing workflows continue unchanged.

```bash
# Works exactly as before
agm new my-session
agm resume my-session
```

### Option 2: Downgrade AGM

If you need to downgrade to the previous version:

```bash
# Install previous version (replace with actual version)
go install github.com/vbonnet/dear-agent/agm/cmd/agm@v2.x.x

# Verify downgrade
agm --version
```

**Safe rollback:** No data loss. Session manifests remain compatible.

### Option 3: Manual Sandbox Cleanup

If sandbox sessions are stuck:

```bash
# List active overlay mounts
mount | grep overlay

# Unmount manually if needed
umount /path/to/sandbox/merged

# Remove sandbox directories
rm -rf /path/to/workspace/sandbox-*

# Verify cleanup
mount | grep overlay  # Should be empty
```

---

## Feature Comparison

### Old vs New Capabilities

| Feature | Without Sandbox (Before) | With Sandbox (New) |
|---------|-------------------------|-------------------|
| **Session isolation** | None (runs on host) | Full filesystem isolation |
| **Destructive operations** | Affects host filesystem | Isolated to sandbox |
| **Multi-repo workspaces** | Not supported | Merge multiple repos |
| **Secrets management** | Manual environment vars | Secure injection (0600) |
| **Performance** | Native filesystem | OverlayFS: <100ms overhead |
| **Repository safety** | Manual backups needed | Host repos always safe |
| **Activation** | Default | Opt-in via `--sandbox` |

### What Stays the Same

- ✅ Session creation and management commands
- ✅ Multi-agent support (Claude, Gemini, Codex, OpenCode)
- ✅ UUID auto-detection and association
- ✅ Interactive TUI and pickers
- ✅ Workspace management
- ✅ Session archiving and cleanup
- ✅ All existing flags and options

### What Improves

- ➕ Optional sandbox isolation (new `--sandbox` flag)
- ➕ Multi-repository merging (new `--repos` option)
- ➕ Secure secrets injection (new `--secrets` option)
- ➕ Enhanced safety for experimental sessions
- ➕ Clean separation of sandbox vs host state

---

## FAQ

### Q: Do I need to migrate my existing sessions?

**A: No.** Existing sessions continue to work unchanged. Sandboxing is opt-in for new sessions only.

### Q: Will my existing workflows break?

**A: No.** Default behavior is unchanged. Use `--sandbox` flag to enable isolation.

### Q: How do I enable sandbox for a new session?

```bash
# Create sandbox-enabled session
agm new my-session --sandbox

# With additional repositories
agm new my-session --sandbox --repos /path/to/repo1,/path/to/repo2

# With secrets injection
agm new my-session --sandbox --secrets ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
```

### Q: Can I convert an existing session to use sandbox?

**A: No.** Sandbox must be enabled at session creation. Create a new session with `--sandbox` flag.

### Q: What happens if sandbox creation fails?

**A:** AGM falls back to non-sandboxed session with a warning. Your workflow continues normally.

### Q: Do I need root/sudo for sandboxing?

**A: Not on modern systems.**
- Linux kernel 5.11+: Rootless OverlayFS (no sudo needed)
- Linux kernel <5.11: Requires sudo for mount operations
- macOS: No sudo required (APFS cloning)

### Q: How do I know if my session is sandboxed?

```bash
# Check session details
agm list --all

# Sandboxed sessions show: sandbox: enabled
```

### Q: Where are sandbox files stored?

- Workspace directory: `/tmp/agm-sandboxes/` (default)
- Upper layer (modifications): `<workspace>/<session-id>/upper/`
- Merged view: `<workspace>/<session-id>/merged/`
- Lower layers (read-only): Original repository paths

### Q: How do I cleanup orphaned sandboxes?

```bash
# AGM auto-cleans on session kill
agm kill my-session  # Destroys sandbox automatically

# Manual cleanup if needed
agm doctor --validate --fix

# Check for orphaned mounts
mount | grep overlay
```

### Q: What if I'm on an older Linux kernel?

**Option 1:** Upgrade kernel to 5.11+ for rootless operation
```bash
# Ubuntu/Debian
sudo apt update && sudo apt upgrade linux-generic

# Check new version
uname -r
```

**Option 2:** Use sudo for sandbox operations (kernel <5.11)

**Option 3:** Don't use sandbox (keep existing workflow)

### Q: Does sandbox affect performance?

**A: Minimal impact.**
- OverlayFS: <100ms creation, zero I/O overhead
- APFS: ~2-5s creation for medium repos
- No runtime performance penalty

### Q: Can I use sandbox with test sessions?

```bash
# Yes! Recommended for extra safety
agm new experiment --sandbox --test
```

### Q: What errors should I watch for?

See `./docs/ERROR_GUIDE.md` for comprehensive troubleshooting:

- `ErrCodeKernelTooOld`: Upgrade kernel to 5.11+
- `ErrCodeMountFailed`: Check OverlayFS module loaded
- `ErrCodePermissionDenied`: Use sudo (kernel <5.11) or check SELinux
- `ErrCodeUnsupportedPlatform`: Verify Linux/macOS platform

---

## Getting Help

**Documentation:**
- Sandbox Architecture: `./internal/sandbox/SPEC.md`
- Error Recovery: `./docs/ERROR_GUIDE.md`
- User Guide: `./docs/USER_GUIDE.md` (once created)
- Platform Support: `./docs/platform-support.md`

**Health Check:**
```bash
agm doctor --validate
```

**Community:**
- File issues: GitHub repository
- Check logs: `journalctl -xe` (Linux) or system logs (macOS)

---

## Best Practices

1. **Use sandbox for experimental work**: Enable `--sandbox` when testing risky operations
2. **Keep existing workflows as-is**: No need to change working sessions
3. **Test before production**: Try `--test --sandbox` for experiments
4. **Monitor system resources**: Check `df -h` and `mount | wc -l` periodically
5. **Cleanup regularly**: Use `agm clean` to remove old sandboxes
6. **Update kernel**: Linux users should upgrade to 5.11+ for best experience
7. **Verify health**: Run `agm doctor --validate` after upgrade

---

## Next Steps

1. **Read the Sandbox Spec**: See `./internal/sandbox/SPEC.md`
2. **Try a test session**: `agm new test --sandbox --test`
3. **Check error guide**: Familiarize with `./docs/ERROR_GUIDE.md`
4. **Keep using AGM normally**: Your existing workflows continue unchanged

**Remember:** Sandboxing is **opt-in**. You control when and where to use it.
