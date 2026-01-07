---
content-hash: PLACEHOLDER
description: Archive CSM session (manual exit required)
allowed-tools: Bash(csm get-uuid:*), Bash(csm archive:*), Bash(tmux display-message:*)
---

# CSM Exit

I'll archive the current CSM session. You'll need to manually exit Claude afterward.

**Step 1: Verify running in tmux and get session name**
- Run: `tmux display-message -p '#S'` and capture output as SESSION_NAME
- If command fails or output is empty:
  - Show error: "❌ Not running in tmux session"
  - Show message: "Use /exit manually to exit Claude"
  - Exit gracefully
- Otherwise, continue to Step 2

**Step 2: Verify CSM association**
- Run: `csm get-uuid "$SESSION_NAME"` (using session name from Step 1)
- This checks if the session is associated with CSM
- If exit code is not 0:
  - Show error: "❌ Session not associated with CSM"
  - Show message: "Run /csm-tools:csm-assoc first to associate this session"
  - Exit gracefully

**Step 3: Archive session**
- Run: `csm archive "$SESSION_NAME" --force` (using session name from Step 1)
- This archives the session and cleans up manifests
- If exit code is not 0:
  - Show error: "❌ Archive failed"
  - Show message: "Check csm doctor for system health"
  - Exit gracefully
- If successful: Continue to completion message

**Step 4: Completion message**
- Show final message:
  ```
  ✓ Session archived successfully

  To exit Claude, please run: /exit
  ```
- Note: Programmatic exit is not currently supported by Claude Code API
