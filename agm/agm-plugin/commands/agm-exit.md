---
model: sonnet
effort: low
content-hash: 9302bfc66567700b25f6094b0dc83932276711ff68b26c08dd7fe6d5bbc6d4bc
description: Archive the current AGM session after its work is delivered. Use only when the user wants to exit and the merged, applicable deployed, and verified completion gates are satisfied.
argument-hint: "[session-name]"
allowed-tools: Bash(agm get-session-name), Bash(agm session get *), Bash(agm session archive *)
---

# Archive and exit an AGM session

1. Use `$ARGUMENTS` as the session name when present. Otherwise run
   `agm get-session-name`; stop if AGM cannot identify the session. Do not call
   tmux directly.
2. Run `agm session get <session-name> --output json` and inspect its current
   status. Pass the name as one argv value.
3. Confirm the work already meets the repository definition of done: merged,
   deployed when applicable, and verified. This skill does not weaken that gate
   to “PR open” or “committed”.
4. For an active session, run
   `agm session archive <session-name> --async --cleanup-worktrees`. For a
   stopped session, omit `--async`. Never use `--force` to bypass a refusal.
5. If AGM rejects archival, show its typed verification errors and stop. Do not
   reproduce git, worktree, permission, marker, or delivery checks in prose.
6. On success, report that archival was accepted. Do not claim merge,
   deployment, verification, pane closure, or archive completion unless AGM's
   current result proves it.
