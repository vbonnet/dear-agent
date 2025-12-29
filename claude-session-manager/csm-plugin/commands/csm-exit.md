---
content-hash: PLACEHOLDER
description: Archive CSM session and exit Claude gracefully
allowed-tools: Bash(csm get-uuid:*), Bash(csm archive:*), Bash(tmux display-message:*), Bash(tmux send-keys:*), Bash(echo:*), Bash(read:*), Bash(sleep:*), Bash(grep:*), Bash(test:*)
---

# CSM Exit

I'll archive the current CSM session and exit Claude gracefully.

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
  - Prompt user: "Exit anyway? (y/n): "
  - If response is not 'y' or 'Y':
    - Show: "Cancelled."
    - Exit with code 0
- If exit code is not 0:
  - Show archive output (contains error details)
  - Show error: "❌ Archive failed - NOT exiting Claude"
  - Show message: "Fix the issue above and try again, or exit manually with /exit"
  - Exit with code 1
- If exit code is 0 and not already archived:
  - Show success: "✓ Session archived"

**Step 6: Get tmux target**
- Run: `tmux display-message -p '#S:#I.#P'`
- Capture target (format: session:window.pane)

**Step 7: Schedule exit command in background**
- Show progress: "🚪 Scheduling Claude exit..."
- Run in background (using `&`):
  ```
  (sleep 2 && tmux send-keys -t <target> "/exit" && sleep 0.1 && tmux send-keys -t <target> C-m) &
  ```
- The 2-second delay allows this slash command to complete first
- After delay, send "/exit" followed by Enter
- Show final message: "✓ Exit scheduled (Claude will close in 2 seconds)"
