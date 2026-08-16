# AGM Sandbox User Guide

## Overview

AGM (Agent Gateway Manager) runs agent sessions in isolated, copy-on-write filesystem sandboxes **by default**. This protects your repositories from accidental destructive operations while allowing agents to operate freely.

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

**Skip sandbox mode** (opt out with `--no-sandbox`) **when**:
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
- Uses APFS reflink cloning (`cp -c` / clonefile — copy-on-write, near-instant)
- Falls back to recursive copy on non-APFS volumes

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

Sandbox isolation is ON by default — every new session is sandboxed:

```bash
agm session new my-session
```

The repositories included in the sandbox come from `sandbox.repos` in
`~/.config/agm/config.yaml`; when none are configured, AGM falls back to the
git repositories in your workspace (`~/src/ws/oss/repos`) and the git
repository containing your current directory. If neither yields a usable
repository, session creation fails loud (see `ErrCodeNoLowerDirs` in
[ERROR_GUIDE.md](./ERROR_GUIDE.md)) rather than sandboxing an arbitrary
directory.

**Opting out** (session runs directly on the host filesystem):
```bash
agm session new my-session --no-sandbox
```

**With multiple repositories** (merged into one workspace), configure them in
`~/.config/agm/config.yaml`:
```yaml
sandbox:
  repos:
    - ~/src/backend
    - ~/src/frontend
    - ~/src/shared
```

### Working in the Sandbox

Once created, AGM automatically places you in the sandbox environment:

```bash
# Your shell is now in the sandbox
pwd
# Output: ~/.agm/sandboxes/my-session/merged

# Make changes freely - they're isolated
rm -rf src/  # Safe! Only affects sandbox layer

# Original repository is unchanged
ls ~/src/my-project/src/  # Still there!
```

### Reviewing Changes

After your AI session, review what was modified:

```bash
# View modified files (in the upper layer)
ls -la ~/.agm/sandboxes/my-session/upper/

# Compare with original
diff -r ~/src/my-project ~/.agm/sandboxes/my-session/merged/
```

### Cleaning Up

```bash
agm session kill my-session
# Original repo unchanged
```

Sandbox directories persist under `~/.agm/sandboxes/` after a session ends —
you can still review changes there. They are removed by the garbage
collector once the session is archived/dead:

```bash
# Preview what would be reaped (dry-run, the default)
agm sandbox gc

# Actually delete eligible sandboxes
agm sandbox gc --reap
```

## Configuration

### Sandbox Provider Selection

AGM auto-detects the best provider for your platform, but you can override
with `--sandbox-provider` (or the `sandbox.provider` config key):

```bash
# Auto-detect (default)
agm session new my-session

# Force specific provider
agm session new my-session --sandbox-provider bubblewrap
agm session new my-session --sandbox-provider overlayfs
```

**Available providers**:
- `auto` - Platform auto-detection (recommended)
- `bubblewrap` - For Cloud Workstations (user namespaces)
- `overlayfs` - For Linux with OverlayFS support
- `gvisor` - For Linux with gVisor (runsc) isolation
- `apfs` - For macOS (APFS reflink cloning)
- `mock` - For testing only

### Secrets Injection

Inject API keys and credentials securely into your sandbox via the
`sandbox.secrets` map in `~/.config/agm/config.yaml`:

```yaml
sandbox:
  secrets:
    ANTHROPIC_API_KEY: "sk-ant-..."
    GITHUB_TOKEN: "ghp_..."
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

### Writable Host Directories

By default a sandboxed session can only write inside its workspace. To let it
write to specific real host paths (e.g. commit to a real worktree), list them
under `sandbox.writable_dirs` in `~/.config/agm/config.yaml`; they are
surfaced to the harness as `--add-dir` entries and are not reflinked into the
sandbox.

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

**macOS APFS**:
- Reflink cloning (copy-on-write) — near-instant on APFS volumes
- Recursive-copy fallback only on non-APFS volumes

**Tips for large repositories**:
- Use OverlayFS on Linux when possible
- Clean up old sandboxes regularly to free disk space (`agm sandbox gc --reap`)

### Resource Cleanup

**Automatic cleanup** (recommended):
```bash
# End the session, then reap its sandbox once it is archived/dead
agm session kill my-session
agm sandbox gc --reap
```

**Prevent orphaned resources**:
- Always use `agm session kill` to destroy sessions
- Check for orphaned mounts after crashes: `mount | grep overlay`
- Let `agm sandbox gc` handle directory removal — it refuses to reap while
  any mount, live session, or live process still references a sandbox (see
  [RECOVERY.md](./RECOVERY.md))

### Multi-Repository Workflows

When merging multiple repositories, list them in `sandbox.repos` in
`~/.config/agm/config.yaml`:

```yaml
sandbox:
  repos:
    - ~/src/api-server
    - ~/src/web-client
    - ~/src/shared-libs
```

**Repository priority**: If files conflict, earlier entries win:
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
sudo agm session new my-session
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
df -h ~/.agm/sandboxes

# Clean up old sandboxes
agm sandbox gc --reap
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
- Slow creation: usually means the workspace is on a non-APFS volume, which
  forces the recursive-copy fallback — keep `~/.agm` on an APFS volume

### Checking Sandbox Status

**View active sandboxes**:
```bash
# Linux: Check overlay mounts
mount | grep overlay

# List sandbox directories
ls -la ~/.agm/sandboxes/

# Check specific sandbox
ls -la ~/.agm/sandboxes/my-session/
```

**Verify sandbox isolation**:
```bash
# Create file in sandbox
touch ~/.agm/sandboxes/my-session/merged/test.txt

# Verify it's NOT in original repo
ls ~/src/my-project/test.txt  # Should not exist

# Verify it IS in the upper layer
ls ~/.agm/sandboxes/my-session/upper/test.txt  # Should exist
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

### Concurrent Sandboxes

Run multiple AI sessions simultaneously — each gets its own isolated sandbox
of the repos configured in `sandbox.repos`:

```bash
# Session 1: Feature development
agm session new feature-x

# Session 2: Bug fix (same repo, isolated)
agm session new bugfix-y

# Session 3: Refactoring
agm session new refactor-z
```

Each session has its own isolated workspace. Recommended limits:
- **Development**: 5-10 concurrent sandboxes
- **Production**: 50 concurrent sandboxes (with tuned file descriptor limits)
- **Enterprise**: 100+ sandboxes (requires system tuning)

### Inspecting Sandbox Internals

**Understanding the directory structure** (OverlayFS provider; the read-only
lower layers are the real repository paths):
```bash
~/.agm/sandboxes/my-session/
├── upper/        # Your session modifications
├── work/         # OverlayFS internal state
└── merged/       # Where you work (combined view)
```

**View only your changes**:
```bash
# Modified files are in the upper layer
find ~/.agm/sandboxes/my-session/upper/ -type f

# Compare sizes
du -sh ~/src/my-project  # Original repo
du -sh ~/.agm/sandboxes/my-session/upper/  # Only your changes
```

## Summary

AGM sandbox mode provides safe, isolated environments for AI-assisted development:

- **Create**: `agm session new my-session` (sandboxed by default; `--no-sandbox` opts out)
- **Work**: AI agent operates in isolated environment
- **Review**: Check changes in the sandbox's `upper/` layer before merging
- **Cleanup**: `agm session kill my-session`, then `agm sandbox gc --reap`

Start with single-repository sandboxes, then explore multi-repo workflows and advanced configurations as you become comfortable with the system.

For comprehensive documentation on scaling, performance, and troubleshooting, see the guides in `docs/`.
