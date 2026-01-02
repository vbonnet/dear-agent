---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: [session-name]
allowed-tools: Bash(csm associate:*)
---

# CSM Session Association

I'll associate this Claude session with CSM and rename it to match the tmux session name.

**Step 1: Determine session name**
- Check if `{{args}}` is provided
- If provided, use it as the session name
- If empty, auto-detect from tmux environment variable `$TMUX_SESSION`
- If `$TMUX_SESSION` is empty, run: `tmux display-message -p '#S'` to auto-detect
- If not in tmux and no args provided, show error and exit:
  ```
  Error: Not in tmux session and no session name provided
  Usage: /csm-tools:csm-assoc [session-name]
  ```

**Step 2: Associate session**
- Run: `csm associate <session-name> --create`
- This will:
  - Create a new manifest if it doesn't exist (using current directory as project)
  - Update existing manifest with current Claude UUID if it exists
  - Auto-detect Claude UUID from history
- Capture the full output

**Step 3: Handle result and show completion message**
- If exit code is non-zero:
  - Show the error output
  - Show troubleshooting hint: "Try running: csm doctor"
  - Exit the skill
- If exit code is 0:
  - Extract manifest path from output
  - Show this completion message (include the /rename reminder):
    ```
    ✓ Session associated successfully

    Session: <session-name>
    Manifest: <manifest-path>

    To keep Claude and tmux session names synchronized, run:
      /rename <session-name>
    ```
  - Note: Programmatic rename is not currently supported by Claude Code API

**Note:** The csm associate command auto-detects the Claude session UUID from ~/.claude/history.jsonl
