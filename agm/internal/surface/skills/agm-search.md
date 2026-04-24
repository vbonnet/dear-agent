---
description: Search for matching sessions
argument-hint: "<query> [--status active|archived|all] [--limit N]"
allowed-tools: Bash(agm session search:*)
---

# Search sessions by name with relevance scoring

I'll search for matching sessions.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract:
  - `--query` (required): Search query for session names (case-insensitive)
  - `--status` (default: active): Filter by session status
  - `--limit` (default: 10): Maximum results to return (1-50)


**Step 2: Run command**

- Run: `session search --output json`

**Step 3: Handle result**

- If exit code is not 0:
  - Show error message from stderr
  - Suggest troubleshooting steps
  - Exit gracefully

**Step 4: Display results**

Parse the JSON and display a table with columns:

| Name | Status | Harness | Score | 
| --- | --- | --- | --- |


**Error Handling**:
- If no results found: suggest broadening filters
- If service error: suggest diagnostic command
