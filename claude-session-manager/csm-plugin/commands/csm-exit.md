---
content-hash: PLACEHOLDER
description: Archive CSM session (manual exit required)
allowed-tools: Bash(csm get-uuid:*), Bash(csm archive:*), Bash(tmux display-message:*)
---

# CSM Exit

I'll archive the current CSM session. You'll need to manually exit Claude afterward.

**Step 1: Get current tmux session name**
- Run: `tmux display-message -p '#S'`
- Capture tmux session name in variable
- If command fails or output is empty:
  - Show error: "❌ Not running in tmux session"
  - Show message: "Use /exit manually to exit Claude"
  - Exit gracefully

**Step 2: Verify CSM association**
- Run: `csm get-uuid <session-name>`
- Check if command succeeds (exit code 0)
- If command fails:
  - Show error: "❌ Session not associated with CSM"
  - Show message: "Run /csm-assoc first to associate this session"
  - Exit gracefully

**Step 3: Archive the session**
- Run: `csm archive <session-name> --force`
- Capture both output and exit code
- Check if archive succeeded

**Step 4: Handle archive result**
- If output contains "already archived":
  - Show warning: "⚠️  Session already archived"
  - Continue to completion message
- If exit code is not 0:
  - Show archive output (contains error details)
  - Show error: "❌ Archive failed"
  - Show message: "Fix the issue above and try again"
  - Exit gracefully
- If exit code is 0:
  - Show success: "✓ Session archived"

**Step 5: Completion message**
- Show final message:
  ```
  ✓ Session archived successfully

  To exit Claude, please run: /exit
  ```
- Note: Programmatic exit is not currently supported by Claude Code API
