---
model: haiku
effort: low
content-hash: 71a942c0418c8e63f4c72a01a786ced9f34e3f0fc979655168a04b42d88d4385
description: Send a message to one or more active AGM sessions. Use when the user wants to contact, redirect, or delegate to an AGM-managed agent.
argument-hint: "<session> <message> [--priority LEVEL]"
allowed-tools: Bash(agm send msg *), Bash(agm session list *), Bash(rm -f -- /tmp/agm-send-*), Write(/tmp/agm-send-*)
---

# Send an AGM message

1. Require a recipient and message. Accept priority only from `fyi`,
   `background`, `normal`, `urgent`, or `critical`.
2. Use the Write tool to place the exact message in a unique
   `/tmp/agm-send-<unique>.txt` file. Do not put message text in a shell
   command, even when it appears safely quoted.
3. Run `agm send msg <session> --prompt-file <path> --priority <level> --output json`.
   Pass the recipient and path as separate argv values. Add `--sender` only
   when AGM says the external caller must identify itself.
4. Always run `rm -f -- <path>` immediately after AGM exits, before reporting
   success or failure. The temporary message file must not survive either path.
5. If the session is missing, run `agm session list --output json` and present
   current names. Do not select a different recipient automatically.
6. Report AGM's delivery status and message ID. On failure, show stderr and
   stop; never fall back to direct tmux input.
