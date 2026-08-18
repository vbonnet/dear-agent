---
model: haiku
effort: low
content-hash: 902e8bd99fc1a792908f7e05a8f25bb82bfd587b2574a793afd1bb97299c7350
description: Associate the current harness session with AGM. Use after starting a supported CLI outside AGM or when its session metadata needs to be re-linked.
argument-hint: "[session-name]"
allowed-tools: Bash(agm get-session-name), Bash(agm session associate *)
---

# Associate an AGM session

1. Use the supplied session name. If none was supplied, run
   `agm get-session-name`; stop if it cannot identify a session.
2. Run `agm session associate <session-name> --create --harness auto --output json`.
   Pass the name as one argv value; do not construct shell syntax from it.
3. If AGM cannot infer the harness, ask for one of `claude-code`, `codex-cli`,
   `agy`, `opencode-cli`, or `pi-cli`, then repeat with that `--harness` value. Mention
   `gemini-cli` only when the user explicitly needs deprecated compatibility.
4. Report the session ID, harness, workspace, and storage returned by AGM. If
   the command fails, show stderr and stop; do not call tmux or create readiness
   files manually.
