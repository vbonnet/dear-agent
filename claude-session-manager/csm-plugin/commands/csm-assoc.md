---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: [session-name]
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

To keep Claude and tmux session names synchronized, run:
  /rename {session_name_from_output}
```

**Error Handling**:
- If csm not found: "Install csm from github.com/user/ai-tools"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** CSM auto-detects Claude UUID from ~/.claude/history.jsonl
