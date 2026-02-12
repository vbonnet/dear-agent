---
content-hash: PLACEHOLDER
description: Exit Claude and archive AGM session
allowed-tools: Bash(agm get-uuid:*), Bash(tmux display-message:*), Bash(echo:*)
---

# AGM Exit

I'll help you exit Claude and show how to archive the AGM session.

**Step 1: Verify running in tmux and get session name**

Execute these checks using separate tool calls. Do NOT use bash if/elif/else conditionals.

**1.1: Check TMUX environment**
- Run: `echo "$TMUX"`
- Capture the output

**1.2: Analyze TMUX status**
- If output from 1.1 is empty or not set:
  - Show error: "❌ Not running in tmux session"
  - Show message: "Use /exit to exit Claude"
  - Exit gracefully (do not proceed to Step 2)
- If output from 1.1 is non-empty (TMUX detected):
  - Continue to check 1.3

**1.3: Get session name**
- Run: `tmux display-message -p '#S'`
- Capture output as SESSION_NAME
- Proceed to Step 2

**Note**: Make separate bash calls, analyze results in your reasoning layer. Do NOT use conditional logic in bash.

**Step 2: Verify AGM association**
- Run: `agm get-uuid "{session-name-from-step-1}"` (using session name from Step 1)
- This checks if the session is associated with AGM
- Capture UUID and session name for display
- If exit code is not 0:
  - Show warning: "⚠️  Session not associated with AGM"
  - Show message: "You can still use /exit to exit Claude"
  - Skip to Step 3 anyway (show exit instructions)

**Step 3: Show exit instructions**
- Show final message:
  ```
  To exit and archive this AGM session:

  1. Type: /exit
     (This will exit Claude and return you to the shell)

  2. Then run: agm session archive {session-name}
     (This will archive the session in AGM)

  Session: {session-name}
  UUID: {uuid}

  Note: AGM 2.0 requires manual archiving after exit.
  Active sessions cannot be archived while Claude is running.
  ```

**Error Handling**:
- If not in tmux: Show message to use /exit
- If not associated with AGM: Show message but still provide /exit instructions
- If agm command not found: "Install agm from github.com/vbonnet/ai-tools"
