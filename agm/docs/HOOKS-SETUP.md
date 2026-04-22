# AGM SessionStart Hook Setup Guide

## Overview

AGM uses Claude Code's SessionStart hooks to reliably detect when the CLI is ready to accept commands. This replaces fragile text-parsing-based prompt detection with a deterministic file-based signal.

## Why Hooks?

**Problem**: Text parsing Claude's prompt output is fragile:
- Control mode timing issues (only sees new output)
- Race conditions between detection and command sending
- False positives from partial prompts
- Breaks when prompt format changes

**Solution**: SessionStart hook creates a ready-file when Claude is initialized:
- Deterministic (file exists = ready, file missing = not ready)
- No text parsing required
- Testable and debuggable
- Resilient to Claude UI changes

## Installation

### Step 1: Copy Hook Script

```bash
# Create hooks directory if it doesn't exist
mkdir -p ~/.config/claude/hooks

# Copy the AGM SessionStart hook
cp docs/hooks/session-start-agm.sh ~/.config/claude/hooks/

# Make executable
chmod +x ~/.config/claude/hooks/session-start-agm.sh
```

### Step 2: Configure Claude Code

Add the hook to your Claude Code configuration file: `~/.config/claude/config.yaml`

If the file doesn't exist, create it with:

```yaml
hooks:
  SessionStart:
    - name: agm-ready-signal
      command: ~/.config/claude/hooks/session-start-agm.sh
```

If you already have hooks configured, add the AGM hook to the existing SessionStart array:

```yaml
hooks:
  SessionStart:
    - name: existing-hook-1
      command: /path/to/existing-hook.sh
    - name: agm-ready-signal  # Add this
      command: ~/.config/claude/hooks/session-start-agm.sh
```

### Step 3: Test the Hook

Create a test session to verify the hook works:

```bash
# Start a new AGM session with debug logging
agm new test-hook --debug

# Check for ready-file (should exist if hook worked)
ls -l ~/.agm/claude-ready-test-hook

# If file exists, hook is working correctly!
# If not, check troubleshooting section below
```

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│ agm new session-name                                        │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ├─> 1. Cleanup old ready-files
                  │
                  ├─> 2. Create tmux session
                  │
                  ├─> 3. Start Claude with AGM_SESSION_NAME env var
                  │      $ AGM_SESSION_NAME=session-name claude ...
                  │
                  ├─> 4. Claude starts initialization
                  │      ├─> Load config, plugins, etc.
                  │      └─> Run SessionStart hooks (including ours)
                  │
                  ├─> 5. SessionStart hook runs
                  │      ├─> Checks AGM_SESSION_NAME is set
                  │      ├─> Creates ~/.agm/claude-ready-{session-name}
                  │      └─> Logs to stderr: "Claude ready signal created"
                  │
                  ├─> 6. agm detects ready-file
                  │      Polling ~/.agm/claude-ready-session-name
                  │      (timeout: 30 seconds)
                  │
                  ├─> 7. agm sends initialization commands
                  │      ├─> /rename session-name
                  │      └─> /agm:agm-assoc session-name
                  │
                  └─> 8. Wait for association ready-file
                       ~/.agm/ready-session-name (from skill)
```

## Verification

### Check Hook is Installed

```bash
# Verify hook file exists and is executable
ls -l ~/.config/claude/hooks/session-start-agm.sh
# Should show: -rwxr-xr-x ... session-start-agm.sh

# Verify config references the hook
grep -A 3 "SessionStart:" ~/.config/claude/config.yaml
# Should show:
#   SessionStart:
#     - name: agm-ready-signal
#       command: ~/.config/claude/hooks/session-start-agm.sh
```

### Monitor Hook Execution

When running `agm new --debug`, you should see:

```
[HH:MM:SS] Waiting for Claude ready signal from SessionStart hook (timeout: 30s)
[AGM Hook] Claude ready signal created for session: <session-name>
[HH:MM:SS] Claude ready signal received
```

If you see timeout instead:

```
[HH:MM:SS] ERROR: timeout waiting for Claude ready signal after 30s (hook may not be configured)
```

This means the hook didn't run or failed.

### Manual Testing

```bash
# 1. Set the environment variable manually
export AGM_SESSION_NAME=manual-test

# 2. Run the hook script directly
~/.config/claude/hooks/session-start-agm.sh

# 3. Check for ready-file
ls -l ~/.agm/claude-ready-manual-test
# Should exist if hook works correctly

# 4. Cleanup
rm ~/.agm/claude-ready-manual-test
unset AGM_SESSION_NAME
```

## Troubleshooting

### Hook Not Running

**Symptom**: `agm new` times out waiting for ready signal

**Possible causes**:

1. **Hook not configured in config.yaml**
   ```bash
   # Check if hook is configured
   grep -A 3 "SessionStart:" ~/.config/claude/config.yaml
   ```

2. **Hook file not executable**
   ```bash
   # Make executable
   chmod +x ~/.config/claude/hooks/session-start-agm.sh
   ```

3. **Hook file path incorrect in config**
   ```bash
   # Verify path matches config
   ls -l ~/.config/claude/hooks/session-start-agm.sh
   ```

4. **Claude config syntax error**
   ```bash
   # Check config is valid YAML
   python3 -c "import yaml; yaml.safe_load(open('~/.config/claude/config.yaml'))"
   ```

### Hook Running But No Ready-File

**Symptom**: Hook logs appear but ready-file doesn't exist

**Debug steps**:

```bash
# 1. Check .agm directory exists and is writable
ls -ld ~/.agm
mkdir -p ~/.agm

