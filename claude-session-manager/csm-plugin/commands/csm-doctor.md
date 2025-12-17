---
content-hash: ce5e58e97ed88d20875e4dee6c1a6180e772e8354d941eafa3afa59313acb886
description: Check CSM system health and configuration
allowed-tools: Bash(~/.local/bin/csm:*)
---

# CSM System Health Check

!`csm doctor`

**Health Check Complete**

The output above shows the status of your CSM installation including:
- Claude history file accessibility
- tmux installation and version
- Session manifest health
- Configuration validation

If any issues were found, follow the recommendations provided by the doctor command.
