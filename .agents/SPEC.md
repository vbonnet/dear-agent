# AGENTS-Compatible Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.agents` directory is the repo-local configuration surface for the AGY
Antigravity harness. It owns AGY-specific instructions, hook wiring, and skill
fallback assets while importing the shared repository policy from `AGENTS.md`.

## Requirements

**AGENTS-DIR-01** When AGY configuration is loaded, the system shall import the root `AGENTS.md` policy before applying AGY-specific guidance.

**AGENTS-DIR-02** When AGY hooks are configured, the system shall declare hook commands from `.agents/hooks` rather than reusing another harness directory.

**AGENTS-DIR-03** When AGY session lifecycle hooks run, the system shall use AGY-specific Beads hook event names.

**AGENTS-DIR-04** When AGY skill fallback assets are published, the system shall keep them under `.agents/skills`.

**AGENTS-DIR-05** When configuration-directory parity is validated, the system shall map the active `agy` harness to `.agents`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates active harness directory mapping.
