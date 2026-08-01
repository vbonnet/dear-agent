# Claude Plugin Marketplace Mirror Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.claude-plugin` directory owns the root Claude Code native marketplace
mirror. It projects the harness-neutral `.dear-agent/marketplace.json` catalog
into Claude's native plugin marketplace format.

## Requirements

**CLAUDE-MARKET-01** When Claude Code native marketplace data is published, the system shall keep the catalog at `.claude-plugin/marketplace.json`.

**CLAUDE-MARKET-02** When the Claude marketplace mirror is validated, the system shall match the neutral marketplace name.

**CLAUDE-MARKET-03** When neutral marketplace plugins are published, the system shall include matching plugin names in the Claude marketplace mirror.

**CLAUDE-MARKET-04** When neutral marketplace plugin sources are published, the system shall keep matching source paths or an explicitly validated native packaging adapter in the Claude marketplace mirror.

**CLAUDE-MARKET-05** When neutral marketplace plugin versions are published, the system shall keep matching versions in the Claude marketplace mirror.

**CLAUDE-MARKET-06** When the native SPEC governance plugin is packaged, the system shall authenticate the isolated `spec-governance` plugin root as its distribution root and shall expose only the canonical `audit-specs` and `write-spec` skill directories.

**CLAUDE-MARKET-07** When the isolated native SPEC governance plugin is validated, the system shall require its manifest and canonical directory tree to contain only `audit-specs` and `write-spec`.

**CLAUDE-MARKET-08** If the isolated native SPEC governance plugin manifest names a missing, escaping, duplicate, noncanonical, or symlink-mediated canonical skill directory or exposes agents, hooks, MCP servers, or language servers, then the system shall reject marketplace parity.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/marketplace_parity.feature` validates that this mirror matches the neutral marketplace catalog.
