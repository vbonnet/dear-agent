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

**CLAUDE-MARKET-04** When neutral marketplace plugin sources are published, the system shall keep matching source paths in the Claude marketplace mirror.

**CLAUDE-MARKET-05** When neutral marketplace plugin versions are published, the system shall keep matching versions in the Claude marketplace mirror.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/marketplace_parity.feature` validates that this mirror matches the neutral marketplace catalog.
