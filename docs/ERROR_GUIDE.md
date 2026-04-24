# AGM Sandbox Error Recovery Guide

This guide provides detailed information about sandbox errors, their causes, and how to fix them.

## Error Categories

Errors are categorized to help you understand the type of issue and appropriate recovery strategy:

- **Transient**: Temporary errors that may resolve if retried
- **Configuration**: Invalid configuration or setup
- **Permission**: Insufficient permissions or access rights
- **Resource**: System resource exhaustion
- **Platform**: Platform incompatibility or missing features
- **State**: Invalid or inconsistent state

## Error Codes Reference

### ErrCodeUnsupportedPlatform

**Category**: Platform
**What it means**: The current platform or filesystem provider is not supported.

**Common causes**:
- Running on an unsupported operating system
- Attempting to use OverlayFS on non-Linux systems
- Attempting to use APFS on non-macOS systems

**How to fix**:
1. Verify your operating system: `uname -s`
2. Check supported platforms in documentation
3. Use platform-appropriate provider (OverlayFS for Linux, APFS for macOS)

**Diagnostic commands**:
```bash
uname -a
df -T .
```

**Prevention**: Check platform compatibility before initialization.

---

### ErrCodeKernelTooOld

**Category**: Platform
**What it means**: Your Linux kernel version is too old to support required features.

**Common causes**:
- Kernel version < 5.11 (required for rootless OverlayFS)
- Running on older Linux distributions without kernel updates

**How to fix**:
1. Check current kernel version: `uname -r`
2. Upgrade kernel to 5.11 or later:
   - Ubuntu/Debian: `sudo apt update && sudo apt upgrade linux-generic`
   - RHEL/CentOS: `sudo yum update kernel`
   - Arch: `sudo pacman -Syu linux`
3. Reboot after kernel upgrade
4. Verify new version: `uname -r`

**Diagnostic commands**:
```bash
uname -r
cat /proc/version
```

**Prevention**: Keep your system updated with latest kernel patches.

---

### ErrCodeMountFailed

**Category**: Transient
**What it means**: Failed to mount the OverlayFS filesystem.

**Common causes**:
- Insufficient permissions (need root or kernel 5.11+)
- OverlayFS kernel module not loaded
- Invalid mount options or paths
- System resource exhaustion (too many mounts)

**How to fix**:
1. **Permission issue**:
   - Check if running as root: `id -u` (should return 0)
   - Or verify kernel >= 5.11: `uname -r`
   - Try with sudo: `sudo <command>`

2. **Module not loaded**:
   ```bash
   # Check if overlay module is available
   cat /proc/filesystems | grep overlay

   # Load module if needed
   sudo modprobe overlay
   ```

3. **Too many mounts**:
   ```bash
   # Check current mount count
   cat /proc/mounts | wc -l

   # Check system limit
   cat /proc/sys/fs/mount-max
   ```

**Diagnostic commands**:
```bash
mount | grep overlay
cat /proc/filesystems
lsmod | grep overlay
```

**Prevention**:
- Use kernel 5.11+ for rootless operation
- Cleanup old sandboxes regularly
- Monitor mount count

---

### ErrCodeUnmountFailed

**Category**: Transient
**What it means**: Failed to unmount the sandbox filesystem.

**Common causes**:
- Processes still using the mount point
- Files or directories open in the mount
- System busy or resource locked

**How to fix**:
1. **Find processes using the mount**:
   ```bash
   lsof +D /path/to/mount
   fuser -m /path/to/mount
   ```

2. **Kill processes if safe**:
   ```bash
   # Kill specific process
   kill <PID>

   # Force kill if needed
   kill -9 <PID>
   ```

3. **Lazy unmount** (last resort):
   ```bash
   umount -l /path/to/mount
   ```

4. **Force unmount** (use with caution):
   ```bash
   umount -f /path/to/mount
   ```

**Diagnostic commands**:
```bash
lsof +D /path/to/mount
mount | grep /path/to/mount
fuser -mv /path/to/mount
```

**Prevention**:
- Always cleanup resources properly
- Ensure no processes are running in sandbox before destroy
- Use context cancellation for graceful shutdown

---

### ErrCodePermissionDenied

**Category**: Permission
**What it means**: Insufficient permissions to perform the operation.

**Common causes**:
- Mount operation requires root on older kernels
- Cannot write to workspace directory
- Cannot read repository directories
- SELinux or AppArmor restrictions

**How to fix**:
1. **For mount operations**:
   ```bash
   # Check kernel version
   uname -r

   # If < 5.11, use sudo
   sudo <command>

   # Or upgrade kernel (see ErrCodeKernelTooOld)
   ```

2. **For file operations**:
   ```bash
   # Check directory permissions
   ls -la /path/to/workspace

   # Fix permissions
   chmod 755 /path/to/workspace
   chown $USER /path/to/workspace
   ```

3. **SELinux issues**:
   ```bash
   # Check SELinux status
   getenforce

   # Temporarily disable for testing
   sudo setenforce 0

   # Fix SELinux contexts
   sudo chcon -R -t container_file_t /path/to/workspace
   ```

