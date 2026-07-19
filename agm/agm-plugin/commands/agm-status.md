---
model: haiku
effort: low
content-hash: c6d30fbd2a903a820ea00aad719af54031c354579db8419accacc92a7a3d238f
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
- Present successful structured output with these useful fields when available: sessions[].name, sessions[].state, sessions[].branch, sessions[].workspace, sessions[].worktree_path, sessions[].uncommitted.
- If no sessions match, say so without treating the empty result as an error.
