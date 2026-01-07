---
content-hash: PLACEHOLDER
description: Archive CSM session (manual exit required)
allowed-tools: Bash(csm get-uuid:*), Bash(csm archive:*), Bash(tmux display-message:*)
---

# CSM Exit

I'll archive the current CSM session. You'll need to manually exit Claude afterward.

**Step 1: Verify running in tmux**
- Run: !`tmux display-message -p '#S'`
- If command fails or output is empty:
  - Show error: "❌ Not running in tmux session"
  - Show message: "Use /exit manually to exit Claude"
  - Exit gracefully
- Otherwise, continue to Step 2

**Step 2: Verify CSM association**
- Run: !`csm get-uuid $(tmux display-message -p '#S')`
- This checks if the session is associated with CSM
- If exit code is not 0:
  - Show error: "❌ Session not associated with CSM"
  - Show message: "Run /csm-tools:csm-assoc first to associate this session"
  - Exit gracefully

**Step 3: Archive session**
- Run: !`csm archive $(tmux display-message -p '#S') --force`
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
