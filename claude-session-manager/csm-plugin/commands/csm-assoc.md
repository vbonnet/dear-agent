---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: "[session-name]"
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*), Bash(pwd)
---

# CSM Session Association

I'll associate this Claude session with CSM.

**Step 1: Determine session name source**
- Check if `$1` is provided (command argument)
- Check if `$TMUX_SESSION` environment variable is set
- Check if running in tmux (can run `tmux display-message -p '#S'`)
- If none available, show error and exit:
  ```
  Error: Not in tmux session and no session name provided
  Usage: /csm-tools:csm-assoc [session-name]
  ```

**Step 2: Try association (without --create)**
Run the appropriate command based on available session name source:
- If `$1` is provided: Run `csm associate "$1"`
- Else if `$TMUX_SESSION` is set: Run `csm associate "$TMUX_SESSION"`
- Else:
  - First get session name: Run `tmux display-message -p '#S'` and capture output as SESSION_NAME
  - Then run: `csm associate "$SESSION_NAME"`

Capture exit code and output.

**Step 3: Handle result**
- If exit code is 0:
  - Extract manifest path from output
  - Show success message (go to Step 4)
- If output contains "session not found":
  - Session needs to be created with --create flag
  - First get current directory: Run `pwd` and capture output as CURRENT_DIR
  - If session name not yet determined:
    - Get session name: Run `tmux display-message -p '#S'` and capture as SESSION_NAME
  - Then run: `csm associate "$SESSION_NAME" --create -C "$CURRENT_DIR"`
  - If this fails, show error and suggest: "Try running: csm doctor"
  - Show success message (go to Step 4)
- If any other error:
  - Show the error output
  - Suggest troubleshooting: "Try running: csm doctor"
  - Exit

**Step 4: Show completion message**
Extract session name and manifest path from command output and display:
```
✓ Session associated successfully

Session: {session_name_from_output}
Manifest: {manifest_path_from_output}

💡 To rename the Claude session to match: /rename {session_name_from_output}
```

**Note:** The skill cannot automatically invoke `/rename` because slash commands can only be executed from user input, not from Claude's responses. Users must manually type the `/rename` command if they want to rename the Claude session.

**Step 4b: Create ready-file signal**

After displaying success message, create ready-file to signal CSM that Claude is ready:

```bash
# Create ~/.csm/ directory if missing
mkdir -p "$HOME/.csm"

# Extract session name from association output (from Step 4)
SESSION_NAME="{session_name_from_step_4}"
MANIFEST_PATH="{manifest_path_from_step_4}"

# Get current timestamp (portable format for Linux and macOS)
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Get CSM version
CSM_VERSION=$(csm --version 2>/dev/null | head -1 || echo "unknown")

# Create ready-file with JSON diagnostics
cat > "$HOME/.csm/ready-$SESSION_NAME" <<EOF
{
  "status": "ready",
  "ready_at": "$TIMESTAMP",
  "session_name": "$SESSION_NAME",
  "manifest_path": "$MANIFEST_PATH",
  "csm_version": "$CSM_VERSION",
  "signals_detected": ["association_complete"]
}
EOF
```

**Note**: This signals CSM's WaitForClaudeReady() that Claude has completed initialization and is ready for use.

**Error Handling**:
- If csm not found: "Install csm from github.com/user/ai-tools"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** CSM auto-detects Claude UUID from ~/.claude/history.jsonl
