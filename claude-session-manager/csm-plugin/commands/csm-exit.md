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

**Step 2: Get current session UUID**
- Run: `csm get-uuid`
- Capture output in variable
- If command fails:
  - Show error: "❌ Could not determine session UUID"
  - Show message: "Run /csm-assoc first to associate this session"
  - Exit with code 1

**Step 3: Archive the session**
- Show progress: "🔄 Archiving session..."
- Run: `csm archive <uuid> --force`
- Capture both output and exit code
- Check if archive succeeded

**Step 4: Handle archive result**
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

**Step 5: Get tmux target**
- Run: `tmux display-message -p '#S:#I.#P'`
- Capture target (format: session:window.pane)

**Step 6: Send exit command**
- Show progress: "🚪 Exiting Claude..."
- Wait 0.5 seconds (brief delay for archive completion)
- Run: `tmux send-keys -t <target> "/exit"`
- Wait 0.1 seconds (tmux pattern from research)
- Run: `tmux send-keys -t <target> C-m`
