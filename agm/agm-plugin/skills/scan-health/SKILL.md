---
name: scan-health
description: Check AGM-managed session health. Use when the user asks whether one or all AGM sessions are healthy, responsive, or resource-constrained, or when an orchestration loop needs typed health evidence before dispatch.
content-hash: 0fe3468299a28c56a4c33fef196eb4e5c5298bdba8262c556477dc7a150e29b3
---

# Scan AGM session health

1. For one requested session, run `agm session health <session> -o json`.
   Otherwise run `agm session health --all -o json`.
2. Pass a session name as one argv value. Do not add a nonexistent subcommand
   `--json` flag, hide stderr, or replace the typed check with shell pipelines.
3. Report each session's health level and the evidence AGM returns. Distinguish
   unhealthy sessions from a failed health command.
4. If the command fails, show stderr and stop. Do not claim host disk or CPU
   health from session evidence.

## Verify

Verification is complete when every returned session is accounted for and a
command failure remains a failure rather than being reported as a health result.
