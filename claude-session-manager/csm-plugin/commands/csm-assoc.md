---
content-hash: 1a62b783e62146bddef4c8227a3dfd1377971a29e66518b0f141a6c06bab2cd2
description: Associate Claude session with CSM (auto-detects tmux session)
argument-hint: [session-name]
allowed-tools: Bash(csm associate:*), Bash(tmux display-message:*)
---

# CSM Session Association

I'll associate this Claude session with CSM.

**Step 1: Determine session name**
- Check if `{{args}}` is provided
- If empty, run: `tmux display-message -p '#S'` to auto-detect
- If not in tmux and no args, show error: "Usage: /csm-tools:csm-assoc [session-name]"

**Step 2: Associate with CSM**
- Run: `csm associate <session-name>`
- Show confirmation with manifest path
- Note: CSM auto-detects the Claude session UUID from history
