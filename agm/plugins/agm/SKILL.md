---
name: agm
description: Manage AGM sessions across Claude Code, Codex CLI, AGY, and OpenCode. Use when an agent needs to create, associate, list, inspect, resume, message, archive, or troubleshoot AGM-managed sessions; when a user mentions AGM session lifecycle; or when another workflow needs current session state. Do not use for task tracking or orchestration policy.
---

# AGM session management

Use the installed `agm` binary as the source of truth. Inspect
`agm <command> --help` when a flag is not covered here; do not infer removed
root-form commands or copy a flag catalog into the response.

## Route the request

- Create: `agm session new [name] --harness <type> --workspace <name>`
- Associate: `agm session associate <name> --create --harness auto`
- List: `agm session list --output json [--all]`
- Aggregate status: `agm session status --format json`. Do not combine JSON
  output with `--workspace`; that filter currently applies only to table output.
- Inspect one session: `agm session get <identifier> --output json`
- Resume: `agm session resume [identifier]`
- Send: `agm send msg <identifier> --prompt-file <path>`; outside an
  AGM-managed session, also pass `--sender <identity>` because no sender can be
  inferred from session context.
- Archive: `agm session archive <identifier>`; follow the command's active
  versus stopped guidance and never bypass a refusal with `--force`.
- Diagnose: `agm admin doctor`

For the richer Claude plugin workflows, read only the relevant file under
`../../agm-plugin/commands/`.

## Preserve these invariants

- Treat Claude Code, Codex CLI, AGY, and OpenCode as active harnesses. Mention
  Gemini only as deprecated compatibility when an existing command accepts it.
- Pass user-controlled message or wiki content through a file input. Never
  interpolate it into shell syntax.
- Use global `--output json` or the command's documented structured-output flag;
  do not invent `--json` on subcommands.
- Keep stderr visible and stop on typed command errors.
- A session is complete only after its work is merged, deployed when
  applicable, and verified. Archival does not prove those delivery gates.

## Verify

Confirm the command's structured result and, for lifecycle changes, re-read the
session with `agm session get <identifier> --output json`.
