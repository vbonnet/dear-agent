---
model: sonnet
effort: medium
description: Synthesise a wiki answer from engram-kb and optionally save it as a new page
argument-hint: "<question> [--save] [--category research|decisions]"
allowed-tools: Bash(agm wiki query-save *), Read, Glob, Grep
---

# Wiki Query Save

I'll answer your question using engram-kb and optionally persist the answer
as a new wiki page (the compounding mechanism).

**Step 1: Parse arguments**

Parse $ARGUMENTS:
- The question/query (everything before any flags)
- `--save` — persist the answer as a new page after synthesis
- `--category research|decisions` — where to file the page (default: research)
- `--kb PATH` — KB root (default: ~/src/engram-kb)

**Step 2: Search the wiki**

Use Glob and Grep to find relevant pages in ~/src/engram-kb:

```
# Find pages by topic
Grep("<key terms from query>", "~/src/engram-kb/**/*.md")

# Read the most relevant pages
Read("<path to relevant page>")
```

Focus on:
- `02-research-index/` — topic summaries
- `01-decisions/` — ADRs (architectural decisions)
- `05-synthesis/` — periodic reconciliations

**Step 3: Synthesise the answer**

Read the relevant pages and compose a clear, accurate answer. Cite sources
as wikilinks: e.g. "Per [[ADR-013]], the preferred approach is…"

**Step 4: Present the answer**

Show the synthesised answer to the user.

Then ask: "Would you like me to save this as a wiki page? It will be filed
under 02-research-index/ and woven into the backlink graph."

**Step 5: Save (if requested)**

If the user confirms, run:
```
agm wiki query-save \
  --query "{QUESTION}" \
  --answer "{ANSWER}" \
  [--category decisions] \
  [--kb PATH]
```

The tool will:
1. Write the page
2. Run a backlink audit
3. Regenerate index.md
4. Append to log.md

Report the saved path and any backlink suggestions.

**Error Handling**:
- If no relevant pages found: answer from general knowledge, clearly flagged as
  "not from the wiki"
- If the page already exists: suggest `--output` with a different filename
