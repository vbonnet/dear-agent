# Hook-Based Readiness Detection Design

## Problem Statement

The current `agm new` implementation uses text parsing to detect when Claude CLI is ready to accept commands. This approach is fragile and has caused multiple regressions:

1. **Control mode timing issues**: Control mode only sees NEW output after attachment, missing already-displayed prompts
2. **Race conditions**: Commands sent before Claude is actually ready
3. **Permission prompts**: Commands blocked by permission approval dialogs
4. **Parsing brittleness**: Prompt pattern matching fails in edge cases

## Current Flow (Text-Parsing Based)

```
agm new session-name
  ├─> Create tmux session
  ├─> Start Claude CLI
  ├─> Wait for Claude prompt (TEXT PARSING)
  ├─> Wait 2 seconds for SessionStart hooks
  ├─> Run InitSequence:
  │     ├─> waitForClaudePrompt() (TEXT PARSING)
  │     ├─> Send /rename command
  │     ├─> waitForNewPrompt() (TEXT PARSING)
  │     ├─> Send /agm:agm-assoc command
  │     └─> Return (caller waits for ready-file)
  └─> Wait for ready-file from /agm:agm-assoc
```

**Problems**:
- 3 separate text-parsing operations, each prone to failure
- No deterministic signal for readiness
- Race conditions between prompt detection and command sending

## Proposed Solution: Hook-Based Ready Signal

### Architecture

Replace text parsing with deterministic ready-file signals:

```
agm new session-name
  ├─> Clean up old ready-files
  ├─> Create session marker file: ~/.agm/pending-{session-name}
  ├─> Create tmux session
  ├─> Start Claude CLI with AGM_SESSION_NAME env var
  ├─> Wait for claude-ready-file: ~/.agm/claude-ready-{session-name}
  │     (Created by SessionStart hook when Claude is ready)
  ├─> Run InitSequence:
  │     ├─> Send /rename command (no waiting, fire-and-forget style)
  │     ├─> Send /agm:agm-assoc command
  │     └─> Return
  └─> Wait for ready-file from /agm:agm-assoc: ~/.agm/ready-{session-name}
```

### Key Changes

1. **SessionStart Hook**: User configures hook that creates ready-file
2. **Environment variable**: `AGM_SESSION_NAME` passed to Claude CLI
3. **Marker files**: `~/.agm/pending-{session}` → `~/.agm/claude-ready-{session}`
4. **No text parsing**: All synchronization via file signals
5. **Simplified InitSequence**: Just sends commands, no waiting/parsing

### SessionStart Hook (User Configuration)

Users add this to their Claude Code hooks config:

```bash
#!/bin/bash
# ~/.config/claude/hooks/session-start-agm.sh

# Check if this is an AGM-managed session
if [ -n "$AGM_SESSION_NAME" ]; then
    # Signal that Claude is ready for commands
    mkdir -p ~/.agm

    # Remove pending marker and create ready marker
    rm -f ~/.agm/pending-${AGM_SESSION_NAME}
    touch ~/.agm/claude-ready-${AGM_SESSION_NAME}

    # Debug logging
    echo "[AGM] Claude ready signal created for session: ${AGM_SESSION_NAME}" >&2
fi
```

Hook configuration in `~/.config/claude/config.yaml`:

```yaml
hooks:
  SessionStart:
    - name: agm-ready-signal
      command: ~/.config/claude/hooks/session-start-agm.sh
```

### Code Changes Required

#### 1. Update `cmd/agm/new.go` - Pass Environment Variable

```go
// Before starting Claude, set environment variable
claudeCmd := fmt.Sprintf("AGM_SESSION_NAME=%s claude --add-dir '%s' && exit",
    sessionName,
    projectPath)
```

#### 2. Create Ready-File Management Functions

```go
// internal/tmux/ready_file.go (new file)

package tmux

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// ClaudeReadyFile manages the ready-file created by SessionStart hook
type ClaudeReadyFile struct {
    sessionName string
}

func NewClaudeReadyFile(sessionName string) *ClaudeReadyFile {
    return &ClaudeReadyFile{sessionName: sessionName}
}

func (r *ClaudeReadyFile) PendingPath() string {
    homeDir, _ := os.UserHomeDir()
    return filepath.Join(homeDir, ".agm", fmt.Sprintf("pending-%s", r.sessionName))
}

func (r *ClaudeReadyFile) ReadyPath() string {
    homeDir, _ := os.UserHomeDir()
    return filepath.Join(homeDir, ".agm", fmt.Sprintf("claude-ready-%s", r.sessionName))
}

// CreatePending creates the pending marker file
func (r *ClaudeReadyFile) CreatePending() error {
    homeDir, _ := os.UserHomeDir()
    agmDir := filepath.Join(homeDir, ".agm")
    if err := os.MkdirAll(agmDir, 0755); err != nil {
        return fmt.Errorf("failed to create .agm directory: %w", err)
    }

    f, err := os.Create(r.PendingPath())
    if err != nil {
        return fmt.Errorf("failed to create pending file: %w", err)
    }
    f.Close()
    return nil
}

// WaitForReady waits for the SessionStart hook to create the ready-file
func (r *ClaudeReadyFile) WaitForReady(timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for time.Now().Before(deadline) {
        // Check if ready file exists
        if _, err := os.Stat(r.ReadyPath()); err == nil {
            return nil
        }

        <-ticker.C
    }

    return fmt.Errorf("timeout waiting for Claude ready signal after %v", timeout)
}

// Cleanup removes both pending and ready files
func (r *ClaudeReadyFile) Cleanup() error {
    os.Remove(r.PendingPath())  // Ignore errors
    os.Remove(r.ReadyPath())    // Ignore errors
    return nil
}
```

