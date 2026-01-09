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
  - Continue to Step 4
- If output contains "session not found":
  - Session needs to be created with --create flag
  - First get current directory: Run `pwd` and capture output as CURRENT_DIR
  - If session name not yet determined:
    - Get session name: Run `tmux display-message -p '#S'` and capture as SESSION_NAME
  - Then run: `csm associate "$SESSION_NAME" --create -C "$CURRENT_DIR"`
  - If this fails, show error and suggest: "Try running: csm doctor", then Exit
  - If successful, continue to Step 4
- If any other error:
  - Show the error output
  - Suggest troubleshooting: "Try running: csm doctor"
  - Exit

**Step 4: Extract session info**
Extract session name and manifest path from the successful `csm associate` output for use in next steps.

**Step 5: Create ready-file signal**
Create ready-file to signal CSM that Claude is ready. Execute these bash commands in sequence:

1. Run `mkdir -p ~/.csm` to ensure directory exists
2. Run `date -u +%Y-%m-%dT%H:%M:%SZ` to get current timestamp
3. Run `csm --version 2>/dev/null | head -1 || echo "unknown"` to get CSM version

4. Create the ready-file at `~/.csm/ready-{SESSION_NAME}` (replace {SESSION_NAME} with the actual session name from Step 4) with a bash heredoc. The file must contain valid JSON with these fields:
   - status: "ready"
   - ready_at: (the timestamp from step 2)
   - session_name: (the session name from Step 4)
   - manifest_path: (the manifest path from Step 4)
   - csm_version: (the version from step 3)
   - signals_detected: ["association_complete"]

Example bash command (fill in the actual values):
```bash
cat > ~/.csm/ready-my-session <<'EOF'
{
  "status": "ready",
  "ready_at": "2026-01-09T00:00:00Z",
  "session_name": "my-session",
  "manifest_path": "/home/user/src/sessions/session-my-session/manifest.yaml",
  "csm_version": "v0.1.0",
  "signals_detected": ["association_complete"]
}
EOF
```

**Step 6: Show completion message**
After successfully creating the ready-file, display:
```
✓ Session associated successfully

Session: {session_name}
Manifest: {manifest_path}

💡 To rename the Claude session to match: /rename {session_name}
```

**Note:** The skill cannot automatically invoke `/rename` because slash commands can only be executed from user input, not from Claude's responses. Users must manually type the `/rename` command if they want to rename the Claude session.

**Note**: The ready-file signals CSM's WaitForClaudeReady() that Claude has completed initialization and is ready for use.

**Error Handling**:
- If csm not found: "Install csm from github.com/user/ai-tools"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** CSM auto-detects Claude UUID from ~/.claude/history.jsonl
