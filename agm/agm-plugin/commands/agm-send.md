---
model: haiku
effort: low
content-hash: 5c7c1bac255cc1205d4422b205312aac524be681662b4fd99df67142bcb3d751
description: Send a message to an AGM session
argument-hint: "<session> --prompt \"<message>\""
allowed-tools: Bash(agm send msg *), Bash(agm session list *)
---

# AGM Send Message

I'll send a message to an AGM session.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract:
  - Session name (required, first positional argument)
  - `--prompt` or `-p` followed by the message text (required)
- If session name is missing:
  - Show: "Session name is required. Usage: /agm:send <session> --prompt \"message\""
  - Exit gracefully
- If prompt/message is missing:
  - Show: "Message is required. Usage: /agm:send <session> --prompt \"message\""
  - Exit gracefully
- Store session name as SESSION_NAME and message as MESSAGE

**Step 2: Send message**

- Run: `agm send msg "{SESSION_NAME}" --prompt "{MESSAGE}" --output json`
- Be careful to properly escape quotes in MESSAGE

**Step 3: Handle result**

- If exit code is 0:
  - Parse JSON output
  - Continue to Step 4
- If output contains "not found" or "no such session":
  - Show: "Session not found: {SESSION_NAME}"
  - Suggest: "Run /agm:list to see available sessions"
  - Exit gracefully
- If output contains "not active" or "not running":
  - Show: "Session '{SESSION_NAME}' is not active"
  - Suggest: "Resume it first with /agm:resume {SESSION_NAME}"
  - Exit gracefully
- If any other error:
  - Show the error output
  - Suggest: "Try running: agm admin doctor"
  - Exit gracefully

**Step 4: Show completion message**

```
Message sent to '{SESSION_NAME}'

Prompt: {MESSAGE}
```

If the JSON output contains delivery status or message ID, display those too.

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/dear-agent"
- If session not found: suggest `/agm:list`
- If session not active: suggest `/agm:resume`