# 2. Run hook manually and check for errors
AGM_SESSION_NAME=debug-test ~/.config/claude/hooks/session-start-agm.sh
echo "Exit code: $?"  # Should be 0

# 3. Verify ready-file was created
ls -l ~/.agm/claude-ready-debug-test
```

### Multiple SessionStart Hooks Conflicting

**Symptom**: Other hooks interfere with AGM hook

**Solution**: AGM hook is designed to be independent. It:
- Only runs when `AGM_SESSION_NAME` is set
- Exits immediately (0) if not an AGM session
- Doesn't interfere with other hooks

Order doesn't matter - all SessionStart hooks run in parallel.

### Stale Ready-Files

**Symptom**: Ready-file exists from previous session

**Solution**: AGM automatically cleans up stale files:
```bash
# Manual cleanup if needed
rm ~/.agm/claude-ready-*
rm ~/.agm/pending-*
```

## Advanced Configuration

### Custom Hook Location

If you want to store the hook elsewhere:

1. Update the path in `config.yaml`:
   ```yaml
   hooks:
     SessionStart:
       - name: agm-ready-signal
         command: /custom/path/to/session-start-agm.sh
   ```

2. Ensure the custom path is absolute and executable

### Debug Logging

The hook logs to stderr, which is captured in agm debug logs.

To see hook output in Claude:

```bash
# Run Claude directly and watch for hook output
claude 2>&1 | grep "AGM Hook"
```

### Integration with Other Hooks

If you have existing SessionStart hooks, you can combine them:

```yaml
hooks:
  SessionStart:
    - name: my-custom-init
      command: ~/.config/claude/hooks/my-init.sh
    - name: agm-ready-signal
      command: ~/.config/claude/hooks/session-start-agm.sh
    - name: another-hook
      command: ~/.config/claude/hooks/another.sh
```

All hooks run in parallel. AGM hook won't interfere with others.

## Fallback Behavior

**Current implementation**: If the hook is not configured, `agm new` will fail with a clear error message:

```
ERROR: timeout waiting for Claude ready signal after 30s (hook may not be configured)

Please ensure SessionStart hook is configured. See docs/HOOKS-SETUP.md
```

**Future enhancement**: We may add a fallback to text-parsing mode with a deprecation warning.

## Performance

Hook-based detection is faster and more reliable than text parsing:

| Metric | Text Parsing | Hook-Based |
|--------|-------------|------------|
| Detection time | 1-5 seconds (variable) | <100ms (deterministic) |
| False positives | Common (partial prompts) | None (file exists or not) |
| Race conditions | Frequent (timing issues) | None (file-based sync) |
| Maintenance | High (breaks on UI changes) | Low (simple file check) |

## Migration from Text Parsing

If you're upgrading from an older AGM version that used text parsing:

1. Install the SessionStart hook (see Installation above)
2. Test with a new session: `agm new test-migration --debug`
3. Verify ready-file appears: `ls ~/.agm/claude-ready-test-migration`
4. If successful, hook-based detection is working
5. Old ready-files are automatically cleaned up

No manual migration steps required - the new version uses hooks automatically.

## FAQ

**Q: Do I need to configure the hook for every session?**

A: No. The hook is configured once in `~/.config/claude/config.yaml` and runs for all AGM sessions.

**Q: What if I don't use AGM?**

A: The hook checks `AGM_SESSION_NAME` and exits immediately if not set. It has zero impact on non-AGM Claude sessions.

**Q: Can I use agm without the hook?**

A: No. The hook is required for reliable session initialization. Without it, agm cannot detect when Claude is ready for commands.

**Q: Will this work with tmux?**

A: Yes. The hook runs inside the Claude CLI process, which runs inside tmux. The hook creates files that agm (running outside tmux) can detect.

**Q: What if the hook hangs or crashes?**

A: The hook is simple (<20 lines) and just creates a file. If it fails, you'll get a timeout error. Check troubleshooting steps above.

**Q: Can I modify the hook?**

A: Yes, but be careful. The hook must:
- Check `AGM_SESSION_NAME` is set
- Create `~/.agm/claude-ready-${AGM_SESSION_NAME}`
- Exit with code 0 on success

**Q: Does this work with Claude Code updates?**

A: Yes. The hook uses standard SessionStart hook mechanism, which is a stable Claude Code API. Claude updates won't break it.

## Support

If you encounter issues:

1. Check troubleshooting section above
2. Run with debug logging: `agm new <session> --debug`
3. Check debug logs: `~/.agm/debug/new-<session>-*.log`
4. Verify ready-files: `ls -l ~/.agm/`
5. Test hook manually (see Manual Testing above)

For bugs or feature requests, open an issue on the AGM repository.
