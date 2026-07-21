---
name: beads
description: Use when working in a repository that uses bd or Beads for durable project task tracking, issue dependencies, blocker management, multi-session handoff, or shared work memory. Trigger when the user asks to find ready work, claim or close tasks, create follow-up work, inspect blockers, recover project context, or choose between local planning and persistent project tracking.
---

# Beads

Use Beads as the shared project task system. Local plans, scratch files, and personal memories are useful, but they are not the durable source of truth for project work.

## First Step

Run:

```bash
bd --db ~/beads/context-engine/.beads prime
```

If that prints nothing, check whether the repository has an active Beads workspace:

```bash
bd --db ~/beads/context-engine/.beads where
```

## Preferred Route

Use the `bd` CLI when shell access is available. It is the most compact and direct Beads interface.

## Core CLI Workflow

1. Find work:

```bash
bd --db ~/beads/context-engine/.beads ready
bd --db ~/beads/context-engine/.beads list --status=open
bd --db ~/beads/context-engine/.beads list --status=in_progress
```

2. Inspect before editing:

```bash
bd --db ~/beads/context-engine/.beads show <id>
```

3. Claim work atomically:

```bash
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on update <id> --claim
```

4. Create durable follow-up work when implementation reveals new tasks:

```bash
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on create "Short title" --description="Why this exists and what needs to be done" --type=task --priority=2
```

5. Close completed work:

```bash
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on close <id> --reason="Completed"
```

## What Belongs In Beads

Use Beads for:

- shared project tasks
- blockers and dependencies
- discovered follow-up work
- work that must survive thread reset, compaction, or handoff
- status that another person or agent should be able to resume

Use agent-local planning tools only for the current turn's execution checklist. Do not treat them as shared project state.

## Rules

- Do not create markdown TODO files as the source of truth when Beads is available.
- Do not use `bd edit`; it opens an interactive editor. Use `bd update` flags instead.
- Prefer `--json` when parsing `bd` output programmatically.
- If hooks are installed, `bd prime` may already be injected. Run it manually when context is missing.
- Do not auto-close or mutate tasks unless the work is actually complete.

## Verify

After a mutation, re-read the task from the same canonical database:

```bash
bd --db ~/beads/context-engine/.beads show <id>
```

Confirm its status, dependencies, and recorded context match the intended
change. A successful command against another database is not completion.
