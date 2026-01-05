---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: [session-name]
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*), Bash(pwd)
---

# CSM Session Association

I'll associate this Claude session with CSM.

**Step 1: Determine session name**
- Check if `$1` is provided
- If provided, use `$1` as session name
- If empty, auto-detect from tmux environment variable `$TMUX_SESSION`
- If `$TMUX_SESSION` is empty, run: !`tmux display-message -p '#S'`
- If not in tmux and no args provided, show error:
  ```
  Error: Not in tmux session and no session name provided
  Usage: /csm-tools:csm-assoc [session-name]
  ```

**Step 2: Get current directory**
- Run: !`pwd`
- Store as working directory for project context

**Step 3: Try association (without --create)**
- Run: !`csm associate $session_name`
- Capture exit code and output

**Step 4: Handle result**
- If exit code is 0:
  - Extract manifest path from output
  - Show success message (go to Step 5)
- If output contains "session not found":
  - Session needs to be created
  - Run: !`csm associate $session_name --create -C $working_dir`
  - If this fails, show error and suggest: "Try running: csm doctor"
  - Show success message (go to Step 5)
- If any other error:
  - Show the error output
  - Suggest troubleshooting: "Try running: csm doctor"
  - Exit

**Step 5: Show completion message**
```
✓ Session associated successfully

Session: $session_name
Manifest: $manifest_path

To keep Claude and tmux session names synchronized, run:
  /rename $session_name
```

**Error Handling**:
- If csm not found: "Install csm from github.com/user/ai-tools"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** CSM auto-detects Claude UUID from ~/.claude/history.jsonl
