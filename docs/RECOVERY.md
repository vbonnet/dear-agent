# Sandbox Recovery Procedures

This document provides step-by-step recovery procedures for common AGM sandbox failures and cleanup issues.

## Table of Contents

- [Quick Diagnostics](#quick-diagnostics)
- [Common Failure Scenarios](#common-failure-scenarios)
- [Recovery Procedures](#recovery-procedures)
- [Manual Cleanup](#manual-cleanup)
- [Prevention Best Practices](#prevention-best-practices)

## Quick Diagnostics

### Check for Orphaned Mounts (Linux)

```bash
# List all overlay mounts
grep overlay /proc/mounts

# Check for mounts in AGM sandbox directory
grep "\.agm/sandboxes" /proc/mounts
```

### Check for Orphaned Directories

```bash
# List sandbox directories with age
ls -lhta ~/.agm/sandboxes/

# Find directories older than 24 hours
find ~/.agm/sandboxes/ -maxdepth 1 -type d -mtime +1
```

### Check Disk Usage

```bash
# Check total disk usage of sandboxes
du -sh ~/.agm/sandboxes/

# Check individual sandbox sizes
du -sh ~/.agm/sandboxes/*
```

## Common Failure Scenarios

### 1. "Device or resource busy" (EBUSY) on Unmount

**Symptoms:**
- Cannot unmount overlay filesystem
- Error: `umount: target is busy`
- Sandbox cleanup fails

**Cause:**
- Process still has files open in sandbox
- Working directory is inside the mount
- File descriptors not closed

**Recovery:**
See [Unmount Busy Filesystem](#unmount-busy-filesystem)

### 2. Orphaned Mounts After Crash

**Symptoms:**
- Process crashed or was killed
- Mounts remain in `/proc/mounts`
- Directories cannot be removed

**Cause:**
- AGM process terminated before cleanup
- System crash or power failure
- OOM killer terminated process

**Recovery:**
See [Clean Up Orphaned Mounts](#clean-up-orphaned-mounts)

### 3. Permission Denied on Cleanup

**Symptoms:**
- Error: `permission denied` when removing directories
- Cannot delete files in upperdir

**Cause:**
- File ownership issues (rare with user namespaces)
- Locked files on macOS
- SELinux/AppArmor restrictions

**Recovery:**
See [Fix Permission Issues](#fix-permission-issues)

### 4. Partial Cleanup Failure

**Symptoms:**
- Some directories removed, others remain
- Mixed error messages
- Inconsistent state

**Cause:**
- Cleanup interrupted mid-operation
- Filesystem errors
- Race conditions

**Recovery:**
See [Complete Partial Cleanup](#complete-partial-cleanup)

## Recovery Procedures

### Unmount Busy Filesystem

**Diagnostic:**
```bash
# Find processes using the mount
lsof +D ~/.agm/sandboxes/SESSION_ID/merged

# Or use fuser
fuser -vm ~/.agm/sandboxes/SESSION_ID/merged
```

**Solution 1 - Wait for processes:**
```bash
# Wait for processes to finish
# AGM sessions should auto-cleanup

# Check if mount is still busy
mountpoint ~/.agm/sandboxes/SESSION_ID/merged
```

**Solution 2 - Force unmount:**
```bash
# Force unmount (Linux)
umount -f ~/.agm/sandboxes/SESSION_ID/merged

# Or use lazy unmount as last resort
umount -l ~/.agm/sandboxes/SESSION_ID/merged
```

**Solution 3 - Kill processes:**
```bash
# Identify PIDs
lsof +D ~/.agm/sandboxes/SESSION_ID/merged | awk 'NR>1 {print $2}' | sort -u

# Kill specific processes
kill -9 <PID>

# Then unmount
umount ~/.agm/sandboxes/SESSION_ID/merged
```

### Clean Up Orphaned Mounts

**Automatic cleanup using AGM tools:**

```bash
# Use the built-in cleanup utility
go run ./cmd/agm-sandbox cleanup --all

# Or cleanup mounts older than 1 hour
go run ./cmd/agm-sandbox cleanup --older-than=1h
```

**Manual cleanup:**

```bash
# List all AGM sandbox mounts
grep "\.agm/sandboxes" /proc/mounts | awk '{print $2}'

# Unmount each one
for mount in $(grep "\.agm/sandboxes" /proc/mounts | awk '{print $2}'); do
    echo "Unmounting $mount"
    umount "$mount" || umount -f "$mount"
done

# Verify all unmounted
grep "\.agm/sandboxes" /proc/mounts
```

### Fix Permission Issues

**Linux - Reset ownership:**
```bash
# If running as non-root user with user namespaces
# Check current ownership
ls -la ~/.agm/sandboxes/SESSION_ID/upper/

# Files should be owned by your user
# If not, try:
sudo chown -R $USER:$USER ~/.agm/sandboxes/SESSION_ID/
```

**macOS - Unlock files:**
```bash
# Unlock all files in directory
chflags -R nouchg ~/.agm/sandboxes/SESSION_ID/

# Then remove
rm -rf ~/.agm/sandboxes/SESSION_ID/
```

**Force removal:**
```bash
# Last resort - use sudo (be careful!)
sudo rm -rf ~/.agm/sandboxes/SESSION_ID/
```

### Complete Partial Cleanup

**Strategy:** Run cleanup in phases with verification.

```bash
# Phase 1: Unmount (if applicable)
if mountpoint -q ~/.agm/sandboxes/SESSION_ID/merged; then
    umount ~/.agm/sandboxes/SESSION_ID/merged || umount -f ~/.agm/sandboxes/SESSION_ID/merged
fi

# Phase 2: Remove merged (usually a symlink or mount point)
rm -rf ~/.agm/sandboxes/SESSION_ID/merged

# Phase 3: Remove work directory
rm -rf ~/.agm/sandboxes/SESSION_ID/work

# Phase 4: Remove upper directory
rm -rf ~/.agm/sandboxes/SESSION_ID/upper

# Phase 5: Remove parent directory
rmdir ~/.agm/sandboxes/SESSION_ID/

# Verify cleanup
ls ~/.agm/sandboxes/SESSION_ID/ 2>&1 | grep "No such file"
```

## Manual Cleanup

### Complete Manual Cleanup Script

Save as `cleanup-sandbox.sh`:

```bash
#!/bin/bash
set -euo pipefail

SANDBOX_DIR="${1:-}"

if [ -z "$SANDBOX_DIR" ]; then
    echo "Usage: $0 <sandbox-directory>"
    echo "Example: $0 ~/.agm/sandboxes/abc-123-def"
    exit 1
fi

if [ ! -d "$SANDBOX_DIR" ]; then
    echo "Directory not found: $SANDBOX_DIR"
    exit 1
fi

echo "Cleaning up: $SANDBOX_DIR"

# Step 1: Unmount merged directory
MERGED_DIR="$SANDBOX_DIR/merged"
if mountpoint -q "$MERGED_DIR" 2>/dev/null; then
    echo "Unmounting $MERGED_DIR..."
    umount "$MERGED_DIR" || umount -f "$MERGED_DIR" || umount -l "$MERGED_DIR"
fi

# Step 2: Remove directories
echo "Removing directories..."
rm -rf "$MERGED_DIR"
rm -rf "$SANDBOX_DIR/work"
rm -rf "$SANDBOX_DIR/upper"

# Step 3: Remove parent directory
echo "Removing parent directory..."
rmdir "$SANDBOX_DIR" || rm -rf "$SANDBOX_DIR"

echo "Cleanup complete: $SANDBOX_DIR"
```

Usage:
```bash
chmod +x cleanup-sandbox.sh
./cleanup-sandbox.sh ~/.agm/sandboxes/abc-123-def
```

### Cleanup All Old Sandboxes

```bash
#!/bin/bash
# Cleanup sandboxes older than 24 hours

SANDBOX_BASE="$HOME/.agm/sandboxes"
CUTOFF_HOURS=24

echo "Finding sandboxes older than $CUTOFF_HOURS hours..."

find "$SANDBOX_BASE" -maxdepth 1 -type d -mtime +1 | while read -r dir; do
    if [ "$dir" == "$SANDBOX_BASE" ]; then
        continue
    fi

    echo "Cleaning: $dir"

    # Unmount if mounted
    if mountpoint -q "$dir/merged" 2>/dev/null; then
        umount -f "$dir/merged" 2>/dev/null || true
    fi

    # Remove directory
    rm -rf "$dir"
done

echo "Cleanup complete"
```

## Prevention Best Practices

### 1. Always Use Context with Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

sandbox, err := provider.Create(ctx, req)
```

### 2. Always Defer Cleanup

```go
sandbox, err := provider.Create(ctx, req)
if err != nil {
    return err
}
defer func() {
    if err := provider.Destroy(context.Background(), sandbox.ID); err != nil {
        log.Printf("Cleanup failed: %v", err)
    }
}()
```

### 3. Use Graceful Shutdown

```go
// In your main application
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    log.Println("Shutting down gracefully...")

    // Cleanup all sandboxes
    for _, id := range activeSandboxes {
        _ = provider.Destroy(context.Background(), id)
    }

    os.Exit(0)
}()
```

### 4. Regular Orphan Cleanup

Set up a cron job or systemd timer:

```bash
# Run every hour
0 * * * * /path/to/agm-sandbox cleanup --older-than=1h
```

### 5. Monitor Disk Usage

```bash
# Alert when sandbox directory exceeds 10GB
SANDBOX_SIZE=$(du -sb ~/.agm/sandboxes | awk '{print $1}')
MAX_SIZE=$((10 * 1024 * 1024 * 1024))  # 10GB

if [ "$SANDBOX_SIZE" -gt "$MAX_SIZE" ]; then
    echo "WARNING: Sandbox directory size: $(du -sh ~/.agm/sandboxes | awk '{print $1}')"
    # Trigger cleanup
fi
```

### 6. Log Cleanup Operations

Enable verbose logging during cleanup:

```go
// In provider implementation
func (p *Provider) cleanup(mergedDir, upperDir, workDir string) error {
    log.Printf("Starting cleanup: merged=%s, upper=%s, work=%s",
        mergedDir, upperDir, workDir)

    // ... cleanup logic ...

    log.Printf("Cleanup complete: %s", mergedDir)
    return nil
}
```

### 7. Test Cleanup in CI

```bash
# In your CI pipeline
go test -v ./internal/sandbox/... -run=Cleanup

# Test with race detector
go test -race -v ./internal/sandbox/... -run=Cleanup
```

## Troubleshooting Commands

### Verify Mount State

```bash
# Check if path is mounted
mountpoint ~/.agm/sandboxes/SESSION_ID/merged

# Show mount details
findmnt ~/.agm/sandboxes/SESSION_ID/merged

# Check mount options
mount | grep SESSION_ID
```

### Verify Filesystem State

```bash
# Check filesystem type
stat -f -c %T ~/.agm/sandboxes/SESSION_ID/merged

# Check inode usage
df -i ~/.agm/sandboxes/

# Check for corrupted filesystem
dmesg | grep overlay | tail -20
```

### Debug Kernel Issues

```bash
# Check kernel logs
dmesg | grep overlay
journalctl -k | grep overlay

# Check for namespace issues
cat /proc/self/uid_map
cat /proc/self/gid_map
```

## Support

If you encounter issues not covered in this guide:

1. **Check logs:** `~/.agm/logs/sandbox.log`
2. **Enable debug mode:** `export AGM_DEBUG=1`
3. **Collect diagnostics:**
   ```bash
   mount | grep agm > agm-mounts.txt
   ls -laR ~/.agm/sandboxes/ > agm-dirs.txt
   dmesg | grep overlay > kernel-logs.txt
   ```
4. **File an issue:** Include the diagnostic files above

## Related Documentation

- [Architecture](./ARCHITECTURE.md) - Understanding sandbox internals
- [Testing Guide](./TESTING.md) - Running sandbox tests
- [API Reference](./API.md) - Provider interface documentation
