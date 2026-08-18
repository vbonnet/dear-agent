---
name: agm
description: Manage AGM sessions across Claude Code, Codex CLI, AGY, OpenCode, and Pi. Use when an agent needs to create, associate, list, inspect, resume, message, archive, or troubleshoot AGM-managed sessions; when a user mentions AGM session lifecycle; or when another workflow needs current session state. Do not use for task tracking or orchestration policy.
content-hash: af0f3db9487a23f4c007c2f30f61d72cb0e5be1c46cd9fe13bca093bc48aecb3
---

# AGM session management

Use the installed `agm` binary as the source of truth. Inspect
`agm <command> --help` when a flag is not covered here; do not infer removed
root-form commands or copy a flag catalog into the response.

## Route the request

- Create: `agm session new [name] --harness <type> --workspace <name>`
- Associate: `agm session associate <name> --create --harness auto`
- List: `agm session list --output json [--all]`
- Aggregate status: `agm session status --format json [--workspace <name>]`
- Inspect one session: `agm session get <identifier> --output json`
- Resume: `agm session resume <identifier>`
- Send: `agm send msg <identifier> --prompt-file <path>`
- Archive: `agm session archive <identifier>`; follow the command's active
  versus stopped guidance and never bypass a refusal with `--force`.
- Diagnose: `agm admin doctor`

For the richer Claude plugin workflows, read only the relevant file under
`../../agm-plugin/commands/`.

## Preserve these invariants

- Treat Claude Code, Codex CLI, AGY, OpenCode, and Pi as active harnesses. Mention
  Gemini only as deprecated compatibility when an existing command accepts it.
- Pass user-controlled message or wiki content through a file input. Never
  interpolate it into shell syntax. For send, create a unique
  `/tmp/agm-send-<random>.txt`, pass it with `--prompt-file`, and remove it
  immediately after the command returns on success or failure.
- Resolve session identifiers from AGM's structured list/get output before
  invoking a lifecycle command. Session identifiers are typed AGM data, not
  arbitrary user text; never splice an unverified user-provided token into a
  shell command.
- Resume requires an identifier. If none was provided, list all sessions and
  ask the user to choose; the interactive picker is not implemented.
- For a disposable recovery prompt, create a unique file, pass both
  `--prompt-file <path>` and `--delete-prompt-file`, and let AGM delete it
  after the prompt passes validation. Omit the delete flag for caller-owned
  files.
- Use global `--output json` or the command's documented structured-output flag;
  do not invent `--json` on subcommands.
- Keep stderr visible and stop on typed command errors.
- A session is complete only after its work is merged, deployed when
  applicable, and verified. Archival does not prove those delivery gates.

## Verify

Confirm the command's structured result and, for lifecycle changes, re-read the
session with `agm session get <identifier> --output json`.
