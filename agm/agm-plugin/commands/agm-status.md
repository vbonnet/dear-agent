---
model: haiku
effort: low
content-hash: d3c3115e2382c7078a7b6185922a9ab706b1411e46c5b01eba6bbbf4745e4ce9
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
- Present successful structured output with these useful fields when available: Session, State, Branch, Workspace, Worktree.
- If no sessions match, say so without treating the empty result as an error.
