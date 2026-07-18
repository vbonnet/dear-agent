---
model: haiku
effort: low
content-hash: c3e414ad73b1fec31493e793dcb3159faff6d1d0953435a6cd36f2375bdf9d6f
description: >-
  Search archived Claude conversation history semantically. Use for a remembered topic when the Claude and Vertex AI extension is available; otherwise use the harness-neutral list fallback.
argument-hint: "<query> [--max-results N]"
allowed-tools: Bash(agm session search *), Bash(agm session list *)
---

<!-- Code generated from registered Cobra metadata. DO NOT EDIT. -->
# Search archived AGM sessions

## Run

- Treat user-provided values as separate argv values. Never build shell syntax with concatenation, command substitution, or unquoted interpolation.
- Run `agm session search <query>`.
- Forward only requested optional flags: `--max-results`.
- `agm session search` is a Claude-history and Vertex AI extension. It may prompt before restoring a result.
- For other harnesses, missing Vertex credentials, or a non-interactive lookup, run the fallback and filter session names, tags, and projects in memory. Run `agm session list --all --output json`.

## Report

- If AGM exits non-zero, show its stderr and stop. Do not invent a fallback command.
- Present AGM's result and any confirmation request without changing its meaning.
- If no sessions match, say so without treating the empty result as an error.
