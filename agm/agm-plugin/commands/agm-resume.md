---
model: haiku
effort: low
content-hash: 5c1132cc470bc05b974e1fe7da8d2b8f6fa1748b49ab26927e3a997dac1254c5
description: Resume an existing AGM session
argument-hint: "<session-name>"
allowed-tools: Bash(agm session resume *), Bash(agm session list *)
---

# AGM Resume Session

I'll resume an existing AGM session.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract session name
- If $ARGUMENTS is empty or whitespace only:
  - Show: "Session name is required. Usage: /agm:resume <session-name>"
  - Suggest: "Run /agm:list to see available sessions"
  - Exit gracefully
- Store the session name as SESSION_NAME

**Step 2: Resume session**

- Run: `agm session resume "{SESSION_NAME}" --output json`

**Step 3: Handle result**

- If exit code is 0:
  - Parse JSON output
  - Continue to Step 4
- If output contains "not found" or "no such session":
  - Show: "Session not found: {SESSION_NAME}"
  - Run: `agm session list --output json`
  - If sessions exist, show available session names as suggestions
  - If no sessions exist: "No sessions found. Create one with /agm:new <name>"
  - Exit gracefully
- If output contains "already active" or "already running":
  - Show: "Session '{SESSION_NAME}' is already active"
  - Exit gracefully
- If any other error:
  - Show the error output
  - Suggest: "Try running: agm admin doctor"
  - Exit gracefully

**Step 4: Show completion message**

```
Session resumed successfully

Name:    {session_name}
Status:  active
```

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/ai-tools"
- If session not found: list available sessions as suggestions
