---
content-hash: PLACEHOLDER
description: Archive CSM session (manual exit required)
allowed-tools: Bash(csm get-uuid:*), Bash(csm archive:*), Bash(tmux display-message:*)
---

# CSM Exit

I'll archive the current CSM session. You'll need to manually exit Claude afterward.

**Step 1: Get and verify tmux session**
- Run: !`tmux display-message -p '#S'`
- If command fails or output is empty:
  - Show error: "❌ Not running in tmux session"
  - Show message: "Use /exit manually to exit Claude"
  - Exit gracefully
- Otherwise, capture the session name for next steps

**Step 2: Verify CSM association and archive**
- Run: !`session_name=$(tmux display-message -p '#S') && csm get-uuid "$session_name" >/dev/null 2>&1 && csm archive "$session_name" --force`
- This command:
  1. Gets the current tmux session name
  2. Verifies it's associated with CSM (csm get-uuid)
  3. Archives the session if verification succeeds
- Check the exit code to determine outcome

**Step 3: Handle result**
- If exit code is 0:
  - Show success: "✓ Session archived successfully"
  - Continue to completion message
- If exit code is not 0:
  - Try to diagnose which step failed
  - Run: !`session_name=$(tmux display-message -p '#S') && csm get-uuid "$session_name"`
  - If this fails:
    - Show error: "❌ Session not associated with CSM"
    - Show message: "Run /csm-assoc first to associate this session"
    - Exit gracefully
  - If this succeeds but original command failed:
    - Show error: "❌ Archive failed"
    - Show message: "Check csm logs for details"
    - Exit gracefully

**Step 4: Completion message**
- Show final message:
  ```
  ✓ Session archived successfully

  To exit Claude, please run: /exit
  ```
- Note: Programmatic exit is not currently supported by Claude Code API
