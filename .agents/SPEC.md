# AGENTS-Compatible Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.agents` directory owns AGY-specific instructions and hook wiring plus the
single AGENTS-compatible repository skill tree consumed by AGY and Codex. It
imports the shared repository policy from `AGENTS.md`.

## Requirements

**AGENTS-DIR-01** When AGY configuration is loaded, the system shall import the root `AGENTS.md` policy before applying AGY-specific guidance.

**AGENTS-DIR-02** When AGY hooks are configured, the system shall declare hook commands from `.agents/hooks` rather than reusing another harness directory.

**AGENTS-DIR-03** When AGY session lifecycle hooks run, the system shall use AGY-specific Beads hook event names.

**AGENTS-DIR-04** When AGENTS-compatible repository skills are published, the system shall keep their single canonical discovery tree under `.agents/skills`.

**AGENTS-DIR-05** When configuration-directory parity is validated, the system shall map the active `agy` harness to `.agents`.

**AGENTS-DIR-06** When Codex loads repository skills, the system shall use `.agents/skills` directly and shall not publish duplicate `.codex/skills` entrypoints.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates active harness directory mapping.
