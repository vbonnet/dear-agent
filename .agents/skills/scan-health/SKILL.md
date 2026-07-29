---
name: scan-health
description: Check AGM-managed session health. Use when the user asks whether one or all AGM sessions are healthy, responsive, or resource-constrained, or when an orchestration loop needs typed health evidence before dispatch.
---

# Scan AGM session health

## Workflow

1. Read `../../../agm/agm-plugin/skills/scan-health/SKILL.md` completely.
2. Follow the canonical workflow and all of its gates.

## Verification

Account for every returned session and preserve command failures as failures.
