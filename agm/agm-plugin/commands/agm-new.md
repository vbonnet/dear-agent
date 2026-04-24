---
model: haiku
effort: low
content-hash: 4ede339747a8a4ff9b6ae96884fcb6fe231f1a0b2a2c7528bdf1bb02a7f81a09
description: Create a new AGM session
argument-hint: "<session-name> [--harness TYPE] [--project PATH]"
allowed-tools: Bash(agm session new *), Bash(pwd)
---

# AGM New Session

I'll create a new AGM session.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract:
  - Session name (required positional argument)
  - `--harness TYPE` (optional, defaults to "claude-code")
  - `--project PATH` (optional, defaults to current working directory)
- If no session name is provided:
  - Run: `pwd`
  - Auto-generate name as `claude-{basename of pwd}`
  - Inform user: "No name provided, using: {generated-name}"

**Step 2: Build and run command**

Construct the command with extracted arguments:

- Base: `agm session new "{SESSION_NAME}" --output json`
- If harness was specified: add `--harness "{HARNESS}"`
- If project was specified: add `-C "{PROJECT}"`

Run the constructed command.

**Step 3: Handle result**

- If exit code is 0:
  - Parse JSON output
  - Continue to Step 4
- If output contains "already exists":
  - Show: "Session '{SESSION_NAME}' already exists"
  - Suggest: "Use /agm:resume {SESSION_NAME} to resume it, or choose a different name"
  - Exit gracefully
- If any other error:
  - Show the error output
  - Suggest: "Try running: agm admin doctor"
  - Exit gracefully

**Step 4: Show completion message**

```
Session created successfully

Name:    {session_name}
Harness: {harness}
Project: {project}

To resume later: /agm:resume {session_name}
To check status: /agm:status {session_name}
```

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/dear-agent"
- If session already exists: suggest resume or different name
- If project path invalid: show path error and suggest valid path
