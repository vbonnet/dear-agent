# OpenCode Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.opencode` directory is the repo-local configuration surface for OpenCode
CLI. Its project plugin owns native hook wiring; `hooks.json` is only exact
retirement metadata for the unsupported legacy JSON projection.

## Requirements

**OPENCODE-DIR-01** When OpenCode hooks are configured, the native project plugin shall invoke the repository guard commands from `.opencode/hooks`.

**OPENCODE-DIR-02** When OpenCode pre-tool hooks run, the system shall include routing, Beads completion, bypass, and PR lifecycle guardrails.

**OPENCODE-DIR-03** When OpenCode terminal feedback is configured, the system shall use bounded `session.idle` follow-up without claiming unsupported `Stop` or `SubagentStop` events.

**OPENCODE-DIR-04** When OpenCode lacks a neutral native Beads lifecycle projection, the system shall leave that lifecycle capability explicitly unmet instead of relabeling unrelated events as equivalent hooks.

**OPENCODE-DIR-05** When configuration-directory parity is validated, the system shall map the active `opencode-cli` harness to `.opencode`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates active harness directory mapping.
- `agm/test/bdd/features/hook_parity.feature` validates native pre-tool guards and bounded terminal feedback.
