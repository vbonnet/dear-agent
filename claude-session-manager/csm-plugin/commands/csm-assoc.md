---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: "[session-name]"
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*), Bash(pwd), Bash(mkdir:*), Bash(date:*), Bash(cat:*), Bash(csm --version:*)
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

**Step 4: Create ready-file and show completion message**

First, extract session name and manifest path from the successful `csm associate` output.

Then create ready-file to signal CSM that Claude is ready:

1. Create `~/.csm/` directory: Run `mkdir -p ~/.csm`
2. Get current timestamp: Run `date -u +%Y-%m-%dT%H:%M:%SZ` and capture as TIMESTAMP
3. Get CSM version: Run `csm --version 2>/dev/null | head -1 || echo "unknown"` and capture as CSM_VERSION
4. Create ready-file using the extracted session name and manifest path from Step 2/3 output:

Write a bash script that creates `~/.csm/ready-{SESSION_NAME}` with this JSON content (replace {SESSION_NAME}, {MANIFEST_PATH}, {TIMESTAMP}, {CSM_VERSION} with actual values):
```json
{
  "status": "ready",
  "ready_at": "{TIMESTAMP}",
  "session_name": "{SESSION_NAME}",
  "manifest_path": "{MANIFEST_PATH}",
  "csm_version": "{CSM_VERSION}",
  "signals_detected": ["association_complete"]
}
```

Use a Write tool or bash heredoc to create this file. Example bash command:
```bash
cat > ~/.csm/ready-{SESSION_NAME} <<'EOF'
{JSON content with actual values}
EOF
```

After creating the ready-file, display success message:
```
✓ Session associated successfully

Session: {session_name_from_output}
Manifest: {manifest_path_from_output}

💡 To rename the Claude session to match: /rename {session_name_from_output}
```

**Note:** The skill cannot automatically invoke `/rename` because slash commands can only be executed from user input, not from Claude's responses. Users must manually type the `/rename` command if they want to rename the Claude session.

**Note**: The ready-file signals CSM's WaitForClaudeReady() that Claude has completed initialization and is ready for use.

**Error Handling**:
- If csm not found: "Install csm from github.com/user/ai-tools"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** CSM auto-detects Claude UUID from ~/.claude/history.jsonl
