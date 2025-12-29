---
content-hash: PLACEHOLDER
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: [session-name]
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*), Bash(pwd:*)
---

# CSM Session Association

I'll associate this Claude session with CSM.

**Step 1: Determine session name**
- Check if `{{args}}` is provided
- If empty, run: `tmux display-message -p '#S'` to auto-detect
- If not in tmux and no args, show error: "Usage: /csm-tools:csm-assoc [session-name]"

**Step 2: Get current working directory**
- Run: `pwd`
- Capture the current directory for project context

**Step 3: Attempt association**
- Run: `csm associate <session-name>`
- Capture exit code

**Step 4: Handle result**
- If exit code is 0:
  - Show success: "✓ Associated session"
  - Show manifest path from output
  - Exit successfully
- If output contains "session not found":
  - Session needs to be created
  - Run: `csm associate <session-name> --create -C <current-directory>`
  - Show success with manifest path
- If any other error:
  - Show error output
  - Exit with code 1

**Note:** CSM auto-detects the Claude session UUID from history
