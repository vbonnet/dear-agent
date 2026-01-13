---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: "{session-name}"
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*), Bash(pwd), Bash(echo:*)
---

# CSM Session Association

I'll associate this Claude session with CSM.

**Step 1: Determine session name source**
- Check if session name is provided as argument (from $ARGUMENTS)
- If argument provided: Use it as SESSION_NAME, skip to Step 2
- Check if `$TMUX` environment variable is set: Run `echo "$TMUX"`
- If `$TMUX` is set:
  - Run `tmux display-message -p '#S'` and capture output as SESSION_NAME
  - Continue to Step 2
- If `$TMUX` is empty/not set and no argument provided:
  - Show error: "❌ Not in tmux session and no session name provided"
  - Show message: "Usage: /csm-tools:csm-assoc {session-name}"
  - Exit gracefully (do not attempt tmux commands)

**Step 2: Try association (without --create)**
Run the appropriate command using SESSION_NAME from Step 1.
IMPORTANT: Always use `--no-lock` flag to avoid deadlock with csm new.

- Run: `csm associate "{SESSION_NAME}" --no-lock`
- Capture exit code and output.

**Step 3: Handle result**
- If exit code is 0:
  - Extract manifest path from output
  - Continue to Step 4
- If output contains "session not found":
  - Session needs to be created with --create flag
  - Run `pwd` and capture output as CURRENT_DIR
  - Run: `csm associate "{SESSION_NAME}" --create --no-lock -C "{CURRENT_DIR}"`
    (using SESSION_NAME from Step 1, not from tmux again)
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
