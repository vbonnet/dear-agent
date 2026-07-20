---
model: haiku
effort: low
content-hash: b63f79d7510aa9f45d605b3397935407f291e6427488e8beaa2cfb490378f64e
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
   `agy`, or `opencode-cli`, then repeat with that `--harness` value. Mention
   `gemini-cli` only when the user explicitly needs deprecated compatibility.
4. Report the session ID, harness, workspace, and storage returned by AGM. If
   the command fails, show stderr and stop; do not call tmux or create readiness
   files manually.
