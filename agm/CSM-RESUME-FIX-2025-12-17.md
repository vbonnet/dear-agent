# AGM Resume Bug Fixes - 2025-12-17

## Issues Fixed

### 1. UUID Display Truncation Bug
**Location**: `cmd/csm/resume.go:60`

**Problem**:
```
✓ Resolved identifier "agent-to-agent" to UUID: session-
```
The code was trying to display `uuid[:8]` but the variable contained the full SessionID `"session-agent-to-agent"`, resulting in truncated output showing only `"session-"`.

**Root Cause**:
Variable naming confusion - the function returns a `SessionID` (format: `session-<name>`), not a Claude UUID (format: 36-char UUID).

**Fix**:
- Renamed variable from `uuid` to `sessionID` throughout `cmd/csm/resume.go`
- Removed `[:8]` slice operations to show full session identifier
- Updated all references in lines: 52, 60, 74, 81, 101, 106, 383

**After**:
```
✓ Resolved identifier "agent-to-agent" to session: session-agent-to-agent
```

---

### 2. Tmux Attach "signal: killed" Error
**Location**: `internal/tmux/tmux.go:76-97`

**Problem**:
```
❌ failed to attach to tmux session: failed to attach to tmux session: signal: killed
```

**Root Cause**:
Insufficient TTY detection. The original code only checked:
```go
if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
    return nil  // No TTY
}
```

This incorrectly identified `/dev/null` as a TTY because:
- `/dev/null` is a **character device** (passes the check)
- But it's **not a terminal** (tmux attach fails with "open terminal failed: not a terminal")

When running from Claude Code, stdin is redirected to `/dev/null`:
```bash
$ ls -la /proc/self/fd/0
lr-x------ 1 user user 64 Dec 17 10:40 /proc/self/fd/0 -> /dev/null
```

**Fix**:
Added proper terminal detection using `golang.org/x/term.IsTerminal()`:

```go
import "golang.org/x/term"

// Check 1: Can we stat stdin?
fileInfo, err := os.Stdin.Stat()
if err != nil {
    return nil  // Can't check, skip attach
}

// Check 2: Is it a character device?
if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
    return nil  // Not a char device, definitely not a TTY
}

// Check 3: Is it actually a terminal? (not /dev/null)
if !term.IsTerminal(int(os.Stdin.Fd())) {
    return nil  // Char device but not a terminal, skip attach
}

// All checks passed - we have a real TTY
cmd := exec.Command("tmux", "attach-session", "-t", name)
```

**Dependencies Added**:
- `golang.org/x/term v0.38.0`

**After**:
```
✓ Attaching to tmux session: agent-to-agent
✓ Successfully resumed session session-agent-to-agent
```

---

### 3. Makefile Installation Path
**Location**: `Makefile:13-26`

**Problem**:
`make install` only installed to `~/.local/bin/`, but `~/go/bin/agm` was in PATH first and contained an outdated binary.

**Fix**:
Updated Makefile to install to both locations:
```makefile
install: build
	mkdir -p ~/.local/bin
	mkdir -p ~/go/bin
	cp bin/agm ~/.local/bin/agm
	cp bin/agm ~/go/bin/agm
	@echo "✓ agm binary installed to ~/.local/bin/agm and ~/go/bin/agm"
```

---

## Files Modified

1. `cmd/csm/resume.go` - Variable renaming and display fixes
2. `internal/tmux/tmux.go` - Proper TTY detection
3. `Makefile` - Install to both `~/.local/bin/` and `~/go/bin/`
4. `go.mod` / `go.sum` - Added `golang.org/x/term` dependency

## Testing

### Before Fix:
```bash
$ agm session resume agent-to-agent
✓ Resolved identifier "agent-to-agent" to UUID: session-
...
❌ failed to attach to tmux session: signal: killed
```

### After Fix:
```bash
$ agm session resume agent-to-agent
✓ Resolved identifier "agent-to-agent" to session: session-agent-to-agent

Session Health Check:
────────────────────────────────────────────────
✓ Worktree:      /home/user
✓ Session env:
✓ File history:
✓ Tmux:          agent-to-agent (EXISTS)

✓ Tmux session agent-to-agent already exists
✓ Claude already running - skipping resume commands
✓ Attaching to tmux session: agent-to-agent

✓ Successfully resumed session session-agent-to-agent
```

## Installation

To apply these fixes:

```bash
make -C ~/src/repos/ai-tools/main/agm clean
make -C ~/src/repos/ai-tools/main/agm build
cp ~/src/repos/ai-tools/main/agm/bin/agm ~/go/bin/agm
# or
make -C ~/src/repos/ai-tools/main/agm install
```

## Notes

### Empty Claude UUID Handling
The `agent-to-agent` session had an empty `claude.uuid` field in its manifest. The code already handles this gracefully by:
1. Detecting the empty UUID
2. Warning: "No Claude UUID found in manifest - starting new Claude session"
3. Sending `claude` command instead of `claude --resume <uuid>`

To properly associate a Claude UUID with a session:
```bash
agm session associate <session-name>           # Auto-detect from history
agm session associate <session-name> --uuid <uuid>  # Explicit UUID
```

### TTY Detection Logic
The three-check approach ensures compatibility:
1. **Error check**: Handles cases where stdin.Stat() fails
2. **Character device check**: Fast filter for non-devices (files, pipes)
3. **Terminal check**: Proper terminal verification using syscall

This prevents false positives from:
- `/dev/null` (char device but not a terminal)
- `/dev/zero` (char device but not a terminal)
- `/dev/random` (char device but not a terminal)

While correctly detecting:
- `/dev/tty*` (actual terminals)
- `/dev/pts/*` (pseudo-terminals in tmux/screen/ssh)
