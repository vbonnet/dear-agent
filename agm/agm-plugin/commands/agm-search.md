---
model: haiku
effort: low
content-hash: bfb2721d6374a61dd0d0c1d05d9b19d85b0aec6ff4159f603e7a2a11595f5c6c
description: Search AGM sessions by name or status
argument-hint: "<query>"
allowed-tools: Bash(agm session list *)
---

# AGM Search Sessions

I'll search AGM sessions matching a query.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract the search query
- If $ARGUMENTS is empty or whitespace only:
  - Show: "Search query is required. Usage: /agm:search <query>"
  - Show: "Examples: /agm:search research, /agm:search --status active"
  - Exit gracefully
- Check if query starts with `--status` to filter by status
- Otherwise treat the query as a name pattern

**Step 2: Fetch all sessions**

- Run: `agm session list --output json`
- If exit code is not 0:
  - Show error and suggest: "Try running: agm admin doctor"
  - Exit gracefully
- If output is empty or `[]`:
  - Show: "No sessions found. Create one with /agm:new <name>"
  - Exit gracefully

**Step 3: Filter results**

Parse the JSON array and filter:
- If filtering by `--status <value>`: match sessions where status equals the value (case-insensitive)
- If filtering by name pattern: match sessions where the name contains the query string (case-insensitive)

**Step 4: Display results**

- If no matches found:
  - Show: "No sessions matching '{query}'"
  - Suggest: "Run /agm:list to see all sessions"
  - Exit gracefully
- If matches found, display a table:

| Name | Status | Harness | Project |
|------|--------|---------|---------|

Show: "Found N session(s) matching '{query}'"

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/ai-tools"
- If no matches: suggest `/agm:list` to see all sessions
