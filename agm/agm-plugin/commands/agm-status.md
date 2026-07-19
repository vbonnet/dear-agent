---
model: haiku
effort: low
content-hash: d539ec5b1f412b18d1433434192f069b2d4b73dcda8ff9c79a1df381288ca835
description: >-
  Show aggregate live status for AGM sessions. Use when the user needs session state, branch, worktree, workspace, or uncommitted-change information.
argument-hint: "[--workspace VALUE]"
allowed-tools: Bash(agm session status *)
---

<!-- Code generated from registered Cobra metadata. DO NOT EDIT. -->
# Show aggregate AGM status

## Run

- Treat user-provided values as separate argv values. Never build shell syntax with concatenation, command substitution, or unquoted interpolation.
- Run `agm session status --format json`.
- Forward only requested optional flags: `--workspace`.

## Report

- If AGM exits non-zero, show its stderr and stop. Do not invent a fallback command.
- Present successful structured output with these useful fields when available: Sessions[].Name, Sessions[].State, Sessions[].Branch, Sessions[].Workspace, Sessions[].WorktreePath, Sessions[].Uncommitted.
- If no sessions match, say so without treating the empty result as an error.
