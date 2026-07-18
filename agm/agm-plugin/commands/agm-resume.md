---
model: haiku
effort: low
content-hash: 25c0f71158fb69ac12d4f7f49356f8692790fe0e9ed334d7f0c49ade832546ef
description: Resume an AGM-managed harness session by name, ID prefix, or project match. Use when the user wants to continue an existing session.
argument-hint: "[identifier] [--detached]"
allowed-tools: Bash(agm session resume *), Bash(agm session list *)
---

# Resume an AGM session

1. Run `agm session resume <identifier> --output json`, adding `--detached` only
   when requested. Pass the identifier as one argv value.
2. With no identifier, allow AGM's current interactive selection behavior.
3. If the session is not found, run `agm session list --all --output json` and
   present likely identifiers. Do not guess or rewrite the resume command.
4. For a recovery prompt, write the prompt to a file and use the typed
   `--prompt-file` flag; never interpolate prompt text into shell syntax.
5. Report the resumed session and harness. On failure, show stderr and stop.
