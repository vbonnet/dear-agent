---
content-hash: ce5e58e97ed88d20875e4dee6c1a6180e772e8354d941eafa3afa59313acb886
description: Check CSM system health and configuration
allowed-tools: Bash(csm doctor:*)
---

# CSM System Health Check

!`csm doctor`

**Health Check Complete**

The output shows CSM installation status:
- Claude history file accessibility
- tmux installation and version
- Session manifest health
- Configuration validation

If issues found, follow the recommendations provided.

**Error Handling**:
- If csm not found: "Install CSM from github.com/user/ai-tools"
- If health check fails: Review output for specific issues, run recommended fixes
