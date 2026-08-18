---
model: haiku
effort: low
content-hash: c877679811ff323b69d6b951e9d4bf9bfeaab3ce14d7a0cfb68d07ed0913dd36
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

- If the primary command exits non-zero because its extension or credentials are unavailable, show its stderr and run the documented fallback `agm session list --all --output json`. For any other non-zero exit, show stderr and stop. Do not invent another fallback command.
- Present AGM's result and any confirmation request without changing its meaning.
- If no sessions match, say so without treating the empty result as an error.
