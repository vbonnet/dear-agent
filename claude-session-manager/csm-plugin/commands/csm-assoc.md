---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: "{session-name}"
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*), Bash(pwd)
---

# CSM Session Association

I'll associate this Claude session with CSM.

**Step 1: Determine session name source**
- Check if session name is provided as argument (from $ARGUMENTS)
- Check if `TMUX_SESSION` environment variable is set
- Check if running in tmux (can run `tmux display-message -p '#S'`)
- If none available, show error and exit:
  ```
  Error: Not in tmux session and no session name provided
  Usage: /csm-tools:csm-assoc {session-name}
  ```

**Step 2: Try association (without --create)**
Run the appropriate command based on available session name source:
- If argument provided: Run `csm associate "{session-name-from-arguments}"`
- Else if `TMUX_SESSION` is set: Run `csm associate "{tmux-session-env}"`
- Else:
  - First get session name: Run `tmux display-message -p '#S'` and capture output
  - Then run: `csm associate "{session-name-from-tmux}"`

Capture exit code and output.

**Step 3: Handle result**
- If exit code is 0:
  - Extract manifest path from output
  - Continue to Step 4
- If output contains "session not found":
  - Session needs to be created with --create flag
  - First get current directory: Run `pwd` and capture output
  - If session name not yet determined:
    - Get session name: Run `tmux display-message -p '#S'` and capture output
  - Then run: `csm associate "{session-name-from-step-2}" --create -C "{current-dir-from-pwd}"`
  - If this fails, show error and suggest: "Try running: csm doctor", then Exit
  - If successful, continue to Step 4
- If any other error:
  - Show the error output
  - Suggest troubleshooting: "Try running: csm doctor"
  - Exit

**Step 4: Show completion message**
Extract session name and manifest path from the `csm associate` output and display:
```
✓ Session associated successfully

Session: {session_name}
Manifest: {manifest_path}

💡 To rename the Claude session to match: /rename {session_name}
```

**Note:** The skill cannot automatically invoke `/rename` because slash commands can only be executed from user input, not from Claude's responses. Users must manually type the `/rename` command if they want to rename the Claude session.

**Note**: The `csm associate` command automatically creates a ready-file signal at `~/.csm/ready-{session_name}` to notify CSM that Claude initialization is complete. This enables `csm new` to detect when Claude is ready without fragile text-matching.

**Error Handling**:
- If csm not found: "Install csm from github.com/user/ai-tools"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** CSM auto-detects Claude UUID from ~/.claude/history.jsonl
