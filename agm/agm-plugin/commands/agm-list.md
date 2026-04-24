---
model: haiku
effort: low
content-hash: d53eead01f0afd0c0a61ba91076641fbaa745457f85b1dc4c16e0a02777bc984
description: List all AGM sessions with status summary
argument-hint: ""
allowed-tools: Bash(agm session list *)
---

# AGM List Sessions

I'll list all AGM sessions.

**Step 1: Fetch session list**

- Run: `agm session list --output json`
- Capture the JSON output.

**Step 2: Handle result**

- If exit code is not 0:
  - Show error: "Failed to list sessions"
  - Suggest: "Try running: agm admin doctor"
  - Exit gracefully
- If output is empty or `[]`:
  - Show: "No sessions found. Create one with /agm:new <name>"
  - Exit gracefully

**Step 3: Display results**

Parse the JSON array and display a table with columns:

| Name | Status | Harness | Project | Updated |
|------|--------|---------|---------|---------|

- **Name**: session name
- **Status**: session state (active, archived, etc.)
- **Harness**: harness type (claude-code, gemini-cli, etc.)
- **Project**: working directory (truncate long paths with `~`)
- **Updated**: last updated timestamp (human-readable relative time if possible)

Show a summary line at the bottom: "N sessions total (X active, Y archived)"

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/dear-agent"
- If JSON parse fails: show raw output and suggest `agm admin doctor`