**Diagnostic commands**:
```bash
id -u
groups
ls -laZ /path/to/workspace
getenforce
```

**Prevention**:
- Use appropriate kernel version for rootless mounts
- Verify directory permissions before operations
- Configure SELinux/AppArmor policies correctly

---

### ErrCodeRepoNotFound

**Category**: State
**What it means**: The specified repository directory does not exist.

**Common causes**:
- Typo in repository path
- Repository was deleted or moved
- Relative path used instead of absolute
- Network mount unavailable

**How to fix**:
1. **Verify path exists**:
   ```bash
   ls -la /path/to/repo
   ```

2. **Check if it's a git repository**:
   ```bash
   git -C /path/to/repo status
   ```

3. **Use absolute path**:
   ```bash
   # Get absolute path
   realpath /path/to/repo
   ```

4. **Check network mounts**:
   ```bash
   mount | grep /path/to/repo
   df /path/to/repo
   ```

**Diagnostic commands**:
```bash
ls -la /path/to/repo
git -C /path/to/repo status 2>&1
realpath /path/to/repo
```

**Prevention**:
- Always use absolute paths
- Verify repository exists before sandbox creation
- Handle network mount failures gracefully

---

### ErrCodeSandboxNotFound

**Category**: State
**What it means**: The specified sandbox does not exist or was already destroyed.

**Common causes**:
- Sandbox ID typo or incorrect
- Sandbox was already destroyed
- Sandbox creation failed but ID was stored
- System restart cleared in-memory registry

**How to fix**:
1. **List active sandboxes**:
   ```bash
   # Check active overlay mounts
   mount | grep overlay
   ```

2. **Verify sandbox directories**:
   ```bash
   ls -la /workspace/path
   ```

3. **Recreate sandbox** if needed

**Diagnostic commands**:
```bash
mount | grep overlay
ls -la /workspace/path
cat /proc/mounts | grep merged
```

**Prevention**:
- Store sandbox metadata persistently if needed
- Check sandbox exists before operations
- Handle cleanup errors gracefully

---

### ErrCodeInvalidConfig

**Category**: Configuration
**What it means**: The sandbox configuration is invalid or incomplete.

**Common causes**:
- Missing required fields (SessionID, LowerDirs, WorkspaceDir)
- Empty or whitespace-only values
- Invalid path formats
- Configuration file parse errors

**How to fix**:
1. **Verify required fields**:
   - SessionID: Must be non-empty unique identifier
   - LowerDirs: Must have at least one directory path
   - WorkspaceDir: Must be non-empty path

2. **Check field formats**:
   ```bash
   # Paths should be absolute
   echo /absolute/path  # Good
   echo relative/path   # Bad
   ```

3. **Validate before use**:
   - SessionID: No special characters
   - Paths: Exist and accessible
   - Secrets: Valid key=value format

**Prevention**:
- Use configuration validation before sandbox creation
- Provide clear defaults where possible
- Validate early in request pipeline

---

### ErrCodeResourceExhausted

**Category**: Resource
**What it means**: System resources are exhausted (file descriptors, mounts, disk space, etc.).

**Common causes**:
- Too many open file descriptors
- Too many active mounts
- Insufficient disk space
- Memory exhaustion

**How to fix**:

1. **File descriptor exhaustion**:
   ```bash
   # Check current limit
   ulimit -n

   # Check current usage
   lsof | wc -l

   # Increase limit temporarily
   ulimit -n 4096

   # Increase limit permanently (add to /etc/security/limits.conf)
   echo "* soft nofile 4096" | sudo tee -a /etc/security/limits.conf
   echo "* hard nofile 8192" | sudo tee -a /etc/security/limits.conf
   ```

2. **Too many mounts**:
   ```bash
   # Check active mounts
   cat /proc/mounts | wc -l

   # Check limit
   cat /proc/sys/fs/mount-max

   # Cleanup old mounts
   umount /old/mount/point
   ```

3. **Disk space exhaustion**:
   ```bash
   # Check disk space
   df -h

   # Find large files
   du -sh /workspace/* | sort -h

   # Cleanup old sandboxes
   rm -rf /workspace/old-sandbox
   ```

**Diagnostic commands**:
```bash
ulimit -a
df -h
cat /proc/mounts | wc -l
lsof | wc -l
```

**Prevention**:
- Monitor resource usage
- Set appropriate system limits
- Implement automatic cleanup policies
- Use resource quotas

---

### ErrCodeCleanupFailed

**Category**: Resource
**What it means**: Failed to cleanup sandbox resources (unmount, remove directories).

**Common causes**:
- Mount still active
- Processes using sandbox files
- Permission issues
- Filesystem errors

**How to fix**:

1. **Check for active processes**:
   ```bash
   lsof +D /sandbox/path
   fuser -mv /sandbox/path
   ```

2. **Kill processes if safe**:
   ```bash
   kill <PID>
   # or force
   kill -9 <PID>
   ```

