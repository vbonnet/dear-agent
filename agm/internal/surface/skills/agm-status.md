---
description: Get the current session status
argument-hint: "[--include-archived]"
allowed-tools: Bash(agm session status:*)
---

# Get live status of all sessions with summary counts

I'll get the current session status.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract:
  - `--include-archived`: Include archived sessions in the status report


**Step 2: Run command**

- Run: `session status --output json`

**Step 3: Handle result**

- If exit code is not 0:
  - Show error message from stderr
  - Suggest troubleshooting steps
  - Exit gracefully

**Step 4: Display results**

Parse the JSON and display a table with columns:

| Name | Status | State | Harness | 
| --- | --- | --- | --- |


**Error Handling**:
- If no results found: suggest broadening filters
- If service error: suggest diagnostic command
