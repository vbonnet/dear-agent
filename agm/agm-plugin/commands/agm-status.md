---
model: haiku
effort: low
content-hash: a612f2c71d2d9ccee5199d3381292a3bbbe1f9aad55428362e3aa67813c6fff5
description: Get detailed status of an AGM session
argument-hint: "{session-name}"
allowed-tools: Bash(agm status *), Bash(agm session list *)
---

# AGM Session Status

I'll get the status of an AGM session.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract session name
- If $ARGUMENTS is empty or whitespace only:
  - Run: `agm status --output json`
  - This shows the status of the currently associated session
  - If this fails, show: "No session specified and no active association. Usage: /agm:status <session-name>"
  - If this fails, also suggest: "Run /agm:list to see available sessions"
  - Exit gracefully on failure
- If session name is provided:
  - Store as SESSION_NAME
  - Run: `agm status --output json "{SESSION_NAME}"`

**Step 2: Handle result**

- If exit code is not 0:
  - If output contains "not found" or "no such session":
    - Show: "Session not found: {SESSION_NAME}"
    - Suggest: "Run /agm:list to see available sessions"
  - Otherwise show the error output
  - Suggest: "Try running: agm admin doctor"
  - Exit gracefully

**Step 3: Display status summary**

Parse the JSON output and display a clear summary:

```
Session: {name}
Status:  {state}
Harness: {harness}
Project: {working_directory}
UUID:    {uuid}
Created: {created_at}
Updated: {updated_at}
```

If additional fields are present (tokens, messages, etc.), display them too.

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/ai-tools"
- If session not found: suggest `/agm:list` to see available sessions