3. **Manual unmount**:
   ```bash
   umount /sandbox/merged
   # or lazy unmount
   umount -l /sandbox/merged
   ```

4. **Manual directory cleanup**:
   ```bash
   rm -rf /sandbox/path
   # if permission denied
   sudo rm -rf /sandbox/path
   ```

5. **Check filesystem errors**:
   ```bash
   dmesg | tail -20
   journalctl -xe
   ```

**Diagnostic commands**:
```bash
ls -la /sandbox/path
cat /proc/mounts | grep /sandbox/path
lsof +D /sandbox/path
dmesg | grep -i error
```

**Prevention**:
- Always cleanup in reverse order (unmount, then delete)
- Use retry logic for transient failures
- Log cleanup errors for debugging
- Implement cleanup verification

---

### ErrCodeOrphanedMount

**Category**: State
**What it means**: Detected an orphaned mount point from a previous session.

**Common causes**:
- Process crash without cleanup
- System restart with active mounts
- Failed destroy operation
- Manual intervention interrupted

**How to fix**:

1. **Identify orphaned mount**:
   ```bash
   mount | grep overlay
   cat /proc/mounts | grep /sandbox/path
   ```

2. **Check for processes**:
   ```bash
   lsof +D /mount/point
   ```

3. **Unmount safely**:
   ```bash
   # Graceful unmount
   umount /mount/point

   # If busy, try lazy unmount
   umount -l /mount/point

   # Last resort: force unmount
   umount -f /mount/point
   ```

4. **Cleanup directories**:
   ```bash
   rm -rf /sandbox/path
   ```

**Diagnostic commands**:
```bash
mount | grep /sandbox/path
lsof +D /mount/point
cat /proc/mounts
```

**Prevention**:
- Use proper shutdown handlers
- Implement cleanup on startup
- Monitor for orphaned resources
- Use systemd units with cleanup on failure

---

### ErrCodeFileSystemNotSupported

**Category**: Platform
**What it means**: The current filesystem does not support the required operation.

**Common causes**:
- Trying to use OverlayFS on non-ext4/xfs/btrfs
- Trying to use APFS reflinks on non-APFS volume
- Using network filesystem (NFS, SMB) that doesn't support features
- Filesystem mounted with restrictive options

**How to fix**:

1. **Check current filesystem**:
   ```bash
   df -T /workspace/path
   mount | grep /workspace/path
   ```

2. **Use supported filesystem**:
   - Linux: ext4, xfs, btrfs for OverlayFS
   - macOS: APFS for reflink cloning

3. **Change workspace directory** to supported filesystem:
   ```bash
   # Find supported filesystem
   df -T | grep -E 'ext4|xfs|btrfs|apfs'

   # Use that path for workspace
   ```

4. **For network mounts**, use local storage instead

**Diagnostic commands**:
```bash
df -T .
mount | grep $(df . | tail -1 | awk '{print $1}')
stat -f /workspace/path
```

**Prevention**:
- Validate filesystem support during initialization
- Document filesystem requirements clearly
- Provide fallback mechanisms where possible
- Test on target filesystem types

---

## General Troubleshooting Steps

### 1. Check System Requirements
```bash
# Linux
uname -r          # Kernel version (need 5.11+)
cat /proc/filesystems | grep overlay  # OverlayFS support

# macOS
sw_vers           # macOS version
df -T .           # Check for APFS
```

### 2. Verify Permissions
```bash
id -u             # User ID (0 = root)
groups            # Group memberships
ls -la /workspace # Directory permissions
```

### 3. Check Resource Usage
```bash
ulimit -a         # Resource limits
df -h             # Disk space
cat /proc/mounts | wc -l  # Mount count
lsof | wc -l      # Open file descriptors
```

### 4. Inspect Active Sandboxes
```bash
mount | grep overlay      # Active overlay mounts
ls -la /workspace/*       # Sandbox directories
cat /proc/mounts          # All active mounts
```

### 5. Review Logs
```bash
dmesg | tail -50          # Kernel messages
journalctl -xe            # System logs
# Application logs (location varies)
```

### 6. Clean Up Manually
```bash
# Unmount all sandboxes
for mount in $(mount | grep overlay | awk '{print $3}'); do
  umount $mount
done

# Remove workspace directories
rm -rf /workspace/*

# Check for orphaned mounts
mount | grep overlay
```

## Getting Help

When reporting issues, include:
1. Error code and message
2. Output of diagnostic commands
3. System information (`uname -a`, `df -T .`)
4. Recent logs (dmesg, journalctl)
5. Steps to reproduce

## Best Practices

1. **Always use absolute paths** for repositories and workspaces
2. **Check system requirements** before deployment
3. **Monitor resource usage** regularly
4. **Implement cleanup on shutdown** (SIGTERM, SIGINT handlers)
5. **Use retry logic** for transient errors
6. **Log errors with context** for debugging
7. **Validate configuration** before sandbox creation
8. **Test on target platforms** before production deployment
9. **Keep kernel and system updated** for security and features
10. **Implement health checks** to detect issues early
