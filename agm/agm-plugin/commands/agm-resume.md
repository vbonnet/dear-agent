---
model: haiku
effort: low
content-hash: c01af4e73e6156d359246d644de19b43b3f985638e5b1a9142d2a863e74b694f
description: Resume an AGM-managed harness session by name, ID prefix, or project match. Use when the user wants to continue an existing session.
argument-hint: "<identifier> [--detached]"
allowed-tools: Bash(agm session resume *), Bash(agm session list *), Bash(rm -f -- /tmp/agm-resume-*), Write(/tmp/agm-resume-*)
---

# Resume an AGM session

1. Run `agm session resume <identifier> --output json`, adding `--detached` only
   when requested. Pass the identifier as one argv value.
2. Require an identifier. If none was provided, run
   `agm session list --all --output json` and ask the user to choose; AGM does
   not currently provide an interactive picker.
3. If the session is not found, run `agm session list --all --output json` and
   present likely identifiers. Do not guess or rewrite the resume command.
4. For a recovery prompt, write the prompt to a unique
   `/tmp/agm-resume-<random>.txt` file and use the typed `--prompt-file` flag;
   never interpolate prompt text into shell syntax. Always run
   `rm -f -- <path>` immediately after `agm session resume` returns, before
   reporting success or failure.
5. Report the resumed session and harness. On failure, show stderr and stop.
