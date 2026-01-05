---
content-hash: 01d09e99bf19c1fc83ec012d3260254139d5fcbc2daa5b2cef270e77ea48104b
description: Discover and sync Claude sessions from history
allowed-tools: Bash(csm sync:*)
---

# CSM Session Sync

!`csm sync`

**Session Sync Complete**

The sync command:
- Scans ~/.claude/history.jsonl for Claude session UUIDs
- Creates/updates manifests for discovered sessions
- Synchronizes session metadata

**Use cases**:
- Discover sessions without manifests
- Update session info after manual changes
- Recover from corrupted/missing manifests

Next: Use `csm resume` in terminal to access discovered sessions.

**Error Handling**:
- If csm not found: "Install CSM from github.com/user/ai-tools"
- If sync fails: Check ~/.claude/history.jsonl exists and is readable
- If no sessions found: Verify Claude Code has been used before
