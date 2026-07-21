---
model: haiku
effort: low
content-hash: d1b464f25b40421fbb5a3c070f70e6e935aeb63e783e2e60b6974adda3ff808d
description: Create an AGM-managed harness session. Use when the user wants a new Claude Code, Codex CLI, AGY, OpenCode, or Pi session.
argument-hint: "[session-name] [--harness TYPE] [--workspace NAME]"
allowed-tools: Bash(agm session new *)
---

# Create an AGM session

1. Forward the optional session name, `--harness`, and `--workspace` as
   separate argv values to `agm session new`, adding `--output json`.
2. If no name, harness, or workspace was supplied, let AGM perform its current
   detection or prompt. Do not invent a Claude-specific name and do not use the
   removed `--project` flag.
3. Active harnesses are `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and
   `pi-cli`. `gemini-cli` is deprecated compatibility only.
4. Report the created session, harness, workspace, project, and resume hint from
   AGM's result. On failure, show stderr and stop.
