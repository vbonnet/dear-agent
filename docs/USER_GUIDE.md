# AGM Sandbox User Guide

## Overview

AGM (Anthropic's Claude Session Manager) sandbox mode enables you to run AI-assisted development sessions in isolated, copy-on-write filesystem environments. This protects your repositories from accidental destructive operations while allowing agents to operate freely.

**Key Benefits**:
- **Zero-Copy Isolation**: No repository duplication - uses platform-native filesystem technologies
- **Host Protection**: Prevents `rm -rf` and other destructive commands from affecting your code
- **Multi-Repository Support**: Merge multiple repositories into a single workspace view
- **Secrets Management**: Securely inject API keys and credentials into the sandbox

## Getting Started

### What is Sandbox Mode?

Sandbox mode creates an isolated filesystem layer on top of your repositories. Any modifications the AI agent makes are written to a separate layer, leaving your original repositories untouched. Think of it as a "draft mode" for your filesystem.

**How it works**:
- Your repositories remain read-only (the "lower" layer)
- All changes are written to an isolated "upper" layer
- You see a merged view combining both layers
- When done, you can review changes and merge what you want to keep

### When to Use Sandboxes

**Use sandbox mode when**:
- Working with AI agents on unfamiliar codebases
- Testing destructive refactoring operations
- Allowing agents to experiment freely without risk
- Running multiple concurrent AI sessions on the same repository
- Working with sensitive repositories where mistakes are costly

**Skip sandbox mode when**:
- You fully trust the operations being performed
- Working on throw-away test repositories
- Running simple read-only analysis tasks
- Platform doesn't support sandboxing (e.g., unsupported kernel version)

### Platform Requirements

AGM sandbox uses different technologies depending on your platform:

**Linux** (Recommended - Best Performance):
- Kernel 5.11+ for rootless OverlayFS (no sudo required)
- Kernel < 5.11 requires sudo for mounting
- Supported filesystems: ext4, xfs, btrfs

**Cloud Workstations** (Google Cloud):
- Bubblewrap (bwrap) for user namespaces
- No special permissions required
- Automatically detected and used

**macOS**:
- APFS filesystem required
- Currently uses directory copy (slower for large repos)
- Future: APFS reflink support for instant cloning

**Check your platform**:
```bash
# Linux: Check kernel version
uname -r

# Linux: Verify OverlayFS support
cat /proc/filesystems | grep overlay

# Cloud Workstation: Check for bubblewrap
which bwrap

# macOS: Check filesystem type
df -T .
```

## Basic Usage

### Creating a Sandboxed Session

The simplest way to create a sandbox session:

```bash
agm session new my-session --sandbox
```

This creates a new AGM session with sandbox isolation enabled. Your current directory becomes the repository to sandbox.

**With specific repository**:
```bash
agm session new my-session --sandbox --repo ~/src/my-project
```

**With multiple repositories** (merged into one workspace):
```bash
agm session new my-session --sandbox \
  --repo ~/src/backend \
  --repo ~/src/frontend \
  --repo ~/src/shared
```

### Working in the Sandbox

Once created, AGM automatically places you in the sandbox environment:

```bash
# Your shell is now in the sandbox
pwd
# Output: /tmp/agm-sandboxes/my-session/merged

# Make changes freely - they're isolated
rm -rf src/  # Safe! Only affects sandbox layer

# Original repository is unchanged
ls ~/src/my-project/src/  # Still there!
```

### Reviewing Changes

After your AI session, review what was modified:

```bash
# View modified files (in upperdir)
ls -la /tmp/agm-sandboxes/my-session/upperdir/

# Compare with original
diff -r ~/src/my-project /tmp/agm-sandboxes/my-session/merged/
```

### Cleaning Up

**Destroy sandbox and discard changes**:
```bash
agm session kill my-session
# Sandbox is removed, original repo unchanged
```

**Keep sandbox for later review**:
```bash
agm session kill my-session --keep-sandbox
# Sandbox files remain in /tmp/agm-sandboxes/my-session/
```

## Configuration

### Sandbox Provider Selection

AGM auto-detects the best provider for your platform, but you can override:

```bash
# Auto-detect (default)
agm session new my-session --sandbox

# Force specific provider
agm session new my-session --sandbox --provider bubblewrap
agm session new my-session --sandbox --provider overlayfs
```

**Available providers**:
- `auto` - Platform auto-detection (recommended)
- `bubblewrap` - For Cloud Workstations (user namespaces)
- `overlayfs` - For Linux with OverlayFS support
- `mock` - For testing only

### Secrets Injection

Inject API keys and credentials securely into your sandbox:

```bash
# Inject from environment variable
agm session new my-session --sandbox \
  --secret ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY}"

# Inject multiple secrets
agm session new my-session --sandbox \
  --secret ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY}" \
  --secret GITHUB_TOKEN="${GITHUB_TOKEN}"
```

Secrets are written to `.env` in the sandbox with strict permissions (0600):

```bash
# Inside sandbox
cat .env
ANTHROPIC_API_KEY=sk-ant-...
GITHUB_TOKEN=ghp_...
```

**Security notes**:
- Secrets are isolated to the sandbox (never written to lowerdir)
- Automatically cleaned up when sandbox is destroyed
- File permissions prevent other users from reading

### Environment Variables

Configure sandbox behavior with environment variables:

```bash
# Set custom workspace directory
export AGM_SANDBOX_WORKSPACE=/custom/path
agm session new my-session --sandbox

# Set provider explicitly
export AGM_SANDBOX_PROVIDER=overlayfs
agm session new my-session --sandbox
```

## Best Practices

### When to Use Sandboxes

**Best use cases**:
1. **Exploratory refactoring** - Let AI experiment with large-scale changes
2. **Risky operations** - Testing database migrations, build system changes
3. **Multi-repo workflows** - Work across repositories as if they're one project
4. **Code review** - Isolate experimental branches from main codebase
5. **CI/CD testing** - Run integration tests without affecting host

### Performance Considerations

**Linux OverlayFS** (Optimal):
- Creation time: < 100ms
- Zero overhead for reads
- Copy-on-write for modified files only
- Recommended for production use

**Bubblewrap on Cloud Workstations** (Good):
- Creation time: < 200ms
- Minimal overhead
- Excellent for cloud development environments

**macOS APFS** (Current - Slower):
- Creation time: 2-5 seconds (directory copy)
- Use for smaller repositories
- Future reflink support will improve performance

**Tips for large repositories**:
- Use OverlayFS on Linux when possible
- Consider smaller subsets of repositories for macOS
- Clean up old sandboxes regularly to free disk space

### Resource Cleanup

**Automatic cleanup** (recommended):
```bash
# Sandbox destroyed when session ends
agm session kill my-session
```

**Manual cleanup** (if needed):
```bash
# Remove all sandboxes for a session
rm -rf /tmp/agm-sandboxes/my-session/

# Linux: Unmount if still mounted
umount /tmp/agm-sandboxes/my-session/merged
```

**Prevent orphaned resources**:
- Always use `agm session kill` to destroy sessions
- Check for orphaned mounts after crashes: `mount | grep overlay`
- Clean up workspace directory periodically

### Multi-Repository Workflows

When merging multiple repositories:

```bash
agm session new multi-repo --sandbox \
  --repo ~/src/api-server \
  --repo ~/src/web-client \
  --repo ~/src/shared-libs
```

**Repository priority**: Left-to-right precedence. If files conflict:
- Files from `api-server` override `web-client`
- Files from `web-client` override `shared-libs`

**Best practices**:
- List most important repository first
- Avoid overlapping file paths when possible
- Use separate directories for each repo's content

## Troubleshooting

### Common Errors

#### "Unsupported platform"

**Cause**: Your platform or kernel version doesn't support required features.

**Fix**:
```bash
# Linux: Check kernel version
uname -r  # Need 5.11+ for rootless

# Upgrade kernel if needed
sudo apt update && sudo apt upgrade linux-generic

# Or use sudo for older kernels
sudo agm session new my-session --sandbox
```

#### "Mount failed"

**Cause**: Permission issues or OverlayFS module not loaded.

**Fix**:
```bash
# Check if overlay module is available
cat /proc/filesystems | grep overlay

# Load module if needed
sudo modprobe overlay

# Verify kernel version
uname -r  # Should be 5.11+ for rootless
```

#### "Too many open files"

**Cause**: File descriptor limit reached (typically with 50+ sandboxes).

**Fix**:
```bash
# Check current limit
ulimit -n

# Increase temporarily
ulimit -n 4096

# Increase permanently (add to /etc/security/limits.conf)
echo "* soft nofile 4096" | sudo tee -a /etc/security/limits.conf
```

#### "No space left on device"

**Cause**: Workspace directory is full.

**Fix**:
```bash
# Check disk space
df -h /tmp/agm-sandboxes

# Clean up old sandboxes
rm -rf /tmp/agm-sandboxes/old-session-*

# Use different workspace location
export AGM_SANDBOX_WORKSPACE=/path/with/more/space
```

### Platform Compatibility Issues

**Cloud Workstation (bubblewrap)**:
- If `bwrap` not found: `sudo apt install bubblewrap`
- Check user namespace support: `cat /proc/sys/user/max_user_namespaces`

**Linux (OverlayFS)**:
- Kernel too old: Upgrade to 5.11+ or use sudo
- Filesystem not supported: Use ext4, xfs, or btrfs
- SELinux issues: `sudo setenforce 0` (temporarily for testing)

**macOS (APFS)**:
- Slow performance: Expected with current copy implementation
- Use for smaller repositories until reflink support is added

### Checking Sandbox Status

**View active sandboxes**:
```bash
# Linux: Check overlay mounts
mount | grep overlay

# List sandbox directories
ls -la /tmp/agm-sandboxes/

# Check specific sandbox
ls -la /tmp/agm-sandboxes/my-session/
```

**Verify sandbox isolation**:
```bash
# Create file in sandbox
touch /tmp/agm-sandboxes/my-session/merged/test.txt

# Verify it's NOT in original repo
ls ~/src/my-project/test.txt  # Should not exist

# Verify it IS in upperdir
ls /tmp/agm-sandboxes/my-session/upperdir/test.txt  # Should exist
```

### Getting Help

For additional help:
- **Error codes**: See `docs/ERROR_GUIDE.md`
- **Performance**: See `docs/SCALING.md`
- **Architecture**: See `internal/sandbox/ARCHITECTURE.md`

When reporting issues, include:
- Platform and kernel version (`uname -a`)
- Filesystem type (`df -T .`)
- Error message and code
- Output of `mount | grep overlay`

## Advanced Usage

### Custom Workspace Locations

```bash
# Use tmpfs for speed (CI/CD)
sudo mount -t tmpfs -o size=4G tmpfs /tmp/fast-sandboxes
export AGM_SANDBOX_WORKSPACE=/tmp/fast-sandboxes
agm session new my-session --sandbox
```

### Concurrent Sandboxes

Run multiple AI sessions simultaneously:

```bash
# Session 1: Feature development
agm session new feature-x --sandbox --repo ~/src/my-project

# Session 2: Bug fix (same repo, isolated)
agm session new bugfix-y --sandbox --repo ~/src/my-project

# Session 3: Refactoring
agm session new refactor-z --sandbox --repo ~/src/my-project
```

Each session has its own isolated workspace. Recommended limits:
- **Development**: 5-10 concurrent sandboxes
- **Production**: 50 concurrent sandboxes (with tuned file descriptor limits)
- **Enterprise**: 100+ sandboxes (requires system tuning)

### Inspecting Sandbox Internals

**Understanding the directory structure**:
```bash
/tmp/agm-sandboxes/my-session/
├── lowerdir/     # Symlinks to read-only repositories
├── upperdir/     # Your session modifications
├── workdir/      # OverlayFS internal state
└── merged/       # Where you work (combined view)
```

**View only your changes**:
```bash
# Modified files are in upperdir
find /tmp/agm-sandboxes/my-session/upperdir/ -type f

# Compare sizes
du -sh ~/src/my-project  # Original repo
du -sh /tmp/agm-sandboxes/my-session/upperdir/  # Only your changes
```

## Summary

AGM sandbox mode provides safe, isolated environments for AI-assisted development:

- **Create**: `agm session new my-session --sandbox`
- **Work**: AI agent operates in isolated environment
- **Review**: Check changes in upperdir before merging
- **Cleanup**: `agm session kill my-session`

Start with single-repository sandboxes, then explore multi-repo workflows and advanced configurations as you become comfortable with the system.

For comprehensive documentation on scaling, performance, and troubleshooting, see the guides in `docs/`.
