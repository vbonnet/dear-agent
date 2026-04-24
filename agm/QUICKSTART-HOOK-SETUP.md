# Quick Start: Hook-Based AGM Setup

## 🎯 What Changed

AGM now uses **hook-based readiness detection** instead of fragile text-parsing. This fixes the regression where `/rename` and `/agm:agm-assoc` commands weren't being sent.

**Key benefit**: Deterministic, reliable session initialization with zero text parsing.

## ⚡ Quick Setup (3 steps)

### 1. Install the hook script

```bash
cd main/agm

# Copy hook to Claude hooks directory
mkdir -p ~/.config/claude/hooks
cp docs/hooks/session-start-agm.sh ~/.config/claude/hooks/
chmod +x ~/.config/claude/hooks/session-start-agm.sh
```

### 2. Configure Claude to run the hook

Create or update `~/.config/claude/config.yaml`:

```bash
cat > ~/.config/claude/config.yaml <<'EOF'
hooks:
  SessionStart:
    - name: agm-ready-signal
      command: ~/.config/claude/hooks/session-start-agm.sh
EOF
```

If you already have hooks configured, add the AGM hook to your existing `SessionStart` array.

### 3. Test the hook

```bash
# Test the hook script directly
cd main/agm
./docs/hooks/test-hook.sh

# Should show:
# ✓ Hook script found and executable
# ✓ Hook script runs successfully
# ✓ Ready-file created
# ✓ Hook is configured
# ✓ Hook is installed
# ✅ All hook tests passed!
```

## 🧪 Verify AGM Works

```bash
# Build the updated agm
cd main/agm
go build -o ~/go/bin/agm ./cmd/agm

# Create a test session with debug logging
agm new test-hook-verification --debug

# What you should see:
# - "Waiting for Claude ready signal from SessionStart hook"
# - "Claude ready signal received"
# - "Running InitSequence for /rename and /agm:agm-assoc"
# - "InitSequence completed successfully"
# - Session created successfully

# Verify ready-file was created
ls -l ~/.agm/claude-ready-test-hook-verification

# Attach to the session and verify commands were sent
tmux attach -t test-hook-verification
```

## 🔍 What to Look For

### Success Indicators

1. **Ready-file exists**:
   ```bash
   ls ~/.agm/claude-ready-*
   # Should show the ready-file for your session
   ```

2. **Debug log shows success**:
   ```bash
   tail -20 ~/.agm/debug/new-test-hook-verification-*.log
   # Look for "Claude ready signal received"
   ```

3. **Session is renamed**:
   ```bash
   # In the Claude session, the title should match session name
   # /rename command should have run
   ```

4. **Association completed**:
   ```bash
   # Manifest file should exist
   cat ~/src/sessions/test-hook-verification/manifest.yaml
   # Should show session_id, name, etc.
   ```

### Failure Indicators

If the hook is NOT configured, you'll see:

```
Failed to detect Claude ready signal
  SessionStart hook may not be configured.
  Please see docs/HOOKS-SETUP.md for setup instructions.
```

**Fix**: Go back to step 2 and configure the hook.

## 📚 Detailed Documentation

For more information, see:

- `docs/HOOKS-SETUP.md` - Comprehensive setup guide (2600+ words)
- `docs/HOOK-BASED-READINESS-DESIGN.md` - Design rationale
- `docs/HOOK-IMPLEMENTATION-SUMMARY.md` - Implementation details

## 🐛 Troubleshooting

### Hook not running

```bash
# Check hook is executable
ls -l ~/.config/claude/hooks/session-start-agm.sh
# Should show: -rwxr-xr-x

# Check config is correct
cat ~/.config/claude/config.yaml | grep -A 3 "SessionStart"

# Test hook manually
AGM_SESSION_NAME=manual-test ~/.config/claude/hooks/session-start-agm.sh
ls ~/.agm/claude-ready-manual-test  # Should exist
rm ~/.agm/claude-ready-manual-test  # Cleanup
```

### Ready-file not appearing

```bash
# Check .agm directory exists and is writable
ls -ld ~/.agm
mkdir -p ~/.agm

# Check for hook errors in debug log
tail -50 ~/.agm/debug/new-*.log | grep -i "hook\|ready"
```

### Commands not being sent

```bash
# Check InitSequence completed
tail -50 ~/.agm/debug/new-*.log | grep "InitSequence"

# Attach to session and check Claude prompt
tmux attach -t <session-name>

# Manually send commands to test
/rename test-manual
/agm:agm-assoc test-manual
```

## ✅ What This Fixes

**Before** (text-parsing approach):
- ❌ Control mode timing issues
- ❌ Race conditions
- ❌ False positives from partial prompts
- ❌ Commands sent before Claude ready
- ❌ Fragile regex pattern matching

**After** (hook-based approach):
- ✅ Deterministic file-based signaling
- ✅ No race conditions
- ✅ Commands only sent when Claude ready
- ✅ No text parsing whatsoever
- ✅ Testable and debuggable

## 🚀 Next Steps

1. Run the quick setup steps above
2. Test with `agm new test-hook-verification --debug`
3. Verify ready-file appears
4. Check debug logs show success
5. Report any issues or success!

For questions or issues, see:
- `docs/HOOKS-SETUP.md` - Comprehensive troubleshooting
- Debug logs: `~/.agm/debug/new-*.log`
- Test script: `./docs/hooks/test-hook.sh`
