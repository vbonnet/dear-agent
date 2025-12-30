---
content-hash: PLACEHOLDER
description: Archive CSM session (manual exit required)
allowed-tools: Bash(csm get-uuid:*), Bash(csm archive:*), Bash(tmux display-message:*), Bash(test:*)
---

# CSM Exit

I'll archive the current CSM session. You'll need to manually exit Claude afterward.

**Step 1: Check if running in tmux**
- Check if `$TMUX` environment variable is set
- If not set:
  - Show error: "❌ Not running in tmux session"
  - Show message: "Use /exit manually to exit Claude"
  - Exit with code 1

**Step 2: Get current session name**
- Run: `tmux display-message -p '#S'`
- Capture tmux session name in variable
- If command fails or output is empty:
  - Show error: "❌ Could not determine tmux session name"
  - Show message: "Ensure you're in a tmux session"
  - Exit with code 1

**Step 3: Verify CSM association**
- Run: `csm get-uuid`
- Check if command succeeds (exit code 0)
- If command fails:
  - Show error: "❌ Session not associated with CSM"
  - Show message: "Run /csm-assoc first to associate this session"
  - Exit with code 1

**Step 4: Archive the session**
- Show progress: "🔄 Archiving session..."
- Run: `csm archive <session-name> --force`
- Capture both output and exit code
- Check if archive succeeded

**Step 5: Handle archive result**
- If output contains "already archived":
  - Show warning: "⚠️  Session already archived"
  - Prompt user: "Continue anyway? (y/n): "
  - If response is not 'y' or 'Y':
    - Show: "Cancelled."
    - Exit with code 0
- If exit code is not 0:
  - Show archive output (contains error details)
  - Show error: "❌ Archive failed"
  - Show message: "Fix the issue above and try again"
  - Exit with code 1
- If exit code is 0 and not already archived:
  - Show success: "✓ Session archived"

**Step 6: Completion message**
- Show final message:
  ```
  ✓ Session archived successfully

  To exit Claude, please run: /exit
  ```
- Note: Programmatic exit is not currently supported by Claude Code API
