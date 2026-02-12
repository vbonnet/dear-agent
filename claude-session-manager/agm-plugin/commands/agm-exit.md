---
content-hash: PLACEHOLDER
description: Exit Claude and archive AGM session automatically
allowed-tools: Bash(agm session archive:*), Bash(agm get-uuid:*), Bash(tmux display-message:*), Bash(echo:*)
---

# AGM Exit

I'll archive the current AGM session asynchronously. The session will exit automatically once you return to the prompt.

**Step 1: Verify running in tmux and get session name**

Execute these checks using separate tool calls. Do NOT use bash if/elif/else conditionals.

**1.1: Check TMUX environment**
- Run: `echo "$TMUX"`
- Capture the output

**1.2: Analyze TMUX status**
- If output from 1.1 is empty or not set:
  - Show error: "❌ Not running in tmux session"
  - Show message: "agm-exit requires tmux. Use /exit manually to exit Claude"
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
- If exit code is not 0:
  - Show error: "❌ Session not associated with AGM"
  - Show message: "Run /agm:agm-assoc first to associate this session"
  - Exit gracefully

**Step 3: Spawn async archive reaper**
- Run: `agm session archive "{session-name-from-step-1}" --async` (using session name from Step 1)
- This spawns a background reaper process that will:
  1. Wait for Claude to return to prompt (smart detection, not fixed interval)
  2. Send /exit command automatically
  3. Wait for pane to close
  4. Archive the session
- If exit code is not 0:
  - Show error: "❌ Failed to spawn async archive reaper"
  - Show stderr output for debugging
  - Show message: "Check agm doctor for system health"
  - Show fallback: "Or use manual exit: /exit then agm session archive {session-name}"
  - Exit gracefully
- If successful: Continue to completion message

**Step 4: Completion message**
- Show final message:
  ```
  ✓ Async archive started

  A background reaper is monitoring this session. Once you finish your
  current response and return to the prompt, the reaper will:
  1. Automatically send /exit to Claude
  2. Wait for the pane to close
  3. Archive the session

  You don't need to do anything - the session will exit automatically.
  Check the reaper log for progress details (path shown in output above).
  ```
- Note: The reaper runs as a detached background process and survives even if the parent shell exits