#### 3. Simplify `internal/tmux/init_sequence.go`

Remove all text-parsing logic:

```go
func (seq *InitSequence) Run() error {
    return withTmuxLock(func() error {
        // Start control mode
        ctrl, err := StartControlMode(seq.SessionName)
        if err != nil {
            return fmt.Errorf("failed to start control mode: %w", err)
        }
        defer ctrl.Close()

        // Send /rename command (fire-and-forget)
        if err := seq.sendCommand(ctrl, fmt.Sprintf("/rename %s", seq.SessionName)); err != nil {
            return fmt.Errorf("failed to send /rename: %w", err)
        }

        // Small delay to ensure /rename is processed before /agm:agm-assoc
        time.Sleep(200 * time.Millisecond)

        // Send /agm:agm-assoc command (fire-and-forget)
        if err := seq.sendCommand(ctrl, fmt.Sprintf("/agm:agm-assoc %s", seq.SessionName)); err != nil {
            return fmt.Errorf("failed to send /agm:agm-assoc: %w", err)
        }

        return nil
    })
}

// sendCommand sends a command using paste buffer (all at once) then ENTER
func (seq *InitSequence) sendCommand(ctrl *ControlModeSession, command string) error {
    // Send command text using -l flag (literal/paste mode)
    sendLiteralCmd := fmt.Sprintf("send-keys -t %s -l %q", seq.SessionName, command)
    if err := ctrl.SendCommand(sendLiteralCmd); err != nil {
        return fmt.Errorf("failed to send command text: %w", err)
    }

    // Delay to ensure text is received before Enter
    time.Sleep(100 * time.Millisecond)

    // Send Enter to execute
    sendEnterCmd := fmt.Sprintf("send-keys -t %s C-m", seq.SessionName)
    if err := ctrl.SendCommand(sendEnterCmd); err != nil {
        return fmt.Errorf("failed to send Enter: %w", err)
    }

    return nil
}
```

**Remove these functions entirely**:
- `waitForClaudePrompt()`
- `waitForNewPrompt()`
- `checkCurrentPromptState()`
- `containsClaudePromptPattern()`

#### 4. Update `cmd/agm/new.go` - Use Ready-File

Replace the "Wait for Claude Prompt" phase:

```go
// OLD: Wait for Claude Prompt (text parsing)
debug.Log("Waiting for Claude prompt to appear (timeout: 30s)")
// ... text parsing logic ...

// NEW: Wait for Claude ready signal (hook-based)
debug.Log("Waiting for Claude ready signal from SessionStart hook (timeout: 30s)")
claudeReady := tmux.NewClaudeReadyFile(sessionName)
if err := claudeReady.WaitForReady(30 * time.Second); err != nil {
    return fmt.Errorf("Claude failed to become ready: %w", err)
}
debug.Log("Claude ready signal received")
```

## Benefits

1. **Deterministic**: File-based signaling has clear semantics
2. **No text parsing**: Eliminates all parsing brittleness
3. **Testable**: Can simulate hook behavior in tests
4. **Debuggable**: Ready-files visible in filesystem for inspection
5. **Extensible**: Can add more hooks/signals as needed

## Migration Path

### Phase 1: Add Hook Support (Backward Compatible)

- Add ready-file wait logic
- Keep existing text-parsing as fallback
- Document hook configuration for users
- Test both code paths

### Phase 2: Deprecate Text Parsing

- Make hook-based approach the default
- Warn if hook not configured
- Remove text-parsing code

### Phase 3: Hook Required

- Require hook configuration
- Remove all text-parsing code
- Simplify InitSequence completely

## Testing Strategy

1. **Unit tests**: Mock ready-file creation/detection
2. **Integration tests**: Test with actual SessionStart hook
3. **Failure modes**: Test timeout scenarios
4. **Hook not configured**: Clear error message
5. **Multiple sessions**: Ensure session isolation

## Documentation Updates

1. **README**: Add hook configuration instructions
2. **INSTALL**: Include hook setup steps
3. **TROUBLESHOOTING**: Document ready-file inspection
4. **UPGRADE**: Migration guide from text-parsing version

## Alternative Considered: Permission-Free Mode

Instead of hooks, we could try to make commands run without permission prompts. However:

- This requires understanding Claude's permission system
- May not be possible for security reasons
- Hooks are more general-purpose and reliable

## Open Questions

1. **Hook ordering**: Can we guarantee SessionStart hook runs before we send commands?
   - **Answer**: SessionStart hooks run synchronously during initialization, blocking the prompt

2. **Hook failures**: What if user doesn't configure hook?
   - **Answer**: Clear error message with setup instructions

3. **Multiple hooks**: What if user has multiple SessionStart hooks?
   - **Answer**: All hooks run, order may vary, but our hook is independent

4. **Hook timing**: Is Claude truly ready when SessionStart hook completes?
   - **Answer**: Need empirical testing to confirm

## Implementation Priority

1. ✅ Design document (this file)
2. [ ] Create ready-file management module
3. [ ] Add hook configuration documentation
4. [ ] Update new.go to use ready-file wait
5. [ ] Simplify InitSequence (remove text parsing)
6. [ ] Add tests
7. [ ] Test in Docker environment
8. [ ] Update all documentation
