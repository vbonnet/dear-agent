---
model: haiku
effort: low
content-hash: 4a2b2c0ad376fc975e82665b1e0b537b11062067f3630c3e3f2e1b742c44c83d
description: >-
  List AGM sessions. Use when the user needs current or archived session names, states, harnesses, workspaces, tags, or trust information.
argument-hint: "[--all] [--tag VALUE] [--filter VALUE] [--trust]"
allowed-tools: Bash(agm session list *)
---

<!-- Code generated from registered Cobra metadata. DO NOT EDIT. -->
# List AGM sessions

## Run

- Treat user-provided values as separate argv values. Never build shell syntax with concatenation, command substitution, or unquoted interpolation.
- Run `agm session list --output json`.
- Forward only requested optional flags: `--all`, `--tag`, `--filter`, `--trust`.

## Report

- If AGM exits non-zero, show its stderr and stop. Do not invent a fallback command.
- Present successful structured output with these useful fields when available: Name, Status, Harness, Workspace, Updated.
- If no sessions match, say so without treating the empty result as an error.
