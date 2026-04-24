---
model: haiku
effort: low
description: Lint the engram-kb wiki — broken links, orphans, stale pages, coverage gaps
argument-hint: "[--kb PATH] [--json]"
allowed-tools: Bash(agm wiki lint *)
---

# Wiki Lint

I'll run a comprehensive lint pass on the engram-kb knowledge base.

**Step 1: Run lint**

Parse $ARGUMENTS for optional flags:
- `--kb PATH` — path to KB root (default: ~/src/engram-kb)
- `--json` — emit JSON instead of text

Build and run:
```
agm wiki lint [--kb PATH] [--json]
```

**Step 2: Interpret the report**

The report uses three severity tiers:

| Tier | Code | Meaning |
|------|------|---------|
| 🔴 Error | `BROKEN_LINK` | Internal link target does not exist — must fix |
| 🟡 Warning | `ORPHAN_PAGE` | Page has no inbound links — low discoverability |
| 🟡 Warning | `STALE_PAGE` | Last-updated > 6 months ago |
| 🔵 Info | `MISSING_META` | No `Last updated:` field |
| 🔵 Info | `COVERAGE_GAP` | Fewer than 2 outbound links — likely a stub |

**Step 3: Report to user**

Show the full output. Then:
- If there are 🔴 errors: "There are N broken links. Here's how to fix them: …"
- If there are 🟡 warnings: list the top 5 highest-priority items
- If clean: "✅ Wiki is in good shape."

**Error Handling**:
- If `agm` not found: "Install agm from github.com/vbonnet/dear-agent"
- If KB path invalid: show the error and suggest `--kb ~/src/engram-kb`
