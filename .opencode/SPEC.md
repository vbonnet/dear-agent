# OpenCode Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.opencode` directory is the repo-local configuration surface for OpenCode
CLI. It owns OpenCode hook wiring and fallback metadata for parity with the
other supported active harnesses.

## Requirements

**OPENCODE-DIR-01** When OpenCode hooks are configured, the system shall declare hook commands from `.opencode/hooks`.

**OPENCODE-DIR-02** When OpenCode pre-tool hooks run, the system shall include routing, Beads completion, bypass, and PR lifecycle guardrails.

**OPENCODE-DIR-03** When OpenCode stop hooks are configured, the system shall run guardrail feedback for both `Stop` and `SubagentStop` events.

**OPENCODE-DIR-04** When OpenCode lifecycle hooks run, the system shall use OpenCode-specific Beads hook event names.

**OPENCODE-DIR-05** When configuration-directory parity is validated, the system shall map the active `opencode-cli` harness to `.opencode`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates active harness directory mapping.
