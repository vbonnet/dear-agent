# Claude Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.claude` directory is the repo-local configuration surface for Claude
Code. It owns Claude-specific instructions, permissions, hook wiring, and local
Claude skill examples while shared policy remains in the root `AGENTS.md`.

## Requirements

**CLAUDE-DIR-01** When Claude Code instructions are loaded from `.claude`, the system shall import the root `AGENTS.md` policy before applying Claude-specific guidance.

**CLAUDE-DIR-02** When Claude Code permissions are configured, the system shall keep allow-list entries in `.claude/settings.json`.

**CLAUDE-DIR-03** When Claude Code hooks are configured, the system shall declare hook commands from `.claude/hooks`.

**CLAUDE-DIR-04** When Claude Code stop hooks are configured, the system shall run guardrail feedback for both `Stop` and `SubagentStop` events.

**CLAUDE-DIR-05** When configuration-directory parity is validated, the system shall map the active `claude-code` harness to `.claude`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates active harness directory mapping.
