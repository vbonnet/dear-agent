---
model: haiku
effort: low
content-hash: e1811a5da26fec1fc00cd63f18f5c62f9f85f6f6f24bdec798881c5e565457a1
description: Resume an AGM-managed harness session by name, ID prefix, or project match. Use when the user wants to continue an existing session.
argument-hint: "<identifier> [--detached]"
allowed-tools: Bash(agm session resume *), Bash(agm session list *), Write(/tmp/agm-resume-*)
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
	never interpolate prompt text into shell syntax. Also pass
	`--delete-prompt-file` so AGM removes the disposable file after it passes
	validation and before attaching. Omit that flag for caller-owned files.
5. Report the resumed session and harness. On failure, show stderr and stop.
