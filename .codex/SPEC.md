# Codex Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.codex` directory is the repo-local configuration surface for Codex CLI.
It owns Codex-specific feature flags and hook wiring while sharing hook behavior
with the repository's harness-neutral guardrails.

## Requirements

**CODEX-DIR-01** When Codex configuration is loaded, the system shall keep Codex feature flags in `.codex/config.toml`.

**CODEX-DIR-02** When Codex hooks are configured, the system shall declare hook commands from `.codex/hooks`.

**CODEX-DIR-03** When Codex write-sensitive tools are matched, the system shall include the Beads directory block guard.

**CODEX-DIR-04** When Codex lifecycle hooks run, the system shall use Codex-specific Beads hook event names.

**CODEX-DIR-05** When configuration-directory parity is validated, the system shall map the active `codex-cli` harness to `.codex`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates active harness directory mapping.
