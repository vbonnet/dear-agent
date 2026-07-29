---
name: scan-health
description: Check AGM-managed session health. Use when one or all managed sessions need typed health evidence before diagnosis or dispatch.
---

# Scan AGM session health

This regular-file entrypoint supplies OpenCode's native repository discovery.

## Workflow

1. Read `../../../agm/agm-plugin/skills/scan-health/SKILL.md` completely.
2. Follow that canonical workflow without substituting this discovery
   entrypoint for any of its requirements or gates.

## Verification

Apply the exported skill's verification criteria. Account for every returned
session and preserve command failures as failures.
