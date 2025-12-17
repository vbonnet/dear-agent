---
content-hash: 01d09e99bf19c1fc83ec012d3260254139d5fcbc2daa5b2cef270e77ea48104b
description: Discover and sync Claude sessions from history
allowed-tools: Bash(~/.local/bin/csm:*)
---

# CSM Session Sync

!`csm sync`

**Session Sync Complete**

The sync command has:
- Scanned `~/.claude/history.jsonl` for Claude session UUIDs
- Created/updated manifests for discovered sessions
- Synchronized session metadata

This is useful for:
- Discovering sessions that don't have manifests yet
- Updating session information after manual changes
- Recovering from corrupted or missing manifests

You can now use `csm resume` in your terminal to access any discovered sessions.
