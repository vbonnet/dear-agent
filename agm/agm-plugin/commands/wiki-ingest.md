---
model: haiku
effort: low
description: Audit backlinks and update index after adding a new page to engram-kb
argument-hint: "--page <repo-relative-path> [--kb PATH] [--no-index]"
allowed-tools: Bash(agm wiki ingest *), Bash(agm wiki index *)
---

# Wiki Ingest

I'll audit backlinks for a newly ingested page and update the index.

**Step 1: Parse arguments**

Parse $ARGUMENTS to extract:
- `--page PATH` — repo-relative path to the new page (required)
- `--kb PATH` — KB root (default: ~/src/engram-kb)
- `--no-index` — skip index regeneration

If `--page` is missing, ask the user: "Which page did you just add? (repo-relative path, e.g. 02-research-index/topic-foo.md)"

**Step 2: Run backlink audit**

```
agm wiki ingest --page "{PAGE}" [--kb PATH] [--no-index]
```

**Step 3: Interpret backlink suggestions**

For each suggestion the tool outputs:
```
📄 <source-page>
   matched: <terms>
   suggest adding: [[<target-stem>]]
```

Present a concise summary:
- "Found N pages that mention this topic and could link back."
- List the source pages with the suggested wikilink to add.

**Step 4: Apply links (optional)**

If the user says "apply" or "yes, add the links":
- For each suggested source page, open it and add `[[target-stem]]` in an
  appropriate location (e.g. "See Also" section or Related ADRs line).
- Confirm each edit before writing.

**Error Handling**:
- If the page is not found: remind the user to write the file first, then re-run
- If agm is not found: "Install agm from github.com/vbonnet/dear-agent"
