---
description: List all AGM sessions
argument-hint: "[--status active|archived|all] [--harness claude-code|gemini-cli|codex|opencode|all] [--limit N] [--offset N]"
allowed-tools: Bash(agm session list:*)
---

# List AGM sessions with filters

I'll list all AGM sessions.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract:
  - `--status` (default: active): Filter by session status
  - `--harness`: Filter by agent type
  - `--limit` (default: 100): Maximum sessions to return (1-1000)
  - `--offset` (default: 0): Pagination offset


**Step 2: Run command**

- Run: `session list --output json`

**Step 3: Handle result**

- If exit code is not 0:
  - Show error message from stderr
  - Suggest troubleshooting steps
  - Exit gracefully

**Step 4: Display results**

Parse the JSON and display a table with columns:

| Name | Status | Harness | Project | Updated | 
| --- | --- | --- | --- | --- |


**Error Handling**:
- If no results found: suggest broadening filters
- If service error: suggest diagnostic command
